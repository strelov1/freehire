//go:build integration

// Integration test for the browser session: it needs a real Chrome, so it is tagged and it
// SKIPS when none is installed rather than failing. CI's backend job installs no browser, and
// a test that turned "no Chrome here" into a red build would make every unrelated change look
// broken. Same contract the CV renderer's tests have for typst.
//
// Run with: go test -tags=integration ./internal/platform/browser/
package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// requireChrome skips unless a browser chromedp can launch is on this machine.
func requireChrome(t *testing.T) {
	t.Helper()
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if _, err := exec.LookPath(name); err == nil {
			return
		}
	}
	// chromedp finds the macOS app bundle without it being on PATH.
	if _, err := exec.LookPath("open"); err == nil {
		if _, err := exec.Command("test", "-d", "/Applications/Google Chrome.app").Output(); err == nil {
			return
		}
	}
	t.Skip("chrome not installed; skipping browser session test")
}

// challengeServer is the smallest thing shaped like the real obstacle: a site that serves its
// content only to a client that ran JavaScript. The real one is Vercel's checkpoint; what
// matters for this test is only that a plain request cannot pass and a browser can.
type challengeServer struct {
	*httptest.Server
	challenges atomic.Int64 // how many times the challenge page was served
}

func newChallengeServer(t *testing.T) *challengeServer {
	t.Helper()
	cs := &challengeServer{}
	mux := http.NewServeMux()
	cleared := func(r *http.Request) bool {
		c, err := r.Cookie("cleared")
		return err == nil && c.Value == "1"
	}
	// The origin: a challenge until the client runs its script, then a plain page.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !cleared(r) {
			cs.challenges.Add(1)
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`<html><body>Checkpoint<script>
document.cookie = "cleared=1; path=/"; location.reload();
</script></body></html>`))
			return
		}
		_, _ = w.Write([]byte("<html><body>origin ok</body></html>"))
	})
	mux.HandleFunc("/payload", func(w http.ResponseWriter, r *http.Request) {
		if !cleared(r) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("Forbidden"))
			return
		}
		_, _ = w.Write([]byte("PAYLOAD-OK"))
	})
	mux.HandleFunc("/missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not here"))
	})
	cs.Server = httptest.NewServer(mux)
	t.Cleanup(cs.Close)
	return cs
}

func TestSessionFetchesThroughAClearedPage(t *testing.T) {
	requireChrome(t)
	srv := newChallengeServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s, err := NewSession(ctx, "", 1)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Close()

	status, body, err := s.Fetch(ctx, srv.URL+"/payload")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200 (the challenge should have been cleared first)", status)
	}
	if got := strings.TrimSpace(string(body)); got != "PAYLOAD-OK" {
		t.Errorf("body = %q, want PAYLOAD-OK", got)
	}
}

// The clearance is the expensive part — seconds, against milliseconds for a fetch — so it must
// be paid once per tab and not once per request.
//
// The assertion is "the count stops growing", not "the count is one". Clearing polls for the
// origin becoming readable, so the challenge may legitimately be served more than once WHILE
// clearing; what must not happen is a fresh challenge for every later request. Pinning the
// exact number would make this test fail on a slow machine for a reason that is not a bug.
func TestSessionClearsOnlyOnce(t *testing.T) {
	requireChrome(t)
	srv := newChallengeServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	s, err := NewSession(ctx, "", 1)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Close()

	if _, _, err := s.Fetch(ctx, srv.URL+"/payload"); err != nil {
		t.Fatalf("first Fetch: %v", err)
	}
	afterClearing := srv.challenges.Load()
	if afterClearing == 0 {
		t.Fatal("the server never served a challenge; the test is not testing what it claims")
	}

	for i := 0; i < 3; i++ {
		if _, _, err := s.Fetch(ctx, srv.URL+"/payload"); err != nil {
			t.Fatalf("Fetch %d: %v", i, err)
		}
	}
	if n := srv.challenges.Load(); n != afterClearing {
		t.Errorf("challenge served %d more times across three later fetches; clearance is being re-paid", n-afterClearing)
	}
}

// A status is data, not a failure. Only some statuses may be read as "this posting is gone",
// so collapsing them all into an error would take that decision away from the caller.
func TestSessionReportsNotFoundAsAStatusNotAnError(t *testing.T) {
	requireChrome(t)
	srv := newChallengeServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s, err := NewSession(ctx, "", 1)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Close()

	status, _, err := s.Fetch(ctx, srv.URL+"/missing")
	if err != nil {
		t.Fatalf("Fetch returned an error for a 404: %v", err)
	}
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}
