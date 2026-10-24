package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/dict/companyname"
	"github.com/strelov1/freehire/internal/platform/db"
)

// stubResolver returns a canned candidate per board, standing in for a real ATS
// fetch so resolveNames can be exercised without network.
type stubResolver struct{ byBoard map[string]string }

func (s stubResolver) Name(_ context.Context, board string) (string, error) {
	return s.byBoard[board], nil
}

func TestResolveNames(t *testing.T) {
	registry := companyname.Registry{
		"pinpoint": stubResolver{byBoard: map[string]string{
			"afcb":       "AFC Bournemouth",          // accepted (substring)
			"kempinski":  "Elena - Meta Recruitment", // rejected (unrelated)
			"lbresearch": "Centellic",                // rejected (rebrand shares nothing)
		}},
	}
	rows := []db.ListSlugLikeCompaniesForBackfillRow{
		{Slug: "afcb", Name: "afcb", Source: "pinpoint", URL: "https://afcb.pinpointhq.com/x"},
		{Slug: "kempinski", Name: "kempinski", Source: "pinpoint", URL: "https://kempinski.pinpointhq.com/x"},
		{Slug: "lbresearch", Name: "lbresearch", Source: "pinpoint", URL: "https://lbresearch.pinpointhq.com/x"},
		{Slug: "acme", Name: "acme", Source: "unknown-ats", URL: "https://acme.example.com/x"},  // no resolver
		{Slug: "bar", Name: "Bar Inc", Source: "pinpoint", URL: "https://bar.pinpointhq.com/x"}, // not slug-like
	}

	renames, stats := resolveNames(context.Background(), rows, registry)

	want := rename{oldSlug: "afcb", oldName: "afcb", name: "AFC Bournemouth", source: "pinpoint", board: "afcb"}
	if len(renames) != 1 || renames[0] != want {
		t.Fatalf("renames = %+v, want [%+v]", renames, want)
	}
	if stats.noSource != 1 {
		t.Errorf("noSource = %d, want 1", stats.noSource)
	}
	if stats.rejected != 2 {
		t.Errorf("rejected = %d, want 2 (kempinski, lbresearch)", stats.rejected)
	}
}

// TestApplyRenames_CountsFailures guards the worker's exit-code contract: a
// per-company write failure must be counted, not just logged and dropped, so
// the caller can turn it into a non-zero exit via worker.ExitCode and cron
// actually alerts instead of silently succeeding on a partial batch.
func TestApplyRenames_CountsFailures(t *testing.T) {
	renames := []rename{
		{oldSlug: "afcb", name: "AFC Bournemouth"},
		{oldSlug: "kempinski", name: "Kempinski Hotels"},
		{oldSlug: "lbresearch", name: "Centellic"},
	}
	applied, failed := applyRenames(context.Background(), renames, func(_ context.Context, r rename) (bool, error) {
		if r.oldSlug == "kempinski" {
			return false, errors.New("unique constraint violation")
		}
		return true, nil
	})
	if applied != 2 {
		t.Errorf("applied = %d, want 2", applied)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1", failed)
	}
}

// TestApplyRenames_ContinuesPastAFailure guards that one company's write
// error doesn't abort the rest of the batch — every rename must still be
// attempted.
func TestApplyRenames_ContinuesPastAFailure(t *testing.T) {
	renames := []rename{
		{oldSlug: "a", name: "A"},
		{oldSlug: "b", name: "B"},
		{oldSlug: "c", name: "C"},
	}
	var attempted []string
	applyRenames(context.Background(), renames, func(_ context.Context, r rename) (bool, error) {
		attempted = append(attempted, r.oldSlug)
		if r.oldSlug == "a" {
			return false, errors.New("boom")
		}
		return true, nil
	})
	if len(attempted) != 3 {
		t.Errorf("attempted %d renames, want 3 (a failing must not stop b, c)", len(attempted))
	}
}

// fakeRenamer records the writes writeRename makes, in order, and can fail the board
// write.
type fakeRenamer struct {
	calls    []string
	boardErr error
}

func (f *fakeRenamer) RenameSlugBoardCompany(_ context.Context, arg db.RenameSlugBoardCompanyParams) (int64, error) {
	f.calls = append(f.calls, "board "+arg.Provider+"/"+arg.Board+" "+arg.OldName+"->"+arg.Name)
	return 1, f.boardErr
}

func (f *fakeRenamer) RenameSlugCompany(_ context.Context, arg db.RenameSlugCompanyParams) (int64, error) {
	f.calls = append(f.calls, "jobs "+arg.OldSlug+"->"+arg.NewSlug)
	return 1, nil
}

// TestWriteRename_BoardBeforeJobs guards why the rename sticks at all. The adapter copies
// boards.company onto every posting on each crawl, so a rename written to jobs alone was
// undone by the next one. The board goes FIRST: if the jobs write then fails, the next
// crawl carries the name from the board anyway.
func TestWriteRename_BoardBeforeJobs(t *testing.T) {
	f := &fakeRenamer{}
	r := rename{oldSlug: "a-place-for-mom", oldName: "a-place-for-mom", name: "A Place for Mom", source: "ashby", board: "a-place-for-mom"}

	if _, err := writeRename(context.Background(), f, r); err != nil {
		t.Fatalf("writeRename: %v", err)
	}
	want := []string{"board ashby/a-place-for-mom a-place-for-mom->A Place for Mom", "jobs a-place-for-mom->a-place-for-mom"}
	if len(f.calls) != 2 || f.calls[0] != want[0] || f.calls[1] != want[1] {
		t.Fatalf("calls = %q, want %q", f.calls, want)
	}
}

// TestWriteRename_BoardFailureLeavesJobs guards the opposite order's trap: jobs renamed
// while the board still holds the slug make the company stop looking slug-like, so no
// later run lists it — and the next crawl restores the slug for good.
func TestWriteRename_BoardFailureLeavesJobs(t *testing.T) {
	f := &fakeRenamer{boardErr: errors.New("boom")}
	r := rename{oldSlug: "airgarage", oldName: "airgarage", name: "AirGarage", source: "ashby", board: "airgarage"}

	if _, err := writeRename(context.Background(), f, r); err == nil {
		t.Fatal("writeRename: want the board error, got nil")
	}
	if len(f.calls) != 1 {
		t.Fatalf("calls = %q, want only the board write", f.calls)
	}
}

// TestEachPerSource_BoundsEachSource guards the 429s the first prod run drew: no source may
// see more than perSourceConcurrency fetches at once, whatever the others are doing, and every
// item is still visited.
func TestEachPerSource_BoundsEachSource(t *testing.T) {
	type item struct{ source string }
	var items []item
	for range 40 {
		items = append(items, item{"join"}, item{"lever"})
	}

	var (
		mu       sync.Mutex
		inFlight = map[string]int{}
		peak     = map[string]int{}
		visited  int
	)
	eachPerSource(items, func(it item) string { return it.source }, func(it item) {
		mu.Lock()
		inFlight[it.source]++
		peak[it.source] = max(peak[it.source], inFlight[it.source])
		mu.Unlock()
		time.Sleep(time.Millisecond)
		mu.Lock()
		inFlight[it.source]--
		visited++
		mu.Unlock()
	})

	if visited != len(items) {
		t.Errorf("visited %d items, want %d", visited, len(items))
	}
	for src, p := range peak {
		if p > perSourceConcurrency {
			t.Errorf("%s peaked at %d concurrent fetches, want at most %d", src, p, perSourceConcurrency)
		}
	}
}
