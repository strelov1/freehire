package linkedinprofile

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testClient points a Client at a stub instead of LinkedIn. It deliberately uses a
// plain http.Client: NewClient's guarded one refuses private address space, which is
// exactly where a test server lives. TestNewClientRefusesPrivateAddressSpace covers
// the guard itself.
func testClient(base string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		base:       base,
		maxBody:    defaultMaxBody,
	}
}

func TestFetchReadsAProfile(t *testing.T) {
	t.Parallel()

	page := string(fixture(t, "profile.html"))

	var gotPath, gotCookie, gotAuth, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotCookie = r.URL.Path, r.Header.Get("Cookie")
		gotAuth, gotUA = r.Header.Get("Authorization"), r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	got, err := testClient(srv.URL).Fetch(context.Background(), "https://br.linkedin.com/in/istrelov?trk=share")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.Name != "Dana Okonkwo" {
		t.Errorf("Name = %q", got.Name)
	}

	// Whatever form the user pasted, one canonical profile path is requested.
	if gotPath != "/in/istrelov" {
		t.Errorf("requested path = %q, want %q", gotPath, "/in/istrelov")
	}
	// The promise the whole design rests on: this reads LinkedIn as a stranger.
	if gotCookie != "" {
		t.Errorf("request carried a Cookie header: %q", gotCookie)
	}
	if gotAuth != "" {
		t.Errorf("request carried an Authorization header: %q", gotAuth)
	}
	// Identifying ourselves honestly is a choice, not an accident: a public profile
	// answers this user-agent, so there is no reason to wear a browser's.
	if !strings.Contains(gotUA, "freehire") {
		t.Errorf("User-Agent = %q, want it to name us", gotUA)
	}
}

func TestFetchRejectsABadURLWithoutARequest(t *testing.T) {
	t.Parallel()

	var reached bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}))
	defer srv.Close()

	_, err := testClient(srv.URL).Fetch(context.Background(), "https://linkedin.com.evil.example/in/istrelov")
	if !errors.Is(err, ErrNotAProfileURL) {
		t.Fatalf("err = %v, want ErrNotAProfileURL", err)
	}
	if reached {
		t.Error("a rejected URL still caused an outbound request")
	}
}

func TestFetchFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler http.HandlerFunc
		client  func(base string) *Client
	}{
		{
			name:    "a non-200 response",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) },
		},
		{
			name:    "a server error",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		},
		{
			// A body past the cap is a failed measurement, not a short read: parsing
			// the prefix would report whatever happened to fit.
			name: "a body past the cap",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(strings.Repeat("x", 4096)))
			},
			client: func(base string) *Client {
				c := testClient(base)
				c.maxBody = 512
				return c
			},
		},
		{
			name: "a server that never answers",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(2 * time.Second)
			},
			client: func(base string) *Client {
				c := testClient(base)
				c.httpClient = &http.Client{Timeout: 50 * time.Millisecond}
				return c
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			newClient := tt.client
			if newClient == nil {
				newClient = testClient
			}
			got, err := newClient(srv.URL).Fetch(context.Background(), "istrelov")
			if !errors.Is(err, ErrFetch) {
				t.Fatalf("err = %v, want ErrFetch", err)
			}
			if got.Name != "" {
				t.Errorf("a failed fetch returned %+v", got)
			}
		})
	}
}

// A page that arrives fine but says nothing is a different outcome from a page that
// did not arrive, and the caller tells the user different things about them.
func TestFetchSeparatesAnUnreadablePageFromAFailedFetch(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>Join LinkedIn to continue</body></html>`))
	}))
	defer srv.Close()

	_, err := testClient(srv.URL).Fetch(context.Background(), "istrelov")
	if !errors.Is(err, ErrNoProfile) {
		t.Fatalf("err = %v, want ErrNoProfile", err)
	}
	if errors.Is(err, ErrFetch) {
		t.Error("an unreadable page was reported as a failed fetch")
	}
}

// The production client must refuse to be pointed at the network it runs inside,
// which is what stops a redirect from turning this into an SSRF gadget.
func TestNewClientRefusesPrivateAddressSpace(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("should never be read"))
	}))
	defer srv.Close()

	c := NewClient()
	c.base = srv.URL // 127.0.0.1 — private, and the guard should say so

	if _, err := c.Fetch(context.Background(), "istrelov"); !errors.Is(err, ErrFetch) {
		t.Fatalf("err = %v, want ErrFetch — the guarded client answered a private address", err)
	}
}
