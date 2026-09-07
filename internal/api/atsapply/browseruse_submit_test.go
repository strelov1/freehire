package atsapply

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/candidate/cv"
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

// fakeAshbyFetcherWithResume adds Ashby's own résumé field (real id/label convention —
// see internal/ingest/applyform/ashby.go and internal/ingest/applyform/ashby_test.go's own
// fixture) to the minimal schema, so the resulting plan needs a rendered CV to resolve.
type fakeAshbyFetcherWithResume struct{}

func (fakeAshbyFetcherWithResume) Fetch(ctx context.Context, c applyform.Claimed) (applyform.Form, error) {
	return applyform.Form{Provider: "ashby", Fields: []applyform.Field{
		{ID: "email", Label: "Email", Type: applyform.TypeText, Required: true},
		{ID: "_systemfield_resume", Label: "Resume", Type: applyform.TypeFile, Required: true},
	}}, nil
}

// newFakeBrowserUseServerWithWorkspace extends newFakeBrowserUseServer's shape with the
// workspace/file-upload endpoints attachResumeIfPresent calls, and captures the run
// creation body plus the uploaded file's bytes so a test can assert on both.
func newFakeBrowserUseServerWithWorkspace(t *testing.T, resultText string) (url string, runBody *map[string]any, uploadedBytes *[]byte, workspaceDeleted *bool) {
	t.Helper()
	runBody = &map[string]any{}
	uploadedBytes = &[]byte{}
	workspaceDeleted = new(bool)
	var uploadServerURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/workspaces", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method for /workspaces: %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"id":"ws-1","archived":false,"createdAt":"2026-09-07T00:00:00Z","updatedAt":"2026-09-07T00:00:00Z"}`))
	})
	mux.HandleFunc("/workspaces/ws-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("unexpected method for /workspaces/ws-1: %s", r.Method)
		}
		*workspaceDeleted = true
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/workspaces/ws-1/files/upload", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"files":[{"id":"file-1","name":"resume.pdf","storedName":"resume.pdf","path":"uploads/resume.pdf","willOverride":false,"uploadUrl":"` + uploadServerURL + `"}]}`))
	})
	mux.HandleFunc("/upload-target", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*uploadedBytes = b
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/runs", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(runBody)
		_, _ = w.Write([]byte(`{"id":"run-1","status":"queued"}`))
	})
	mux.HandleFunc("/runs/run-1/status", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"completed"}`))
	})
	mux.HandleFunc("/runs/run-1", func(w http.ResponseWriter, r *http.Request) {
		encoded, _ := json.Marshal(resultText)
		_, _ = w.Write([]byte(`{"id":"run-1","status":"completed","result":` + string(encoded) + `,"totalCostUsd":"0.01"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	uploadServerURL = srv.URL + "/upload-target"
	return srv.URL, runBody, uploadedBytes, workspaceDeleted
}

// A résumé/CV field no longer disqualifies Ashby/Workable from the browser-use fallback:
// the rendered PDF (attachApprovedResume — the same rendering the chromedp/Greenhouse path
// already uses) is uploaded into a fresh browser-use workspace and attached to the run,
// which is then deleted once the run is done.
func TestSubmit_BrowserUseFallback_UploadsAndAttachesTheApprovedResume(t *testing.T) {
	t.Setenv("AUTO_APPLY_BROWSERUSE_ENFORCE", "1")
	url, runBody, uploadedBytes, workspaceDeleted := newFakeBrowserUseServerWithWorkspace(t, "All done.\nCONFIRMED: Thanks for applying!")

	c := (&Client{
		fetchers: map[string]applyform.Fetcher{"ashby": fakeAshbyFetcherWithResume{}},
		cvs:      fakeCVReader{rec: cv.Record{}},
		renderer: fakeCVRenderer{pdf: []byte("%PDF-1.4 fake resume")},
	}).WithBrowserUse(newTestBrowserUseExecutor(url))

	result, err := c.Submit(context.Background(), autoapply.Claimed{
		Provider: "ashby", JobURL: "https://jobs.ashbyhq.com/example/123", TailoredCVID: uuid.New(),
	}, map[string]string{"email": "ada@example.com"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if result.Status != autoapply.StatusApplied {
		t.Fatalf("result = %+v, want applied", result)
	}
	if (*runBody)["workspaceId"] != "ws-1" {
		t.Errorf("run body workspaceId = %v, want ws-1", (*runBody)["workspaceId"])
	}
	ids, _ := (*runBody)["attachedFileIds"].([]any)
	if len(ids) != 1 || ids[0] != "file-1" {
		t.Errorf("run body attachedFileIds = %v, want [file-1]", (*runBody)["attachedFileIds"])
	}
	if string(*uploadedBytes) != "%PDF-1.4 fake resume" {
		t.Errorf("uploaded bytes = %q, want the rendered PDF's own bytes", *uploadedBytes)
	}
	if !*workspaceDeleted {
		t.Error("workspace was never deleted after the run — a résumé must not outlive its one attempt")
	}
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
//
// It must also not let the run's cost vanish from the spend guard: CreateRun already
// started billable work, so the executor separately fetches the run's cost-so-far
// (recordSpendBestEffort) once Wait itself has given up — the /runs/run-1 handler below is
// that read, distinct from the /runs/run-1/status poll Wait itself fails on.
func TestSubmit_BrowserUseFallback_ARunThatErrorsMidFlightIsUnconfirmedNotRetryable(t *testing.T) {
	t.Setenv("AUTO_APPLY_BROWSERUSE_ENFORCE", "1")
	calls, resultCalls := 0, 0
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
		case r.URL.Path == "/runs/run-1":
			// recordSpendBestEffort's own read, made with a fresh context after Wait has
			// already failed — the run is still genuinely in flight from the API's own
			// point of view, cost included.
			resultCalls++
			_, _ = w.Write([]byte(`{"id":"run-1","status":"running","totalCostUsd":"0.05"}`))
		default:
			t.Fatalf("unexpected browser-use request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	executor := newTestBrowserUseExecutor(srv.URL)
	c := (&Client{fetchers: map[string]applyform.Fetcher{"ashby": fakeAshbyFetcher{}}}).
		WithBrowserUse(executor)

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
	if resultCalls != 1 {
		t.Errorf("browser-use result-fetch calls = %d, want exactly 1 — the run's cost must still be looked up after Wait fails", resultCalls)
	}
	if executor.spend.spentUSD != 0.05 {
		t.Errorf("spend.spentUSD = %v, want 0.05 — the run's cost must reach the spend guard even though Wait itself failed", executor.spend.spentUSD)
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
