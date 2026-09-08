package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/search/search"
)

// sequencedSearcher returns a different result/error on each successive Search call, so a
// test can drive the retry runJobSearch attempts on a filter-classified rejection.
type sequencedSearcher struct {
	calls   int
	results []search.SearchResult
	errs    []error
	got     []search.SearchParams
}

func (s *sequencedSearcher) Search(_ context.Context, p search.SearchParams) (search.SearchResult, error) {
	i := s.calls
	s.calls++
	s.got = append(s.got, p)
	var res search.SearchResult
	if i < len(s.results) {
		res = s.results[i]
	}
	var err error
	if i < len(s.errs) {
		err = s.errs[i]
	}
	return res, err
}

// A filter Meilisearch rejects degrades to a widened, successful result — the retry drops
// the filter entirely, and every filter param the request carried is reported.
func TestSearchJobs_AFilterRejectionDegradesToAWidenedResult(t *testing.T) {
	seq := &sequencedSearcher{
		errs:    []error{search.ErrBadQuery},
		results: []search.SearchResult{{}, {Total: 3}},
	}
	app := searchApp(seq)

	status, body := doGet(t, app, "/jobs/search?work_mode=remote")

	if status != 200 {
		t.Fatalf("status = %d, want 200", status)
	}
	if seq.calls != 2 {
		t.Fatalf("Search calls = %d, want 2 (primary + retry)", seq.calls)
	}
	if seq.got[1].Filter != nil {
		t.Errorf("retry Filter = %v, want nil (dropped)", seq.got[1].Filter)
	}
	meta, _ := body["meta"].(map[string]any)
	ignored, _ := meta["ignored_params"].([]any)
	if len(ignored) != 1 {
		t.Fatalf("ignored_params = %v, want exactly one entry", ignored)
	}
	entry, _ := ignored[0].(map[string]any)
	if entry["param"] != "work_mode" {
		t.Errorf("ignored_params[0].param = %v, want work_mode", entry["param"])
	}
}

// A request with no dynamic filter never retries, even if the engine somehow errors with
// ErrBadQuery (e.g. a bad sort attribute) — there is nothing a dropped filter would fix.
func TestSearchJobs_NoFilterNeverRetries(t *testing.T) {
	seq := &sequencedSearcher{errs: []error{search.ErrBadQuery}}
	app := searchApp(seq)

	status, _ := doGet(t, app, "/jobs/search")

	if status != 400 {
		t.Errorf("status = %d, want 400 (ErrBadQuery's own mapping)", status)
	}
	if seq.calls != 1 {
		t.Errorf("Search calls = %d, want 1 — no filter to blame, no retry", seq.calls)
	}
}

// A filtered request whose failure is NOT classified as a filter rejection (a genuine
// engine/transport fault) must not retry or degrade — it fails exactly as it always has.
func TestSearchJobs_ANonFilterFailureIsNotDegraded(t *testing.T) {
	seq := &sequencedSearcher{errs: []error{errors.New("meilisearch: connection refused")}}
	app := searchApp(seq)

	status, _ := doGet(t, app, "/jobs/search?work_mode=remote")

	if status != 500 {
		t.Errorf("status = %d, want 500", status)
	}
	if seq.calls != 1 {
		t.Errorf("Search calls = %d, want 1 — not a filter rejection, nothing to retry", seq.calls)
	}
}

// When the retry ALSO fails, the request reports a failure — not a degraded success — and
// the engine was asked exactly twice.
func TestSearchJobs_ARetryThatAlsoFailsStillFails(t *testing.T) {
	seq := &sequencedSearcher{errs: []error{search.ErrBadQuery, errors.New("meilisearch: still down")}}
	app := searchApp(seq)

	status, _ := doGet(t, app, "/jobs/search?work_mode=remote")

	if status != 500 {
		t.Errorf("status = %d, want 500", status)
	}
	if seq.calls != 2 {
		t.Errorf("Search calls = %d, want 2 (primary + one retry, no more)", seq.calls)
	}
}
