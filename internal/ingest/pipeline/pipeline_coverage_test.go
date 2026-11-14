package pipeline

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// fakeCoverageGatedSource implements sources.CoverageGated on top of the hydrating fake. It
// records the companies the probe was asked about and the answer it got, so a test can prove
// the runner routed to the gated path and handed it a resolver that actually resolves.
type fakeCoverageGatedSource struct {
	fakeHydratingSource
	gatedCalled  bool
	askedAbout   []string
	coveredBack  map[string]bool
	rejectedBack []string // titles the gate said the catalogue filter will reject
	probeReturns bool     // set once the probe has answered, so nil vs empty is distinguishable
}

func (f *fakeCoverageGatedSource) FetchNewGated(_ context.Context, _ sources.CompanyEntry,
	seen func(string) bool, gate sources.DetailGate) ([]sources.Job, error) {
	f.gatedCalled = true
	for _, j := range f.jobs {
		f.askedAbout = append(f.askedAbout, j.Company)
		if gate.Rejected(j) {
			f.rejectedBack = append(f.rejectedBack, j.Title)
		}
	}
	f.coveredBack = gate.Covered(f.askedAbout)
	f.probeReturns = true

	out := make([]sources.Job, len(f.jobs))
	for i, j := range f.jobs {
		j.SeenRefresh = seen(j.ExternalID)
		out[i] = j
	}
	return out, nil
}

// TestRunPrefersCoverageGatedFetchForAnAggregatorBoard proves the runner routes a
// CoverageGated adapter through FetchNewGated and hands it a resolver keyed by the company
// NAMES it asked about — the whole point being that the adapter learns a posting is doomed
// before it pays for that posting's detail.
func TestRunPrefersCoverageGatedFetchForAnAggregatorBoard(t *testing.T) {
	src := &fakeCoverageGatedSource{fakeHydratingSource: fakeHydratingSource{
		provider: "himalayas",
		jobs: []sources.Job{
			{ExternalID: "1", Title: "Backend Engineer", Company: "Acme", Description: "<p>x</p>"},
			{ExternalID: "2", Title: "Backend Engineer", Company: "Beta Corp", Description: "<p>y</p>"},
		},
	}}
	store := &fakeStore{}
	coverage := &fakeCoverage{covered: map[string]bool{"acme": true}}
	r := Runner{Registry: registry(src), Store: store, Coverage: coverage}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Himalayas", Provider: "himalayas"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !src.gatedCalled {
		t.Fatal("FetchNewGated was not called — a CoverageGated adapter must take the gated path")
	}
	if src.fetchNewCalled {
		t.Error("FetchNew was also called; the gated path replaces it, it does not precede it")
	}
	// Keyed by the name the adapter passed, not by the slug the lookup speaks.
	if !src.coveredBack["Acme"] {
		t.Errorf("covered = %v, want Acme covered under its own name", src.coveredBack)
	}
	if src.coveredBack["Beta Corp"] {
		t.Errorf("covered = %v, want Beta Corp uncovered", src.coveredBack)
	}
}

// The gate also carries the catalogue filter's answer, so a CoverageGated adapter does not buy
// the body of a posting the filter is about to reject — and the answer is the write path's own,
// so the rejected posting is still counted where every rejection is.
func TestCoverageGatedFetchIsToldWhatTheCatalogueFilterRejects(t *testing.T) {
	src := &fakeCoverageGatedSource{fakeHydratingSource: fakeHydratingSource{
		provider: "himalayas",
		jobs: []sources.Job{
			{ExternalID: "1", Title: "Backend Engineer", Company: "Acme", URL: "https://example.com/1"},
			{ExternalID: "2", Title: refusedTitle, Company: "Acme", URL: "https://example.com/2"},
		},
	}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, Coverage: &fakeCoverage{covered: map[string]bool{}}}
	stats, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Himalayas", Provider: "himalayas"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(src.rejectedBack) != 1 || src.rejectedBack[0] != refusedTitle {
		t.Errorf("gate rejected %v, want only the social worker posting", src.rejectedBack)
	}
	if got := stats.Total(); got.Rejected != 1 || got.Ingested != 1 {
		t.Errorf("stats = %+v, want the rejection counted by the pipeline and the engineer ingested", got)
	}
}

