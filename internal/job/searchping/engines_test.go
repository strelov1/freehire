package searchping

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- Google ---

func googleEngine(t *testing.T, handler http.HandlerFunc) (*GoogleEngine, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	// No token source: the credential exchange is Google's code, and a test that stood
	// up a fake one would be testing oauth2/jwt rather than this package's handling of
	// what the Indexing API answers.
	return &GoogleEngine{client: srv.Client(), budget: 200, endpoint: srv.URL}, srv
}

func TestGooglePublishesOnePerURL(t *testing.T) {
	var got []string
	engine, _ := googleEngine(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("unmarshal: %v", err)
		}
		if payload["type"] != "URL_UPDATED" {
			t.Errorf("type = %q, want URL_UPDATED", payload["type"])
		}
		got = append(got, payload["url"])
		w.WriteHeader(http.StatusOK)
	})

	accepted, err := engine.Announce(context.Background(), []string{"https://x/a", "https://x/b"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(accepted) != 2 || len(got) != 2 {
		t.Fatalf("accepted=%v sent=%v, want two of each", accepted, got)
	}
}

// A 429 means the day is spent, and it is the ONE status that does not itself consume
// quota — so walking the rest of the batch past it would start spending tomorrow's.
func TestGoogleStopsAtQuotaAndKeepsWhatWasAccepted(t *testing.T) {
	calls := 0
	engine, _ := googleEngine(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls > 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"code":429}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	accepted, err := engine.Announce(context.Background(), []string{"https://x/a", "https://x/b", "https://x/c"})

	if err == nil {
		t.Fatal("want an error when the quota is exhausted")
	}
	if calls != 2 {
		t.Fatalf("made %d calls, want 2 — the batch must stop at the 429", calls)
	}
	if len(accepted) != 1 || accepted[0] != "https://x/a" {
		t.Fatalf("accepted = %v, want the one url that succeeded", accepted)
	}
}

func TestGoogleReportsAnUnexpectedStatus(t *testing.T) {
	engine, _ := googleEngine(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("Permission denied. Failed to verify the URL ownership."))
	})

	accepted, err := engine.Announce(context.Background(), []string{"https://x/a"})

	if err == nil {
		t.Fatal("want an error")
	}
	// The 403 a misconfigured service account produces is the failure an operator will
	// actually meet, so the body has to survive into the message.
	if !strings.Contains(err.Error(), "URL ownership") {
		t.Fatalf("error = %v, want it to carry Google's own words", err)
	}
	if len(accepted) != 0 {
		t.Fatalf("accepted = %v, want nothing", accepted)
	}
}

func TestGoogleRejectsACredentialThatIsNotAServiceAccount(t *testing.T) {
	_, err := jwtConfigFromServiceAccountKey([]byte(`{"type":"authorized_user","client_id":"x"}`))

	if err == nil || !strings.Contains(err.Error(), "service_account") {
		t.Fatalf("err = %v, want it to name the wrong credential type", err)
	}
}

// --- IndexNow ---

func TestIndexNowSubmitsTheWholeBatchInOneCall(t *testing.T) {
	calls := 0
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	engine, err := NewIndexNowEngine("https://freehire.me", "abc123", time.Second)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	engine.endpoint = srv.URL
	engine.client = srv.Client()

	accepted, err := engine.Announce(context.Background(), []string{"https://freehire.me/jobs/a", "https://freehire.me/jobs/b"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("made %d calls, want 1 — the protocol takes a batch", calls)
	}
	if len(accepted) != 2 {
		t.Fatalf("accepted %d, want 2", len(accepted))
	}
	if payload["host"] != "freehire.me" {
		t.Fatalf("host = %v, want the origin's host", payload["host"])
	}
	if payload["keyLocation"] != "https://freehire.me/abc123.txt" {
		t.Fatalf("keyLocation = %v", payload["keyLocation"])
	}
}

// 202 is the normal answer to the first submission after a key change, not a failure.
func TestIndexNowTreats202AsAccepted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	engine, _ := NewIndexNowEngine("https://freehire.me", "abc123", time.Second)
	engine.endpoint, engine.client = srv.URL, srv.Client()

	accepted, err := engine.Announce(context.Background(), []string{"https://freehire.me/jobs/a"})

	if err != nil || len(accepted) != 1 {
		t.Fatalf("accepted=%v err=%v, want the url accepted", accepted, err)
	}
}

// The key lives in two places — the static file the site serves and the worker's
// configuration. IndexNow answers a mismatch with a 403 on every submission, which
// reads like a broken integration rather than two copies that drifted apart.
func TestIndexNowVerifyKeyCatchesDrift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("a-different-key\n"))
	}))
	defer srv.Close()

	engine, _ := NewIndexNowEngine(srv.URL, "abc123", time.Second)
	engine.client = srv.Client()

	err := engine.VerifyKey(context.Background())

	if err == nil || !strings.Contains(err.Error(), "different key") {
		t.Fatalf("err = %v, want a mismatch to be named", err)
	}
}

func TestIndexNowVerifyKeyAcceptsAMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Trailing newline is what a text file on disk actually serves.
		_, _ = w.Write([]byte("abc123\n"))
	}))
	defer srv.Close()

	engine, _ := NewIndexNowEngine(srv.URL, "abc123", time.Second)
	engine.client = srv.Client()

	if err := engine.VerifyKey(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A missing key is "this engine is off", not a misconfiguration — the fleet's rollback.
func TestIndexNowIsOffWithoutAKey(t *testing.T) {
	engine, err := NewIndexNowEngine("https://freehire.me", "  ", time.Second)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if engine != nil {
		t.Fatal("want nil engine when no key is configured")
	}
}

func TestGoogleIsOffWithoutAKeyFile(t *testing.T) {
	engine, err := NewGoogleEngine(context.Background(), "", 200, time.Second)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if engine != nil {
		t.Fatal("want nil engine when no credential is configured")
	}
}
