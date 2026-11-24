package scraplingfetch

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeService stands in for services/scrapling-fetch: it records the /fetch request it was
// sent and answers with whatever the test sets.
type fakeService struct {
	got     wireRequest
	auth    string
	status  int
	answer  any
	rawBody string
}

func (f *fakeService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.auth = r.Header.Get("Authorization")
	if r.URL.Path != "/fetch" || r.Method != http.MethodPost {
		http.Error(w, "wrong endpoint", http.StatusNotFound)
		return
	}
	_ = json.NewDecoder(r.Body).Decode(&f.got)
	if f.status != 0 && f.status != http.StatusOK {
		w.WriteHeader(f.status)
		_, _ = io.WriteString(w, `{"error":"browser crashed"}`)
		return
	}
	if f.rawBody != "" {
		_, _ = io.WriteString(w, f.rawBody)
		return
	}
	_ = json.NewEncoder(w).Encode(f.answer)
}

func newTransport(t *testing.T, svc *fakeService, mode Mode, proxy bool) *Transport {
	t.Helper()
	srv := httptest.NewServer(svc)
	t.Cleanup(srv.Close)
	tr, err := New(Config{URL: srv.URL, Token: "tok"}, mode, proxy, 5*time.Second)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return tr
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestRoundTripEncodesTheRequest(t *testing.T) {
	svc := &fakeService{answer: wireResponse{Status: 200, URL: "https://ex.com/a", Body: b64("ok")}}
	tr := newTransport(t, svc, ModeHTTP, true)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://ex.com/a?x=1", strings.NewReader(`{"q":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Add("Accept", "text/html")
	req.Header.Add("Accept", "application/json")
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	resp.Body.Close()

	if svc.auth != "Bearer tok" {
		t.Errorf("Authorization = %q, want the bearer token", svc.auth)
	}
	g := svc.got
	if g.URL != "https://ex.com/a?x=1" || g.Method != http.MethodPost || g.Mode != "http" || !g.Proxy {
		t.Errorf("request = %+v, want the URL, POST, http mode and proxy", g)
	}
	if body, _ := base64.StdEncoding.DecodeString(g.Body); string(body) != `{"q":1}` {
		t.Errorf("body = %q, want the request body", body)
	}
	if g.Headers["Content-Type"] != "application/json" || g.Headers["Accept"] != "text/html, application/json" {
		t.Errorf("headers = %v, want them flattened with repeated values joined", g.Headers)
	}
}

func TestRoundTripRebuildsTheUpstreamResponse(t *testing.T) {
	svc := &fakeService{answer: wireResponse{
		Status:  404,
		URL:     "https://ex.com/final",
		Headers: map[string]string{"Content-Type": "text/html"},
		Body:    b64("gone"),
	}}
	tr := newTransport(t, svc, ModeBrowser, false)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://ex.com/start", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 404 || string(body) != "gone" || resp.Header.Get("Content-Type") != "text/html" {
		t.Errorf("response = %d %q %v, want the upstream 404 passed through", resp.StatusCode, body, resp.Header)
	}
	if resp.Request.URL.String() != "https://ex.com/final" {
		t.Errorf("Request.URL = %s, want the final URL after redirects", resp.Request.URL)
	}
	if svc.got.Mode != "browser" || svc.got.Proxy {
		t.Errorf("mode/proxy = %q/%v, want browser/direct", svc.got.Mode, svc.got.Proxy)
	}
}

func TestServiceFailureIsATransportError(t *testing.T) {
	tr := newTransport(t, &fakeService{status: http.StatusBadGateway}, ModeHTTP, false)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://ex.com", nil)
	err := roundTripErr(tr, req)
	var failure *ServiceError
	if !errors.As(err, &failure) || failure.StatusCode != http.StatusBadGateway || !strings.Contains(err.Error(), "browser crashed") {
		t.Fatalf("RoundTrip error = %v, want the service's own error surfaced", err)
	}
}

func TestMalformedAnswerIsATransportError(t *testing.T) {
	tr := newTransport(t, &fakeService{rawBody: `{"status":200,"body":"%%%"}`}, ModeHTTP, false)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://ex.com", nil)
	if err := roundTripErr(tr, req); err == nil {
		t.Fatal("RoundTrip: want an error for a body that is not base64")
	}
}

func TestUnreachableServiceIsATransportError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // nothing listens any more
	tr, err := New(Config{URL: srv.URL, Token: "tok"}, ModeHTTP, false, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://ex.com", nil)
	if err := roundTripErr(tr, req); err == nil {
		t.Fatal("RoundTrip: want an error when the service is down")
	}
}

func TestNewRefusesAnIncompleteConfig(t *testing.T) {
	for _, cfg := range []Config{{URL: "", Token: "t"}, {URL: "http://127.0.0.1:8098", Token: ""}, {URL: "::bad", Token: "t"}} {
		if _, err := New(cfg, ModeHTTP, false, time.Second); err == nil {
			t.Errorf("New(%+v): want an error", cfg)
		}
	}
	if _, err := New(Config{URL: "http://127.0.0.1:8098", Token: "t"}, Mode("teleport"), false, time.Second); !errors.Is(err, ErrUnknownMode) {
		t.Errorf("New with an unknown mode: err = %v, want ErrUnknownMode", err)
	}
}

// roundTripErr runs one round trip for a test that only cares whether it failed, closing any
// response it did get.
func roundTripErr(tr *Transport, req *http.Request) error {
	resp, err := tr.RoundTrip(req)
	if resp != nil {
		resp.Body.Close()
	}
	return err
}