// The probe answers a board's companies in ONE lookup, not one per posting — the cost the
// gated path saves must not reappear as a lookup storm.
func TestCoverageProbeAsksOnceForTheWholeBoard(t *testing.T) {
	src := &fakeCoverageGatedSource{fakeHydratingSource: fakeHydratingSource{
		provider: "himalayas",
		jobs: []sources.Job{
			{ExternalID: "1", Title: "Backend Engineer", Company: "Acme", Description: "<p>x</p>"},
			{ExternalID: "2", Title: "Frontend Engineer", Company: "Acme", Description: "<p>y</p>"},
			{ExternalID: "3", Title: "Backend Engineer", Company: "Beta Corp", Description: "<p>z</p>"},
		},
	}}
	coverage := &fakeCoverage{covered: map[string]bool{}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, Coverage: coverage}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Himalayas", Provider: "himalayas"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(coverage.calls) == 0 {
		t.Fatal("coverage was never consulted")
	}
	if got := coverage.calls[0]; len(got) != 2 {
		t.Errorf("first probe asked about %v, want the 2 DISTINCT companies", got)
	}
}

// The probe resolves a board's companies through the alias registry in ONE batch, too. It
// used to resolve them one company at a time: shine lists ~62k distinct employers, the crawl
// host sits ~36 ms from the database, and 62k sequential round trips ate the whole 40-minute
// crawl budget before a single posting page was requested — every run died with nothing stored.
func TestCoverageProbeResolvesAliasesInOneBatch(t *testing.T) {
	src := &fakeCoverageGatedSource{fakeHydratingSource: fakeHydratingSource{
		provider: "himalayas",
		jobs: []sources.Job{
			{ExternalID: "1", Title: "Backend Engineer", Company: "DollarTree", Description: "<p>x</p>"},
			{ExternalID: "2", Title: "Frontend Engineer", Company: "Acme", Description: "<p>y</p>"},
			{ExternalID: "3", Title: "Backend Engineer", Company: "Beta Corp", Description: "<p>z</p>"},
		},
	}}
	coverage := &fakeCoverage{covered: map[string]bool{"dollar-tree": true}}
	aliases := &fakeAliases{canon: map[string]string{"dollartree": "dollar-tree"}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, Coverage: coverage, Aliases: aliases}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Himalayas", Provider: "himalayas"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !src.coveredBack["DollarTree"] {
		t.Errorf("covered = %v, want DollarTree covered through its canonical slug", src.coveredBack)
	}
	for i, call := range aliases.calls {
		if len(call) == 1 {
			t.Errorf("alias call %d asked about one company %v; the probe must resolve the board in one batch (calls: %v)",
				i, call, aliases.calls)
		}
	}
}

// A provider the gate does not apply to must keep the plain hydrating path: handing it a
// resolver that can only answer "nothing is covered" would be a lie dressed as an answer.
func TestCoverageGatedFallsBackToFetchNewWhenTheGateDoesNotApply(t *testing.T) {
	// greenhouse is an ATS, not an aggregator, so aggregatorGate declines.
	src := &fakeCoverageGatedSource{fakeHydratingSource: fakeHydratingSource{
		provider: "greenhouse",
		jobs:     []sources.Job{{ExternalID: "1", Title: "Backend Engineer", Company: "Acme", Description: "<p>x</p>"}},
	}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, Coverage: &fakeCoverage{}}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if src.gatedCalled {
		t.Error("FetchNewGated was called for a non-aggregator board; the gate does not apply there")
	}
	if !src.fetchNewCalled {
		t.Error("FetchNew was not called; a CoverageGated adapter is still a HydratingSource")
	}
}

// A failed probe must not read as "everything is covered" — that would starve the catalogue of
// bodies over a transient lookup failure. It reads as "nothing is covered": the run costs what it
// used to, which is the recoverable direction.
func TestCoverageProbeFailureHydratesEverything(t *testing.T) {
	src := &fakeCoverageGatedSource{fakeHydratingSource: fakeHydratingSource{
		provider: "himalayas",
		jobs:     []sources.Job{{ExternalID: "1", Title: "Backend Engineer", Company: "Acme", Description: "<p>x</p>"}},
	}}
	coverage := &fakeCoverage{covered: map[string]bool{"acme": true}, err: errors.New("meili down")}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, Coverage: coverage}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Himalayas", Provider: "himalayas"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !src.probeReturns {
		t.Fatal("the probe was never consulted")
	}
	if len(src.coveredBack) != 0 {
		t.Errorf("covered = %v, want empty — a failed probe must not claim coverage", src.coveredBack)
	}
}

