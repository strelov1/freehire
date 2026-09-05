package pipeline

import (
	"context"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// TestRunReportsQualifyingBoardOnASuccessfulCrawl is the ordinary case the sweep's board
// scope exists for: a board whose crawl saved something is reported as qualifying.
func TestRunReportsQualifyingBoardOnASuccessfulCrawl(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{{ExternalID: "1", Title: "Backend Engineer"}}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats["greenhouse"].QualifyingBoards; !slices.Contains(got, "acme") {
		t.Errorf("QualifyingBoards = %v, want it to contain %q", got, "acme")
	}
}

// TestRunExcludesABoardNameSharedAcrossRegions: the boards catalog allows one board name to
// exist twice under one provider, distinguished only by region
// (UNIQUE(provider, lower(board), region) — see internal/ingest/boardcatalog). The
// board-scoped close's SQL predicate has no region dimension at all (externalid.Namespace
// does not encode it), so it cannot tell which region's crawl proved coverage. Both regions'
// crawls succeed and yield here — the failure case (one region's crawl not qualifying) would
// be even less safe to allow through — so this pins the conservative behavior: a board name
// spanning more than one region in this run must never enter QualifyingBoards, regardless of
// how any single region's crawl went.
func TestRunExcludesABoardNameSharedAcrossRegions(t *testing.T) {
	src := fakeSource{provider: "lever", jobs: []sources.Job{{ExternalID: "1", Title: "Backend Engineer"}}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme US", Provider: "lever", Board: "acme", Region: "us"},
		{Company: "Acme EU", Provider: "lever", Board: "acme", Region: "eu"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats["lever"].QualifyingBoards; slices.Contains(got, "acme") {
		t.Errorf("QualifyingBoards = %v, must not contain %q — its region is ambiguous this run", got, "acme")
	}
}

// TestRunReportsNoBoardForAZeroYieldCrawl: a board that fetched successfully but returned
// nothing is indistinguishable from a board whose crawl silently broke, so it must not
// qualify even though it did not fail.
func TestRunReportsNoBoardForAZeroYieldCrawl(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: nil}
	r := Runner{Registry: registry(src), Store: &fakeStore{}}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats["greenhouse"].QualifyingBoards; len(got) != 0 {
		t.Errorf("QualifyingBoards = %v, want none — a zero-yield crawl cannot be distinguished from a broken one", got)
	}
}

// TestRunReportsNoBoardForAFailedCrawl: a board whose fetch errored must not qualify.
func TestRunReportsNoBoardForAFailedCrawl(t *testing.T) {
	src := fakeSource{provider: "greenhouse", err: context.DeadlineExceeded}
	r := Runner{Registry: registry(src), Store: &fakeStore{}}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats["greenhouse"].QualifyingBoards; len(got) != 0 {
		t.Errorf("QualifyingBoards = %v, want none — a failed crawl proves nothing", got)
	}
}

// TestRunReportsNoBoardForABoardlessEntry: an entry with no board id has no board scope to
// speak of — BoardPattern("") would match the provider's whole catalogue.
func TestRunReportsNoBoardForABoardlessEntry(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{{ExternalID: "1", Title: "Backend Engineer"}}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: ""},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats["greenhouse"].QualifyingBoards; len(got) != 0 {
		t.Errorf("QualifyingBoards = %v, want none for a boardless entry", got)
	}
}

// TestRunReportsQualifyingBoardWhenEveryPostingWasRejected: the crawl reached these postings
// and the catalogue filter turned them away — that is still proof the board was listed, and
// refusing it would spare exactly the non-tech-heavy boards where stale rows accumulate.
func TestRunReportsQualifyingBoardWhenEveryPostingWasRejected(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Line Cook", Company: "Acme"},
	}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats["greenhouse"]; got.Rejected != 1 {
		t.Fatalf("stats = %+v, want Rejected=1 (fixture assumption)", got)
	}
	if got := stats["greenhouse"].QualifyingBoards; !slices.Contains(got, "acme") {
		t.Errorf("QualifyingBoards = %v, want it to contain %q despite every posting being rejected", got, "acme")
	}
}

