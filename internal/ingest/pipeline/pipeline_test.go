package pipeline

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/job"
)

// fakeStore records every saved job and every (source, external_id) closed. It implements
// the optional closer capability, so it is safe for the runner's concurrent Save/Close calls.
// It also implements the optional seenLookup capability (ExistingExternalIDs), so a hydrating
// source can be driven by a canned seen-set.
type fakeStore struct {
	mu        sync.Mutex
	saved     []job.Job
	closed    [][2]string
	touched   [][2]string
	err       error
	seenIDs   map[string]bool            // stored (namespaced) external_id -> is_tech evidence
	seenByBrd map[string]map[string]bool // per-board sets, keyed by the requested board
	seenAsked []string                   // every board the runner scoped a lookup to
	seenErr   error                      // when set, ExistingExternalIDs fails
}

func (s *fakeStore) ExistingExternalIDs(_ context.Context, _, board string) (map[string]bool, error) {
	s.mu.Lock()
	s.seenAsked = append(s.seenAsked, board)
	s.mu.Unlock()
	if s.seenErr != nil {
		return nil, s.seenErr
	}
	if s.seenByBrd != nil {
		return s.seenByBrd[board], nil
	}
	return s.seenIDs, nil
}

func (s *fakeStore) Save(_ context.Context, j job.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.saved = append(s.saved, j)
	return nil
}

func (s *fakeStore) Close(_ context.Context, source, externalID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.closed = append(s.closed, [2]string{source, externalID})
	return nil
}

func (s *fakeStore) Touch(_ context.Context, source, externalID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.touched = append(s.touched, [2]string{source, externalID})
	return nil
}

// fakeCoverage answers NonAggregatorCompanies from a canned covered set, and records every
// batch of company slugs it was asked about — so a test can prove how often (per board vs
// per distinct company) the pipeline calls it.
type fakeCoverage struct {
	mu             sync.Mutex
	covered        map[string]bool // exact company_slug -> covered
	calls          [][]string      // each call's companySlugs argument, in order
	aggregatorArgs [][]string      // each call's aggregators argument, in order
	err            error
}

func (f *fakeCoverage) NonAggregatorCompanies(_ context.Context, companySlugs, aggregators []string) (map[string]bool, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string(nil), companySlugs...))
	f.aggregatorArgs = append(f.aggregatorArgs, append([]string(nil), aggregators...))
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]bool)
	for _, slug := range companySlugs {
		if f.covered[slug] {
			out[slug] = true
		}
	}
	return out, nil
}

// fakeSource returns canned jobs or an error, keyed by provider.
type fakeSource struct {
	provider string
	jobs     []sources.Job
	err      error
}

func (f fakeSource) Provider() string { return f.provider }

func (f fakeSource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return f.jobs, f.err
}

// fakeStreamingSource implements sources.StreamingSource: FetchStream emits jobs through the
// sink, optionally failing after failAfter jobs (-1 = never), so a test can prove the runner
// persists incrementally and keeps the jobs emitted before a mid-crawl error. Its Fetch returns
// ALL jobs with no error, so a test that sees a partial/failed result proves the streaming path
// (not Fetch) was used.
type fakeStreamingSource struct {
	provider  string
	jobs      []sources.Job
	failAfter int
}

func (f fakeStreamingSource) Provider() string { return f.provider }

func (f fakeStreamingSource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return f.jobs, nil
}

func (f fakeStreamingSource) FetchStream(_ context.Context, _ sources.CompanyEntry, emit func(sources.Job)) error {
	for i, j := range f.jobs {
		if f.failAfter >= 0 && i >= f.failAfter {
			return errors.New("stream failed midway")
		}
		emit(j)
	}
	return nil
}

// fakeHydratingSource implements sources.HydratingSource: FetchNew records that it was used and
// captures the seen(externalID) result for each job's raw id, so a test can prove the runner
// preferred FetchNew and supplied a predicate reflecting the store's seen-set. Its Fetch returns
// the same jobs, so a test seeing FetchNew's side effects proves the hydrating path was taken.
type fakeHydratingSource struct {
	provider       string
	jobs           []sources.Job
	fetchNewCalled bool
	seenResults    map[string]bool
}