// A covered posting is not written, but the catalogue may already hold it — stored before its
// employer's own board joined the fleet. Dropping it without a word left that row's last_seen_at
// frozen and its company out of the sweep's scope, so it stayed open whether or not the
// aggregator still listed it: on 2026-10-30, 17,306 of 4dayweek's 23,046 open rows had gone
// unseen for a week, 98% of them under a covered employer. The gate still decides what is
// WRITTEN; what it listed is refreshed, so the stored row's liveness tells the truth again.
func TestRunRefreshesAStoredPostingTheCoverageGateSkips(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
		{ExternalID: "2", Title: "Frontend Engineer", Company: "Acme"},
		{ExternalID: "3", Title: "Data Engineer", Company: "Uncovered"},
	}}
	store := &fakeStore{}
	coverage := &fakeCoverage{covered: map[string]bool{"acme": true}}
	r := Runner{Registry: registry(src), Store: store, Coverage: coverage}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Himalayas", Provider: "himalayas"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats.Total().Refreshed; got != 2 {
		t.Errorf("Refreshed = %d, want 2 — what the store reported refreshing", got)
	}
	got := slices.Sorted(slices.Values(store.refreshed["himalayas"]))
	if want := []string{":1", ":2"}; !slices.Equal(got, want) {
		t.Errorf("refreshed = %v, want %v — every covered posting, and only those", got, want)
	}
	if len(store.saved) != 1 {
		t.Errorf("saved %d postings, want 1 — the gate still decides what is written", len(store.saved))
	}
}

// A failed refresh leaves the covered rows exactly as stale as before, while the listing named
// them. The run must say so: a sweep that read those rows' age as absence would close postings
// still listed — and a covered row, never written again, would never reopen.
func TestRunCountsTheCoveredPostingsAFailedRefreshMissed(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
		{ExternalID: "2", Title: "Frontend Engineer", Company: "Acme"},
		{ExternalID: "3", Title: "Data Engineer", Company: "Uncovered"},
	}}
	store := &fakeStore{refreshErr: errors.New("db down")}
	coverage := &fakeCoverage{covered: map[string]bool{"acme": true}}
	r := Runner{Registry: registry(src), Store: store, Coverage: coverage}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Himalayas", Provider: "himalayas", Board: "remote"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats.Total().RefreshFailed; got != 2 {
		t.Errorf("RefreshFailed = %d, want 2 — both covered postings went unrefreshed", got)
	}
	if got := stats.Total().Refreshed; got != 0 {
		t.Errorf("Refreshed = %d, want 0", got)
	}
	if got := stats["himalayas"].QualifyingBoards; len(got) != 0 {
		t.Errorf("QualifyingBoards = %v, want none — the board's covered rows were not refreshed", got)
	}
}

// The streaming path runs the same gate one posting at a time, so it owes the same refresh.
func TestRunRefreshesStreamedPostingsTheCoverageGateSkips(t *testing.T) {
	src := fakeStreamingSource{provider: "jobtech", failAfter: -1, jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
		{ExternalID: "2", Title: "Data Engineer", Company: "Uncovered"},
	}}
	store := &fakeStore{}
	coverage := &fakeCoverage{covered: map[string]bool{"acme": true}}
	r := Runner{Registry: registry(src), Store: store, Coverage: coverage}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "jobtech", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := store.refreshed["jobtech"], []string{"acme:1"}; !slices.Equal(got, want) {
		t.Errorf("refreshed = %v, want %v", got, want)
	}
	if got := stats.Total().Refreshed; got != 1 {
		t.Errorf("Refreshed = %d, want 1", got)
	}
}

// A board with nothing covered asks the store for nothing.
func TestRunRefreshesNothingWhenNothingIsCovered(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store, Coverage: &fakeCoverage{}}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Himalayas", Provider: "himalayas"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.refreshed) != 0 {
		t.Errorf("refreshed = %v, want no call", store.refreshed)
	}
}

// TestRunSkipsAggregatorPostingForATSCoveredCompany proves the ingest-time coverage gate:
// a posting from an aggregator-classified provider (himalayas) is not saved when its company
// already has open coverage from a non-aggregator source, per the wired fakeCoverage.
func TestRunSkipsAggregatorPostingForATSCoveredCompany(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
	}}
	store := &fakeStore{}
	coverage := &fakeCoverage{covered: map[string]bool{"acme": true}}
	r := Runner{Registry: registry(src), Store: store, Coverage: coverage}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "himalayas", Board: ""},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 0 {
		t.Fatalf("saved = %+v, want none (company already covered by a non-aggregator source)", store.saved)
	}
	if stats.Total().ATSCovered != 1 {
		t.Errorf("stats.Total().ATSCovered = %d, want 1", stats.Total().ATSCovered)
	}
	if stats.Total().Ingested != 0 || stats.Total().Rejected != 0 {
		t.Errorf("stats = %+v, want Ingested=0 Rejected=0 (a coverage skip is neither)", stats.Total())
	}
}

