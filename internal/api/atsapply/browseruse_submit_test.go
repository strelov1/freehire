package atsapply

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/platform/browseruse"
)

// fakeAshbyFetcher returns a fixed, minimal schema: one required text field that
// answerKeyFor already knows how to resolve, so the plan built from it is fully
// resolved without needing an LLM drafter.
type fakeAshbyFetcher struct{}

func (fakeAshbyFetcher) Fetch(ctx context.Context, c applyform.Claimed) (applyform.Form, error) {
	return applyform.Form{Provider: "ashby", Fields: []applyform.Field{
		{ID: "email", Label: "Email", Type: applyform.TypeText, Required: true},
	}}, nil
}

// fakeAshbyFetcherWithCustomQuestion adds one required question no known answer or
// drafter can resolve, so the resulting plan is never fully resolved.
type fakeAshbyFetcherWithCustomQuestion struct{}

func (fakeAshbyFetcherWithCustomQuestion) Fetch(ctx context.Context, c applyform.Claimed) (applyform.Form, error) {
	return applyform.Form{Provider: "ashby", Fields: []applyform.Field{
		{ID: "email", Label: "Email", Type: applyform.TypeText, Required: true},
		{ID: "custom_1", Label: "Why do you want to work here?", Type: applyform.TypeText, Required: true},
	}}, nil
}