func (f *fakeHydratingSource) Provider() string { return f.provider }

func (f *fakeHydratingSource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return f.jobs, nil
}

func (f *fakeHydratingSource) FetchNew(_ context.Context, _ sources.CompanyEntry, seen func(string) bool) ([]sources.Job, error) {
	f.fetchNewCalled = true
	f.seenResults = map[string]bool{}
	out := make([]sources.Job, len(f.jobs))
	for i, j := range f.jobs {
		s := seen(j.ExternalID)
		f.seenResults[j.ExternalID] = s
		// Mirror the real adapter: a seen offer is marked for liveness refresh, not upsert.
		j.SeenRefresh = s
		out[i] = j
	}
	return out, nil
}

func registry(srcs ...sources.Source) map[string]sources.Source {
	m := make(map[string]sources.Source)
	for _, s := range srcs {
		m[s.Provider()] = s
	}
	return m
}

// TestRunStreamsAndPersistsPartialBeforeError proves the runner consumes a StreamingSource via
// FetchStream and persists each job as it is emitted: when the stream fails mid-crawl, the jobs
// emitted before the error stay saved (incremental), and the board is counted failed. Via the
// old Fetch path this source would save all 3 with no failure, so this result is unique to the
// streaming path.
func TestRunStreamsAndPersistsPartialBeforeError(t *testing.T) {
	src := fakeStreamingSource{provider: "eightfold", failAfter: 2, jobs: []sources.Job{
		{ExternalID: "1", Title: "A", Company: "C", URL: "u"},
		{ExternalID: "2", Title: "B", Company: "C", URL: "u"},
		{ExternalID: "3", Title: "D", Company: "C", URL: "u"},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "eightfold", Board: "host.example/dom"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 2 {
		t.Fatalf("len(saved) = %d, want 2 (jobs emitted before the error persist)", len(store.saved))
	}
	if stats.Total().Ingested != 2 || stats.Total().Failed != 1 {
		t.Fatalf("stats = %+v, want Ingested=2 Failed=1", stats.Total())
	}
}

// TestRunStreamClosesRemovedJobs proves the runner routes a removed posting to the Store's
// close path (by identity) instead of upserting it: a self-closing stream emits one live ad
// and one removed ad, and the runner saves the first and closes the second. The closed
// identity is the same (source, external_id) the live upsert would use — board-namespaced,
// here with an empty board (jobtech is boardless), so external_id is ":<id>".
func TestRunStreamClosesRemovedJobs(t *testing.T) {
	src := fakeStreamingSource{provider: "jobtech", failAfter: -1, jobs: []sources.Job{
		{ExternalID: "1", Title: "A", Company: "C", URL: "u"},
		{ExternalID: "2", Removed: true},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "jobtech", Board: ""},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 1 || store.saved[0].Fields().ExternalID != ":1" {
		t.Fatalf("saved = %+v, want 1 live job with external_id \":1\"", store.saved)
	}
	if len(store.closed) != 1 || store.closed[0] != [2]string{"jobtech", ":2"} {
		t.Fatalf("closed = %v, want one close of (jobtech, :2)", store.closed)
	}
	if stats.Total().Ingested != 1 || stats.Total().Failed != 0 {
		t.Fatalf("stats = %+v, want Ingested=1 Failed=0 (a close is not a saved job)", stats.Total())
	}
}

// TestRunStreamsAllJobs verifies a clean streaming crawl saves every emitted job.
func TestRunStreamsAllJobs(t *testing.T) {
	src := fakeStreamingSource{provider: "eightfold", failAfter: -1, jobs: []sources.Job{
		{ExternalID: "1", Title: "A", Company: "C", URL: "u"},
		{ExternalID: "2", Title: "B", Company: "C", URL: "u"},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "eightfold", Board: "host.example/dom"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 2 || stats.Total().Ingested != 2 || stats.Total().Failed != 0 {
		t.Fatalf("saved=%d stats=%+v, want 2 saved Ingested=2 Failed=0", len(store.saved), stats.Total())
	}
}

// TestRunDrivesHydratingSourceWithSeenSet proves the runner prefers FetchNew for a
// HydratingSource and supplies a seen predicate backed by the store's set of already-ingested
// (namespaced) external_ids: the boardless offer already stored as ":seen" reads seen=true, a
// new offer reads seen=false.
func TestRunDrivesHydratingSourceWithSeenSet(t *testing.T) {
	src := &fakeHydratingSource{provider: "justjoin", jobs: []sources.Job{
		{ExternalID: "seen", Title: "A", Company: "C", URL: "u"},
		{ExternalID: "new", Title: "B", Company: "C", URL: "u"},
	}}
	store := &fakeStore{seenIDs: map[string]bool{":seen": true}}
	r := Runner{Registry: registry(src), Store: store}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "justjoin", Board: ""},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !src.fetchNewCalled {
		t.Fatal("runner should call FetchNew for a HydratingSource, not Fetch")
	}
	if !src.seenResults["seen"] {
		t.Error("seen(\"seen\") = false, want true (already stored as \":seen\")")
	}
	if src.seenResults["new"] {
		t.Error("seen(\"new\") = true, want false (not yet stored)")
	}
	// The new offer is upserted; the seen offer is only touched (liveness refresh), so its
	// hydrated content is never overwritten by a content-less re-upsert. Boardless → ":<id>".
	if len(store.saved) != 1 || store.saved[0].Fields().ExternalID != ":new" {
		t.Errorf("saved = %+v, want only the new offer (\":new\")", store.saved)
	}
	if len(store.touched) != 1 || store.touched[0] != [2]string{"justjoin", ":seen"} {
		t.Errorf("touched = %v, want one touch of (justjoin, :seen)", store.touched)
	}
}

// TestRunScopesSeenSetToTheCrawledBoard proves the seen-set of a multi-board provider is read per
// board, not per provider: each board's lookup is scoped to its own board id, so a posting stored
// under a sibling board is NOT reported as seen. Without the scope a provider-wide read would cost
// the whole catalogue on every one of the provider's boards.
func TestRunScopesSeenSetToTheCrawledBoard(t *testing.T) {
	src := &fakeHydratingSource{provider: "workday", jobs: []sources.Job{
		{ExternalID: "1", Title: "A", Company: "C", URL: "u"},
	}}
	store := &fakeStore{seenByBrd: map[string]map[string]bool{
		"boardA": {"boardA:1": true},
		"boardB": {"boardB:9": true},
	}}
	r := Runner{Registry: registry(src), Store: store}

	// Crawl boardB, whose stored posting is a different id: posting "1" belongs to boardA and
	// must not read as seen here.
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "workday", Board: "boardB"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !slices.Equal(store.seenAsked, []string{"boardB"}) {
		t.Errorf("seen-set scoped to %v, want [boardB]", store.seenAsked)
	}
	if src.seenResults["1"] {
		t.Error("seen(\"1\") = true, want false — boardA:1 belongs to another board")
	}
	if len(store.saved) != 1 || store.saved[0].Fields().ExternalID != "boardB:1" {
		t.Errorf("saved = %+v, want the posting hydrated as boardB:1", store.saved)
	}
}

// TestRunHydratingSourceFailsOpenOnSeenLookupError proves a seen-set query failure does not skip
// the board: the runner falls back to an empty seen-set (every offer treated as new) and still
// crawls via FetchNew.
func TestRunHydratingSourceFailsOpenOnSeenLookupError(t *testing.T) {
	src := &fakeHydratingSource{provider: "justjoin", jobs: []sources.Job{
		{ExternalID: "a", Title: "A", Company: "C", URL: "u"},
	}}
	store := &fakeStore{seenErr: errors.New("db down")}
	r := Runner{Registry: registry(src), Store: store}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "justjoin", Board: ""},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !src.fetchNewCalled {
		t.Fatal("runner should still crawl via FetchNew despite the seen-lookup error")
	}
	if src.seenResults["a"] {
		t.Error("seen(\"a\") = true, want false (empty set on lookup error)")
	}
	if len(store.saved) != 1 {
		t.Errorf("len(saved) = %d, want 1 (board still crawled)", len(store.saved))
	}
}

func TestRunNormalizesAndNamespaces(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "42", Title: "Senior Go Developer", Company: "Acme Inc", URL: "u", Location: "Remote", Remote: true},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme Inc", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Total().Ingested != 1 || stats.Total().Failed != 0 {
		t.Fatalf("stats = %+v, want Ingested=1 Failed=0", stats)
	}
	if len(store.saved) != 1 {
		t.Fatalf("len(saved) = %d, want 1", len(store.saved))
	}

	j := store.saved[0].Fields()
	if j.Source != "greenhouse" {
		t.Errorf("Source = %q, want %q", j.Source, "greenhouse")
	}
	if j.ExternalID != "acme:42" {
		t.Errorf("ExternalID = %q, want %q (board-namespaced)", j.ExternalID, "acme:42")
	}
	// "Acme Inc" keys as `acme`: the company slug is normalize.CompanySlug, which drops the
	// corporate form, so the same employer cannot arrive as two companies depending on
	// whether its source spelled the form out.
	if j.CompanySlug != "acme" {
		t.Errorf("CompanySlug = %q, want %q", j.CompanySlug, "acme")
	}
	if j.Title != "Senior Go Developer" || j.URL != "u" || !j.Remote {
		t.Errorf("passthrough fields wrong: %+v", j)
	}
	// public_slug is minted from the stored identity (title, company, source,
	// namespaced external_id) so it is deterministic with the dedup key.
	wantSlug := normalize.JobSlug(j.Title, j.Company, j.Source, j.ExternalID)
	if j.PublicSlug == "" || j.PublicSlug != wantSlug {
		t.Errorf("PublicSlug = %q, want %q", j.PublicSlug, wantSlug)
	}
}

func TestNormalizeJobParsesGeographyFromLocation(t *testing.T) {
	e := sources.CompanyEntry{Company: "Acme", Provider: "greenhouse", Board: "acme"}

	geoJob, err := normalizeJob(e, sources.Job{ExternalID: "1", Title: "Dev", Company: "Acme", Location: "Remote - Germany"}, nil)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	geo := geoJob.Fields()
	if !reflect.DeepEqual(geo.Countries, []string{"de"}) || !reflect.DeepEqual(geo.Regions, []string{"eu"}) {
		t.Errorf("geography = %v/%v, want [de]/[eu]", geo.Countries, geo.Regions)
	}

	// A bare "Remote" resolves no country, so it falls into the open-anywhere global
	// region (its remoteness stays on WorkMode; see location.Parse).
	bareJob, err := normalizeJob(e, sources.Job{ExternalID: "2", Title: "Dev", Company: "Acme", Location: "Remote"}, nil)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	bare := bareJob.Fields()
	if len(bare.Countries) != 0 || !reflect.DeepEqual(bare.Regions, []string{"global"}) {
		t.Errorf("bare remote geography = %v/%v, want []/[global]", bare.Countries, bare.Regions)
	}
}

func TestNormalizeJobPrefersAdapterWorkModeOverParser(t *testing.T) {
	e := sources.CompanyEntry{Company: "Acme", Provider: "greenhouse", Board: "acme"}

	// The adapter states hybrid structurally; the location text would parse as
	// remote. The structured signal wins.
	structured, err := normalizeJob(e, sources.Job{ExternalID: "1", Title: "Dev", Company: "Acme", Location: "Remote", WorkMode: "hybrid"}, nil)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	if structured.Fields().WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid (adapter structured wins over parser)", structured.Fields().WorkMode)
	}

	// No structured signal: the parser fills from the location text.
	parsed, err := normalizeJob(e, sources.Job{ExternalID: "2", Title: "Dev", Company: "Acme", Location: "Remote"}, nil)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	if parsed.Fields().WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (parser fallback)", parsed.Fields().WorkMode)
	}
}