// TestRunSavesAggregatorPostingForUncoveredCompany proves the gate does not fire when the
// company has no non-aggregator coverage: the posting is saved exactly as before.
func TestRunSavesAggregatorPostingForUncoveredCompany(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
	}}
	store := &fakeStore{}
	coverage := &fakeCoverage{covered: map[string]bool{}}
	r := Runner{Registry: registry(src), Store: store, Coverage: coverage}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "himalayas", Board: ""},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 1 {
		t.Fatalf("saved = %+v, want 1 (no non-aggregator coverage)", store.saved)
	}
	if stats.Total().Ingested != 1 || stats.Total().ATSCovered != 0 {
		t.Errorf("stats = %+v, want Ingested=1 ATSCovered=0", stats.Total())
	}
}

// TestRunIgnoresCoverageForATSProvider proves the gate only ever evaluates for a
// KindAggregator provider: an ATS board (greenhouse) saves every posting even when a
// configured Coverage port would report the company covered.
func TestRunIgnoresCoverageForATSProvider(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
	}}
	store := &fakeStore{}
	coverage := &fakeCoverage{covered: map[string]bool{"acme": true}}
	r := Runner{Registry: registry(src), Store: store, Coverage: coverage}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 1 || stats.Total().Ingested != 1 {
		t.Fatalf("saved=%d stats=%+v, want 1 saved (ATS boards ignore the coverage gate)", len(store.saved), stats.Total())
	}
	if len(coverage.calls) != 0 {
		t.Errorf("coverage.calls = %v, want none — the gate must not even query for a KindATS provider", coverage.calls)
	}
}

// TestRunSavesAggregatorPostingWhenCoverageNotWired proves a nil Coverage port reproduces
// today's behavior: the gate is simply not applied, no error is raised.
func TestRunSavesAggregatorPostingWhenCoverageNotWired(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store} // Coverage left nil

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "himalayas", Board: ""},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 1 || stats.Total().Ingested != 1 || stats.Total().ATSCovered != 0 {
		t.Fatalf("saved=%d stats=%+v, want 1 saved Ingested=1 ATSCovered=0", len(store.saved), stats.Total())
	}
}

// TestRunCoverageLookupFailureSavesEverything proves a failed lookup reads as "nothing is
// covered" for the WHOLE board rather than failing the run. The two errors the gate can make
// are not symmetric: a wrong "covered" is unrecoverable — the posting is never written, so it
// is never in the database, in /find, or in search, and leaves nothing to query afterwards —
// while a wrong "uncovered" costs one duplicate row that aggregator-ats-dedup's periodic pass
// already marks. So the recoverable direction is the only safe one to fail in.
func TestRunCoverageLookupFailureSavesEverything(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
		{ExternalID: "2", Title: "Frontend Engineer", Company: "Acme"},
	}}
	store := &fakeStore{}
	// covered says Acme IS covered, but the error takes precedence: an errored lookup has no
	// answer at all, and reusing a partial one would be the unrecoverable direction.
	coverage := &fakeCoverage{covered: map[string]bool{"acme": true}, err: errors.New("connection refused")}
	r := Runner{Registry: registry(src), Store: store, Coverage: coverage}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "himalayas", Board: ""},
	})
	if err != nil {
		t.Fatalf("Run: %v — a coverage lookup failure must not fail the run", err)
	}
	if len(store.saved) != 2 || stats.Total().Ingested != 2 || stats.Total().ATSCovered != 0 {
		t.Fatalf("saved=%d stats=%+v, want both saved Ingested=2 ATSCovered=0",
			len(store.saved), stats.Total())
	}
}