// TestRunReportsOnlyBoardsItActuallyCrawled: a run that only entered a subset of a
// provider's boards (a targeted run, a shard) reports only those as qualifying — a board
// simply absent from the run's entries can never appear, by construction of RunStats being
// built entirely from what Run() iterated.
func TestRunReportsOnlyBoardsItActuallyCrawled(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{{ExternalID: "1", Title: "Backend Engineer"}}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}}

	// Only "acme" is in this run's entries — "globex" (another board of the same provider)
	// is not, standing in for a board this run never reached.
	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := stats["greenhouse"].QualifyingBoards
	if !slices.Contains(got, "acme") {
		t.Errorf("QualifyingBoards = %v, want it to contain the crawled board %q", got, "acme")
	}
	if slices.Contains(got, "globex") {
		t.Errorf("QualifyingBoards = %v, must not contain a board this run never crawled", got)
	}
}

// TestAmbiguousBoardNamesReportsBoardNameAlone: cmd/ingest calls AmbiguousBoardNames on the
// FULL, unsharded board list before sharding splits it across processes (see its doc comment
// for why a per-Run() ambiguity check alone cannot catch a region-ambiguous board split across
// shards). It reports by board name alone since a caller only ever crawls one provider.
func TestAmbiguousBoardNamesReportsBoardNameAlone(t *testing.T) {
	entries := []sources.CompanyEntry{
		{Company: "Acme US", Provider: "workday", Board: "acme", Region: "us"},
		{Company: "Acme EU", Provider: "workday", Board: "acme", Region: "eu"},
		{Company: "Globex", Provider: "workday", Board: "globex", Region: "us"},
	}
	got := AmbiguousBoardNames(entries)
	if !got["acme"] {
		t.Errorf("AmbiguousBoardNames(%v) = %v, want it to contain %q", entries, got, "acme")
	}
	if got["globex"] {
		t.Errorf("AmbiguousBoardNames(%v) = %v, must not contain %q — it has only one region", entries, got, "globex")
	}
}

// TestRunReportsNoBoardWhenAStreamDiedMidCrawl is the regression this board scope exists to
// avoid (see design.md's "the Failed>0 refinement is load-bearing", freehire#725): a streaming
// board that fails partway through after partial progress is deliberately treated as HEALTHY
// by BoardHealth (a rate-limited stream must not cool a working board down), but its returned
// Stats.Failed still carries the failure — and qualification must refuse it on that basis
// directly, independent of BoardHealth's tolerant verdict. Without this, a stream that died at
// posting 2 of 3 would license the board-scoped sweep to close everything past the point it
// died.
func TestRunReportsNoBoardWhenAStreamDiedMidCrawl(t *testing.T) {
	src := fakeStreamingSource{provider: "eightfold", failAfter: 2, jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
		{ExternalID: "2", Title: "Frontend Engineer", Company: "Acme"},
		{ExternalID: "3", Title: "Data Engineer", Company: "Acme"},
	}}
	health := &fakeHealth{}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "eightfold", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Sanity check on the fixture: BoardHealth must see this as healthy (partial progress),
	// exactly the case that makes Stats.Failed the only signal left to gate on.
	if len(health.failures) != 0 {
		t.Fatalf("board recorded as failed (%v) — fixture must reproduce the tolerant-health case", health.failures)
	}
	if got := stats["eightfold"]; got.Failed == 0 {
		t.Fatalf("stats = %+v, want Failed>0 (fixture assumption: the stream died mid-crawl)", got)
	}
	if got := stats["eightfold"].QualifyingBoards; len(got) != 0 {
		t.Errorf("QualifyingBoards = %v, want none — a board caught mid-crawl must not qualify even though BoardHealth treats it as healthy", got)
	}
}