func TestRunIsolatesSourceFailure(t *testing.T) {
	good := fakeSource{provider: "greenhouse", jobs: []sources.Job{{ExternalID: "1", Title: "ok"}}}
	bad := fakeSource{provider: "lever", err: errors.New("boom")}
	store := &fakeStore{}
	r := Runner{Registry: registry(good, bad), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Good", Provider: "greenhouse", Board: "good"},
		{Company: "Bad", Provider: "lever", Board: "bad"},
	})
	if err != nil {
		t.Fatalf("Run should not return an error when a single source fails: %v", err)
	}
	if stats.Total().Failed != 1 {
		t.Errorf("stats.Total().Failed = %d, want 1", stats.Total().Failed)
	}
	if stats.Total().Ingested != 1 {
		t.Errorf("stats.Total().Ingested = %d, want 1 (the healthy board)", stats.Total().Ingested)
	}
	if len(store.saved) != 1 || store.saved[0].Fields().Source != "greenhouse" {
		t.Errorf("only the healthy board's job should be saved, got %+v", store.saved)
	}
}

func TestRunSkipsWorkOnCancelledContext(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{{ExternalID: "1", Title: "x"}}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before Run

	stats, err := r.Run(ctx, []sources.CompanyEntry{
		{Company: "C", Provider: "greenhouse", Board: "c"},
	})
	if err == nil {
		t.Fatal("Run should return the context error on a cancelled context")
	}
	if stats.Total().Ingested != 0 {
		t.Errorf("stats.Total().Ingested = %d, want 0 (no work on a cancelled context)", stats.Total().Ingested)
	}
	if len(store.saved) != 0 {
		t.Errorf("saved %d jobs, want 0 on a cancelled context", len(store.saved))
	}
}