// newFakeBrowserUseServer starts an httptest.Server implementing just enough of the v4
// API for a single run to go straight to "completed" with the given result text, and
// counts how many run-creation requests it received.
func newFakeBrowserUseServer(t *testing.T, resultText string) (url string, calls *int) {
	t.Helper()
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/runs":
			n++
			_, _ = w.Write([]byte(`{"id":"run-1","status":"queued"}`))
		case r.URL.Path == "/runs/run-1/status":
			_, _ = w.Write([]byte(`{"status":"completed"}`))
		case r.URL.Path == "/runs/run-1":
			encoded, _ := json.Marshal(resultText)
			_, _ = w.Write([]byte(`{"id":"run-1","status":"completed","result":` + string(encoded) + `,"totalCostUsd":"0.01"}`))
		default:
			t.Fatalf("unexpected browser-use request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &n
}

func newTestBrowserUseExecutor(baseURL string) *BrowserUseExecutor {
	e := NewBrowserUseExecutor(browseruse.New("test-key", baseURL, nil))
	return e
}

func TestSubmit_BrowserUseFallback_FullyResolvedAshbyExecutesAndConfirms(t *testing.T) {
	t.Setenv("AUTO_APPLY_BROWSERUSE_ENFORCE", "1")
	url, calls := newFakeBrowserUseServer(t, "All done.\nCONFIRMED: Thanks for applying!")

	c := (&Client{fetchers: map[string]applyform.Fetcher{"ashby": fakeAshbyFetcher{}}}).
		WithBrowserUse(newTestBrowserUseExecutor(url))

	result, err := c.Submit(context.Background(), autoapply.Claimed{
		Provider: "ashby", JobURL: "https://jobs.ashbyhq.com/example/123",
	}, map[string]string{"email": "ada@example.com"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Status != autoapply.StatusApplied {
		t.Fatalf("result = %+v, want applied", result)
	}
	if *calls != 1 {
		t.Errorf("browser-use run-creation calls = %d, want exactly 1", *calls)
	}
}

func TestSubmit_BrowserUseFallback_UnresolvedPlanNeverCallsBrowserUse(t *testing.T) {
	t.Setenv("AUTO_APPLY_BROWSERUSE_ENFORCE", "1")
	url, calls := newFakeBrowserUseServer(t, "CONFIRMED: should never be seen")

	c := (&Client{fetchers: map[string]applyform.Fetcher{"ashby": fakeAshbyFetcherWithCustomQuestion{}}}).
		WithBrowserUse(newTestBrowserUseExecutor(url))

	// No answer for custom_1 -> the plan can never fully resolve.
	result, err := c.Submit(context.Background(), autoapply.Claimed{Provider: "ashby"}, map[string]string{"email": "ada@example.com"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Status != autoapply.StatusParked || len(result.Unmapped) == 0 {
		t.Fatalf("result = %+v, want parked with an unmapped field", result)
	}
	if *calls != 0 {
		t.Errorf("browser-use run-creation calls = %d, want 0 for an unresolved plan", *calls)
	}
}

func TestSubmit_BrowserUseFallback_ShadowModeNeverCallsBrowserUse(t *testing.T) {
	t.Setenv("AUTO_APPLY_BROWSERUSE_ENFORCE", "") // shadow (default)
	url, calls := newFakeBrowserUseServer(t, "CONFIRMED: should never be seen")

	c := (&Client{fetchers: map[string]applyform.Fetcher{"ashby": fakeAshbyFetcher{}}}).
		WithBrowserUse(newTestBrowserUseExecutor(url))

	result, err := c.Submit(context.Background(), autoapply.Claimed{Provider: "ashby"}, map[string]string{"email": "ada@example.com"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Status != autoapply.StatusParked || result.Reason != reasonSubmissionNotImplemented {
		t.Fatalf("result = %+v, want parked/%s (shadow mode still parks)", result, reasonSubmissionNotImplemented)
	}
	if *calls != 0 {
		t.Errorf("browser-use run-creation calls = %d, want 0 in shadow mode", *calls)
	}
}

// A run that was created (so it may already be interacting with the live employer form)
// but then errors mid-flight — found by code review: this must map to StatusUnconfirmed,
// not an ordinary retryable error, exactly like an unconfirmed chromedp submission. The
// caller's own context deadline (RunOptions.CallTimeout) is the realistic trigger in
// production, since it is shorter than this executor's own internal Wait timeout.
func TestSubmit_BrowserUseFallback_ARunThatErrorsMidFlightIsUnconfirmedNotRetryable(t *testing.T) {
	t.Setenv("AUTO_APPLY_BROWSERUSE_ENFORCE", "1")
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/runs":
			calls++
			_, _ = w.Write([]byte(`{"id":"run-1","status":"queued"}`))
		case r.URL.Path == "/runs/run-1/status":
			// The run WAS created — the agent may already be on the live page — but the
			// status poll itself now fails (a transport hiccup, or the caller's own
			// context deadline firing, which Wait surfaces the same way).
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected browser-use request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	c := (&Client{fetchers: map[string]applyform.Fetcher{"ashby": fakeAshbyFetcher{}}}).
		WithBrowserUse(newTestBrowserUseExecutor(srv.URL))

	result, err := c.Submit(context.Background(), autoapply.Claimed{Provider: "ashby"}, map[string]string{"email": "ada@example.com"})
	if err != nil {
		t.Fatalf("Submit: %v, want a nil error paired with StatusUnconfirmed — never a plain retryable error once a run has been created", err)
	}
	if result.Status != autoapply.StatusUnconfirmed {
		t.Fatalf("result = %+v, want unconfirmed", result)
	}
	if calls != 1 {
		t.Errorf("browser-use run-creation calls = %d, want exactly 1", calls)
	}
}

func TestSubmit_BrowserUseFallback_UnconfiguredClientParksAsBefore(t *testing.T) {
	t.Setenv("AUTO_APPLY_BROWSERUSE_ENFORCE", "1")
	// No WithBrowserUse call at all — c.browserUse stays nil.
	c := &Client{fetchers: map[string]applyform.Fetcher{"ashby": fakeAshbyFetcher{}}}

	result, err := c.Submit(context.Background(), autoapply.Claimed{Provider: "ashby"}, map[string]string{"email": "ada@example.com"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Status != autoapply.StatusParked || result.Reason != reasonSubmissionNotImplemented {
		t.Fatalf("result = %+v, want parked/%s", result, reasonSubmissionNotImplemented)
	}
}
