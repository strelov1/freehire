package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/sources"
)

// TestRunFillsCompanyDescriptionOncePerBoard proves the runner calls a
// CompanyDescriber adapter's CompanyDescription exactly once per board regardless of
// how many postings it carries, and forwards a non-empty result to the Store's
// RecordCompanyDescription, keyed by the board's company slug.
func TestRunFillsCompanyDescriptionOncePerBoard(t *testing.T) {
	src := &fakeDescriberSource{
		fakeSource: fakeSource{provider: "greenhouse", jobs: []sources.Job{
			{ExternalID: "1", Title: "A", Company: "Coinbase", URL: "u"},
			{ExternalID: "2", Title: "B", Company: "Coinbase", URL: "u"},
		}},
		description: "Ready to be pushed beyond what you think you're capable of?",
	}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Coinbase", Provider: "greenhouse", Board: "coinbase"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Total().Ingested != 2 {
		t.Fatalf("Ingested = %d, want 2 (the company-description fetch must not affect job ingest)", stats.Total().Ingested)
	}
	if src.descCalls != 1 {
		t.Fatalf("CompanyDescription called %d times, want exactly 1 (once per board, not per posting)", src.descCalls)
	}
	if len(store.filled) != 1 {
		t.Fatalf("RecordCompanyDescription called %d times, want 1", len(store.filled))
	}
	want := filledDescription{slug: normalize.CompanySlug("Coinbase"), name: "Coinbase", description: src.description}
	if store.filled[0] != want {
		t.Errorf("filled = %+v, want %+v", store.filled[0], want)
	}
}

// TestRunRecordsAnEmptyCompanyDescription proves an empty CompanyDescription result
// (the employer left the field blank) is still RECORDED: the fetch answered, so the
// Store must stamp the attempt — otherwise a board with a blank "about us" is asked
// again on every run, which is exactly the request volume the refresh window removes.
func TestRunRecordsAnEmptyCompanyDescription(t *testing.T) {
	src := &fakeDescriberSource{
		fakeSource: fakeSource{provider: "greenhouse", jobs: []sources.Job{
			{ExternalID: "1", Title: "A", Company: "C", URL: "u"},
		}},
		description: "",
	}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "greenhouse", Board: "c"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := filledDescription{slug: normalize.CompanySlug("C"), name: "C", description: ""}
	if len(store.filled) != 1 || store.filled[0] != want {
		t.Errorf("recorded = %+v, want exactly %+v (an answered fetch is stamped even when blank)", store.filled, want)
	}
}

// TestRunCompanyDescriptionUsesTheCanonicalSlug proves the due-check and the write both go
// through canonicalOf, like the posting upsert. A board whose company a merge retired must
// not insert a 0-job row under the retired slug — that row would shadow the alias redirect —
// and the canonical company is the one that should get the description.
func TestRunCompanyDescriptionUsesTheCanonicalSlug(t *testing.T) {
	src := &fakeDescriberSource{
		fakeSource:  fakeSource{provider: "workable", jobs: []sources.Job{{ExternalID: "1", Title: "A", Company: "Dollartree", URL: "u"}}},
		description: "",
	}
	store := &fakeStore{}
	aliases := &fakeAliases{canon: map[string]string{"dollartree": "dollar-tree"}}
	r := Runner{Registry: registry(src), Store: store, Aliases: aliases}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Dollartree", Provider: "workable", Board: "dollartree"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := filledDescription{slug: "dollar-tree", name: "Dollartree", description: ""}
	if len(store.filled) != 1 || store.filled[0] != want {
		t.Errorf("recorded = %+v, want exactly %+v (the canonical slug, not the retired one)", store.filled, want)
	}
}