func TestRunIsolatesPerJobSaveError(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{{ExternalID: "1", Title: "x"}}}
	store := &fakeStore{err: errors.New("write failed")}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "greenhouse", Board: "c"},
	})
	if err != nil {
		t.Fatalf("Run: a per-job save error must not fail the run: %v", err)
	}
	// A save error is skipped: the job is not counted ingested, but the board did not fail.
	if stats.Total().Ingested != 0 {
		t.Errorf("stats.Total().Ingested = %d, want 0 (save failed)", stats.Total().Ingested)
	}
	if stats.Total().Failed != 0 {
		t.Errorf("stats.Total().Failed = %d, want 0 (a save error is not a board failure)", stats.Total().Failed)
	}
	// The skip is counted so a run whose every save fails (e.g. schema drift) is not
	// reported as a clean ingested=0/failed=0 success.
	if stats.Total().Skipped != 1 {
		t.Errorf("stats.Total().Skipped = %d, want 1 (the save error is counted, not silently swallowed)", stats.Total().Skipped)
	}
}

// TestRunSkipsInvalidDraft proves the runner does not persist a posting the
// aggregate factory rejects (here an empty title): job.New returns ErrInvalidDraft,
// so the job is skipped rather than upserted as junk. Not every adapter filters a
// blank title, so this guard lives in the shared write path.
func TestRunSkipsInvalidDraft(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "", Company: "Acme"}, // no title → invalid draft
		{ExternalID: "2", Title: "Real Job", Company: "Acme"},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 1 {
		t.Fatalf("len(saved) = %d, want 1 (the empty-title posting is skipped, not saved as junk)", len(store.saved))
	}
	if stats.Total().Ingested != 1 || stats.Total().Skipped != 1 {
		t.Errorf("stats = %+v, want Ingested=1 Skipped=1", stats.Total())
	}
}

