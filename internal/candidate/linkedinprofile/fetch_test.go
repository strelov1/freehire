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

// blockUntilClientGivesUp returns a handler that answers nothing until the caller hangs up,
// so the client's timeout is what ends the request. Sleeping a fixed span instead would make
// srv.Close() wait out that span for real; waiting on a channel the test closes deadlocks,
// because srv.Close() runs first and blocks on this very handler. The request's own context
// is the one signal that arrives at the right moment, and it is also what really happens.
func blockUntilClientGivesUp() http.HandlerFunc {
	return func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }
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
			name:    "a server that never answers",
			handler: blockUntilClientGivesUp(),
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

// Production reaches LinkedIn only through the egress proxy — the datacenter IP is answered
// with 999 and no JSON-LD. A misconfigured value must therefore be visible as a failed
// import, never as a silent direct fetch that always fails for a reason nobody can see.
func TestEgressProxy(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{"unset means direct", "", ""},
		{"blank means direct", "   ", ""},
		{"a proxy is used", "http://user:pw@proxy.example:8080", "proxy.example:8080"},
		{"a value with no host degrades to direct", "not-a-url", ""},
		{"an unparseable value degrades to direct", "http://[::1", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SOURCES_PROXY_URL", tt.env)
			got := egressProxy()
			if tt.want == "" {
				if got != nil {
					t.Fatalf("egressProxy() = %v, want nil", got)
				}
				return
			}
			if got == nil || got.Host != tt.want {
				t.Fatalf("egressProxy() = %v, want host %q", got, tt.want)
			}
		})
	}
}

// A body exactly at the cap is a whole page and must succeed; the cap is a bound, not a
// budget. This pins the boundary against a future >= where a > belongs.
func TestFetchAcceptsABodyExactlyAtTheCap(t *testing.T) {
	t.Parallel()

	const node = `{"@type":"Person","name":"Dana Okonkwo"}`
	body := `<html><script type="application/ld+json">` + node + `</script><!--`
	body += strings.Repeat("x", 1024-len(body)-3) + "-->"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := testClient(srv.URL)
	c.maxBody = int64(len(body))

	got, err := c.Fetch(context.Background(), "danaokonkwo")
	if err != nil {
		t.Fatalf("a body exactly at the cap was rejected: %v", err)
	}
	if got.Name != "Dana Okonkwo" {
		t.Errorf("Name = %q", got.Name)
	}
}

// LinkedIn treats public ids case-insensitively, so a user who typed their own name with a
// capital must reach the same one page as everyone else.
func TestFetchFoldsTheIDToOneCanonicalRequest(t *testing.T) {
	t.Parallel()

	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write(fixture(t, "profile.html"))
	}))
	defer srv.Close()

	c := testClient(srv.URL)
	for _, in := range []string{"Istrelov", "https://www.linkedin.com/in/ISTRELOV"} {
		if _, err := c.Fetch(context.Background(), in); err != nil {
			t.Fatalf("Fetch(%q): %v", in, err)
		}
	}
	for _, p := range paths {
		if p != "/in/istrelov" {
			t.Errorf("requested %q, want %q", p, "/in/istrelov")
		}
	}
}

// The production client must refuse to be pointed at the network it runs inside — the
// guard sits in the transport's dialer, so it applies to every hop, not just the first.
// This exercises the first hop; a redirect hop is the same dial through the same Control
// hook, and testing it hermetically would need hop one to resolve publicly.
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