// TestRunSkipsCompanyDescriptionWhenNotDue proves the runner asks the Store first and
// makes NO request when the company is not due — it already holds a description, or it
// was checked inside the refresh window. Measured on prod 2026-10-19, fetching it on
// every run was half of Workable's ~63k requests a day and half of its 18,520 refusals
// in two days.
func TestRunSkipsCompanyDescriptionWhenNotDue(t *testing.T) {
	src := &fakeDescriberSource{
		fakeSource: fakeSource{provider: "workable", jobs: []sources.Job{
			{ExternalID: "1", Title: "A", Company: "C", URL: "u"},
		}},
		description: "About us",
	}
	store := &fakeStore{descNotDue: true}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "workable", Board: "c"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Total().Ingested != 1 {
		t.Fatalf("Ingested = %d, want 1", stats.Total().Ingested)
	}
	if src.descCalls != 0 {
		t.Errorf("CompanyDescription called %d times for a company that is not due, want 0", src.descCalls)
	}
	if len(store.filled) != 0 {
		t.Errorf("recorded %v for a company that is not due, want nothing", store.filled)
	}
}

// TestRunAsksDueWithTheRefreshWindow pins the bound the runner hands the Store: a
// company checked more recently than CompanyDescriptionRefreshWindow ago is not due.
func TestRunAsksDueWithTheRefreshWindow(t *testing.T) {
	src := &fakeDescriberSource{fakeSource: fakeSource{provider: "workable"}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	before := time.Now()
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "workable", Board: "c"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	after := time.Now()
	if len(store.dueAsked) != 1 {
		t.Fatalf("CompanyDescriptionDue called %d times, want 1", len(store.dueAsked))
	}
	got := store.dueAsked[0]
	lo, hi := before.Add(-CompanyDescriptionRefreshWindow), after.Add(-CompanyDescriptionRefreshWindow)
	if got.Before(lo) || got.After(hi) {
		t.Errorf("checkedSince = %v, want now - %v (between %v and %v)", got, CompanyDescriptionRefreshWindow, lo, hi)
	}
}

// TestRunDueCheckErrorSkipsTheFetch proves a failed due-check costs no request and does
// not fail the board: the description is optional, and a database hiccup must not turn
// into a request per board against a rate-limited platform.
func TestRunDueCheckErrorSkipsTheFetch(t *testing.T) {
	src := &fakeDescriberSource{
		fakeSource: fakeSource{provider: "workable", jobs: []sources.Job{
			{ExternalID: "1", Title: "A", Company: "C", URL: "u"},
		}},
		description: "About us",
	}
	store := &fakeStore{dueErr: errors.New("connection reset")}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "workable", Board: "c"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Total().Ingested != 1 || stats.Total().Failed != 0 {
		t.Fatalf("stats = %+v, want Ingested=1 Failed=0", stats.Total())
	}
	if src.descCalls != 0 {
		t.Errorf("CompanyDescription called %d times after a failed due-check, want 0", src.descCalls)
	}
}

// TestRunCompanyDescriptionErrorDoesNotFailTheBoard proves a CompanyDescription
// error is swallowed (logged, not propagated): the board's own job ingest still
// succeeds, and no company-info write happens for that board.
func TestRunCompanyDescriptionErrorDoesNotFailTheBoard(t *testing.T) {
	src := &fakeDescriberSource{
		fakeSource: fakeSource{provider: "greenhouse", jobs: []sources.Job{
			{ExternalID: "1", Title: "A", Company: "C", URL: "u"},
		}},
		descriptionErr: errors.New("board metadata endpoint unreachable"),
	}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "greenhouse", Board: "c"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Total().Ingested != 1 || stats.Total().Failed != 0 {
		t.Fatalf("stats = %+v, want Ingested=1 Failed=0 — a company-description error must not fail the board", stats.Total())
	}
	if len(store.filled) != 0 {
		t.Errorf("RecordCompanyDescription called despite a CompanyDescription error: %v", store.filled)
	}
}

// TestRunSourceWithoutCompanyDescriberIsUnaffected proves an adapter that does not
// implement CompanyDescriber (the common case) ingests normally with no company-info
// write attempted at all.
func TestRunSourceWithoutCompanyDescriberIsUnaffected(t *testing.T) {
	src := fakeSource{provider: "lever", jobs: []sources.Job{
		{ExternalID: "1", Title: "A", Company: "C", URL: "u"},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "lever", Board: "c"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Total().Ingested != 1 {
		t.Fatalf("Ingested = %d, want 1", stats.Total().Ingested)
	}
	if len(store.filled) != 0 {
		t.Errorf("RecordCompanyDescription called for a non-CompanyDescriber adapter: %v", store.filled)
	}
}