func TestRunCountsUnknownProviderAsFailed(t *testing.T) {
	store := &fakeStore{}
	r := Runner{Registry: registry(), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "X", Provider: "myspace", Board: "x"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Total().Failed != 1 || stats.Total().Ingested != 0 {
		t.Errorf("stats = %+v, want Failed=1 Ingested=0", stats)
	}
}

func TestNormalizeJobDerivesSkills(t *testing.T) {
	dj, err := normalizeJob(
		sources.CompanyEntry{Provider: "greenhouse", Board: "acme"},
		sources.Job{
			Title: "Backend Engineer", Company: "Acme", ExternalID: "1",
			Description: "<p>Build services in Golang with PostgreSQL and Kubernetes.</p>",
		},
		nil,
	)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	want := []string{"go", "kubernetes", "postgresql"}
	if got := dj.Fields().Skills; !reflect.DeepEqual(got, want) {
		t.Fatalf("Skills = %#v, want %#v", got, want)
	}
}

func TestNormalizeJobDerivesClassification(t *testing.T) {
	dj, err := normalizeJob(
		sources.CompanyEntry{Provider: "greenhouse", Board: "acme", Company: "Acme"},
		sources.Job{ExternalID: "1", Title: "Senior Backend Engineer", Description: "x"},
		nil,
	)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	f := dj.Fields()
	if f.Seniority != "senior" {
		t.Errorf("Seniority = %q, want senior", f.Seniority)
	}
	if f.Category != "backend" {
		t.Errorf("Category = %q, want backend", f.Category)
	}
}

// fakeCoverageGatedSource implements sources.CoverageGated on top of the hydrating fake. It
// records the companies the probe was asked about and the answer it got, so a test can prove
// the runner routed to the gated path and handed it a resolver that actually resolves.
type fakeCoverageGatedSource struct {
	fakeHydratingSource
	gatedCalled  bool
	askedAbout   []string
	coveredBack  map[string]bool
	probeReturns bool // set once the probe has answered, so nil vs empty is distinguishable
}

func (f *fakeCoverageGatedSource) FetchNewGated(_ context.Context, _ sources.CompanyEntry,
	seen func(string) bool, covered func([]string) map[string]bool) ([]sources.Job, error) {
	f.gatedCalled = true
	for _, j := range f.jobs {
		f.askedAbout = append(f.askedAbout, j.Company)
	}
	f.coveredBack = covered(f.askedAbout)
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

// A run over several providers tallies stats per provider (one map key each), so the
// caller can sweep each provider independently. Ingest counts are kept apart.
func TestRunReturnsPerProviderStats(t *testing.T) {
	gh := fakeSource{provider: "greenhouse", jobs: []sources.Job{{ExternalID: "1", Title: "a"}}}
	lv := fakeSource{provider: "lever", jobs: []sources.Job{{ExternalID: "2", Title: "b"}, {ExternalID: "3", Title: "c"}}}
	store := &fakeStore{}
	r := Runner{Registry: registry(gh, lv), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "GH", Provider: "greenhouse", Board: "gh"},
		{Company: "LV", Provider: "lever", Board: "lv"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("want stats for 2 providers, got %d: %+v", len(stats), stats)
	}
	if stats["greenhouse"].Ingested != 1 {
		t.Errorf("greenhouse Ingested = %d, want 1", stats["greenhouse"].Ingested)
	}
	if stats["lever"].Ingested != 2 {
		t.Errorf("lever Ingested = %d, want 2", stats["lever"].Ingested)
	}
}

// fakeAliases answers CanonicalCompanySlugs from a canned folded_key -> canonical_slug map and
// records every batch it was asked about, so a test can prove the lookup happens once per board
// rather than once per posting.
type fakeAliases struct {
	mu    sync.Mutex
	canon map[string]string
	calls [][]string
	err   error
}

func (f *fakeAliases) CanonicalCompanySlugs(_ context.Context, foldedKeys []string) (map[string]string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string(nil), foldedKeys...))
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]string)
	for _, k := range foldedKeys {
		if c, ok := f.canon[k]; ok {
			out[k] = c
		}
	}
	return out, nil
}