// TestRunKeysCoverageByTheSlugItWillStore proves the pipeline speaks to the port in exactly
// one vocabulary: the alias-resolved company_slug the upsert is about to write. Folding two
// spellings of one employer together is real and necessary, but it is the IMPLEMENTATION's
// business (cmd/ingest/coverage.go folds before asking and credits the answer back to every
// spelling that folds to it) — the port's contract says an answer keyed any other way is not
// an answer to what was asked. So a covered-set entry that agrees with the posting only after
// folding must NOT match here: if the pipeline honoured it, the gate would be deciding about
// one slug and the upsert writing another.
func TestRunKeysCoverageByTheSlugItWillStore(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		// CompanySlug normalizes "CFO Insights" to "cfo-insights". The covered set below
		// answers with the FOLDED spelling ("cfoinsights", no hyphen) — a key the pipeline
		// never asked about, which a conforming lookup would never return.
		{ExternalID: "1", Title: "Backend Engineer", Company: "CFO Insights"},
	}}
	store := &fakeStore{}
	coverage := &fakeCoverage{covered: map[string]bool{"cfoinsights": true}}
	r := Runner{Registry: registry(src), Store: store, Coverage: coverage}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "CFO Insights", Provider: "himalayas", Board: ""},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 1 || stats.Total().ATSCovered != 0 {
		t.Fatalf("saved=%d ATSCovered=%d, want 1 saved / 0 covered — an answer keyed by a slug nobody asked about must not match", len(store.saved), stats.Total().ATSCovered)
	}
}

// TestRunGatesStreamingAggregatorAndMemoizesPerCompany proves the coverage gate also applies
// to a streaming aggregator source (jobtech is the one today), which never hands the pipeline
// a full batch to resolve up front: covered postings are skipped, uncovered postings are
// saved, and the underlying CoverageLookup is asked about each distinct company only once
// even though "Acme" appears in two postings in this one stream.
func TestRunGatesStreamingAggregatorAndMemoizesPerCompany(t *testing.T) {
	src := fakeStreamingSource{provider: "jobtech", failAfter: -1, jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
		{ExternalID: "2", Title: "Frontend Engineer", Company: "Acme"},
		{ExternalID: "3", Title: "Data Engineer", Company: "Other"},
	}}
	store := &fakeStore{}
	coverage := &fakeCoverage{covered: map[string]bool{"acme": true}}
	r := Runner{Registry: registry(src), Store: store, Coverage: coverage}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Multi", Provider: "jobtech", Board: ""},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 1 || store.saved[0].Fields().Company != "Other" {
		t.Fatalf("saved = %+v, want only the Other posting (Acme is covered)", store.saved)
	}
	if stats.Total().ATSCovered != 2 || stats.Total().Ingested != 1 {
		t.Errorf("stats = %+v, want ATSCovered=2 Ingested=1", stats.Total())
	}
	if len(coverage.calls) != 2 {
		t.Fatalf("coverage.calls = %v, want 2 calls (one per distinct company, memoized)", coverage.calls)
	}
}

// TestRunPassesAggregatorListToCoverageLookup proves the gate forwards
// sources.AggregatorProviders(sources.Taxonomy()) as NonAggregatorCompanies' aggregators
// argument, not an empty or nil list — a wrong list here would silently pass every
// coverage check on the adapter side, regardless of a company's real
// coverage, and no other test asserts on this argument.
func TestRunPassesAggregatorListToCoverageLookup(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
	}}
	coverage := &fakeCoverage{}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, Coverage: coverage}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "himalayas", Board: ""},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(coverage.calls) != 1 {
		t.Fatalf("coverage.calls = %v, want 1 call", coverage.calls)
	}
	if len(coverage.aggregatorArgs) != 1 || len(coverage.aggregatorArgs[0]) == 0 {
		t.Fatalf("aggregators argument = %v, want a non-empty provider list", coverage.aggregatorArgs)
	}
	if !slices.Contains(coverage.aggregatorArgs[0], "himalayas") {
		t.Errorf("aggregators argument %v does not contain %q", coverage.aggregatorArgs[0], "himalayas")
	}
}

// TestRunStreamingCoverageSkipsBlankCompany proves the streaming resolver never queries
// CoverageLookup for a posting with no company name — normalize.Slug("") folds to "", and
// there is nothing meaningful to ask the lookup about.
func TestRunStreamingCoverageSkipsBlankCompany(t *testing.T) {
	src := fakeStreamingSource{provider: "jobtech", failAfter: -1, jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: ""},
	}}
	coverage := &fakeCoverage{}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, Coverage: coverage}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Blank", Provider: "jobtech", Board: ""},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(coverage.calls) != 0 {
		t.Errorf("coverage.calls = %v, want none (blank company slug is never looked up)", coverage.calls)
	}
}