// TestRunStoresTheCanonicalCompanySlugAndGatesOnIt is the structural guarantee behind duplicate
// class 1b, and behind the coverage gate not leaking a second time.
//
// "DollarTree" derives to `dollartree`, which a merge registered as an alias of `dollar-tree`.
// The posting must be STORED under the canonical slug, and the coverage lookup must be asked
// about that same canonical slug — because what it is really being asked is "does this employer
// already have ATS coverage", and `dollartree` is not the key the catalogue holds that under.
//
// The two agreeing is not a coincidence to be maintained by hand: both read one resolved map.
func TestRunStoresTheCanonicalCompanySlugAndGatesOnIt(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "DollarTree", URL: "u"},
		{ExternalID: "2", Title: "Data Engineer", Company: "DollarTree", URL: "u"},
	}}
	store := &fakeStore{}
	coverage := &fakeCoverage{covered: map[string]bool{}}
	aliases := &fakeAliases{canon: map[string]string{"dollartree": "dollar-tree"}}
	r := Runner{Registry: registry(src), Store: store, Coverage: coverage, Aliases: aliases}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Himalayas", Provider: "himalayas"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(store.saved) != 2 {
		t.Fatalf("len(saved) = %d, want 2", len(store.saved))
	}
	for _, j := range store.saved {
		if got := j.Fields().CompanySlug; got != "dollar-tree" {
			t.Errorf("stored CompanySlug = %q, want %q (the canonical slug, not the derived one)",
				got, "dollar-tree")
		}
	}
	if len(coverage.calls) != 1 {
		t.Fatalf("coverage was called %d times, want 1 (one batch per board)", len(coverage.calls))
	}
	if got := coverage.calls[0]; len(got) != 1 || got[0] != "dollar-tree" {
		t.Errorf("coverage asked about %v, want [dollar-tree] — the gate must ask about the "+
			"slug the upsert will write, or it silently stops matching", got)
	}
	if len(aliases.calls) != 1 {
		t.Errorf("alias lookup ran %d times, want 1 per board run", len(aliases.calls))
	}
	if got := aliases.calls[0]; len(got) != 1 || got[0] != "dollartree" {
		t.Errorf("alias lookup asked for %v, want [dollartree] (the folded key)", got)
	}
}

// TestRunWithoutAliasesKeepsTheDerivedSlug pins the day-one behaviour: the registry is empty
// until the merge worker writes to it, and an unwired lookup must change nothing.
func TestRunWithoutAliasesKeepsTheDerivedSlug(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "DollarTree", URL: "u"},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Himalayas", Provider: "himalayas"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := store.saved[0].Fields().CompanySlug; got != "dollartree" {
		t.Errorf("CompanySlug = %q, want %q", got, "dollartree")
	}
}

// TestRunGatesOnTheCanonicalSlugAtDecisionTime is the other half of the invariant. The batch
// asks the lookup about the canonical slug; this proves the SKIP decision reads it too.
//
// `dollar-tree` has ATS coverage; the aggregator posting arrives spelled `DollarTree`. Before
// the registry resolved it, the gate compared `dollartree` against a catalogue holding
// `dollar-tree`, found nothing, and saved the duplicate it exists to suppress — the leak in
// its most literal form.
func TestRunGatesOnTheCanonicalSlugAtDecisionTime(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "DollarTree", URL: "u"},
	}}
	store := &fakeStore{}
	coverage := &fakeCoverage{covered: map[string]bool{"dollar-tree": true}}
	aliases := &fakeAliases{canon: map[string]string{"dollartree": "dollar-tree"}}
	r := Runner{Registry: registry(src), Store: store, Coverage: coverage, Aliases: aliases}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Himalayas", Provider: "himalayas"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 0 {
		t.Errorf("saved %d postings, want 0 — the employer already has ATS coverage under its "+
			"canonical slug, and the gate must recognise that through the spelling",
			len(store.saved))
	}
	if got := stats.Total().ATSCovered; got != 1 {
		t.Errorf("ATSCovered = %d, want 1", got)
	}
}

// TestRunGatesOnTheStrippedSlugWithNoRegistry catches the leak in its plainest form, with no
// alias registry involved at all.
//
// The gate's question set and its decision must be the same derivation. When the batch was
// built with normalize.Slug while the posting keyed on normalize.CompanySlug, every company
// carrying a corporate form asked about "acme-inc" and then decided on "acme" — so the lookup
// answered about a company nobody would consult, and the gate skipped nothing. Silently:
// a gate that matches nothing looks exactly like a board with nothing to suppress.
func TestRunGatesOnTheStrippedSlugWithNoRegistry(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme Inc", URL: "u"},
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
	if got := coverage.calls[0]; len(got) != 1 || got[0] != "acme" {
		t.Errorf("coverage asked about %v, want [acme] — the corporate form is not part of the "+
			"company key, so it must not be part of the question", got)
	}
	if len(store.saved) != 0 {
		t.Errorf("saved %d postings, want 0 (the employer has ATS coverage)", len(store.saved))
	}
	if got := stats.Total().ATSCovered; got != 1 {
		t.Errorf("ATSCovered = %d, want 1", got)
	}
}
