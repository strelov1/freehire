package pipeline

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

// fakeStore records every saved job and every (source, external_id) closed. It implements
// the optional closer capability, so it is safe for the runner's concurrent Save/Close calls.
// It also implements the optional seenLookup capability (ExistingExternalIDs), so a hydrating
// source can be driven by a canned seen-set.
type fakeStore struct {
	mu               sync.Mutex
	saved            []job.Job
	closed           [][2]string
	touched          [][2]string
	refreshed        map[string][]string // source -> external ids passed to RefreshCovered
	refreshErr       error               // when set, RefreshCovered fails (writes still succeed)
	refreshCompanies []string            // optional actual stored companies returned by the refresh
	touchSlug        string              // the stored company slug Touch reports
	err              error
	seenIDs          map[string]bool            // stored (namespaced) external_id -> is_tech evidence
	seenByBrd        map[string]map[string]bool // per-board sets, keyed by the requested board
	seenAsked        []string                   // every board the runner scoped a lookup to
	seenErr          error                      // when set, ExistingExternalIDs fails
	// filled records every RecordCompanyDescription call, in order.
	filled []filledDescription
	// descNotDue makes CompanyDescriptionDue answer false (the company already has a
	// description, or was checked inside the refresh window); dueErr makes it fail.
	// dueAsked records the checkedSince bound of every call.
	descNotDue bool
	dueErr     error
	dueAsked   []time.Time
}

type filledDescription struct {
	slug, name, description string
}

func (s *fakeStore) CompanyDescriptionDue(_ context.Context, _ string, checkedSince time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dueAsked = append(s.dueAsked, checkedSince)
	if s.dueErr != nil {
		return false, s.dueErr
	}
	return !s.descNotDue, nil
}

func (s *fakeStore) RecordCompanyDescription(_ context.Context, slug, name, description string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.filled = append(s.filled, filledDescription{slug, name, description})
	return nil
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

func (s *fakeStore) Touch(_ context.Context, source, externalID string, deadline *time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return "", s.err
	}
	s.touched = append(s.touched, [2]string{source, externalID})
	return s.touchSlug, nil
}

// RefreshCovered reports every id as a stored open row.
func (s *fakeStore) RefreshCovered(_ context.Context, source string, externalIDs []string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	if s.refreshErr != nil {
		return nil, s.refreshErr
	}
	if s.refreshed == nil {
		s.refreshed = map[string][]string{}
	}
	s.refreshed[source] = append(s.refreshed[source], externalIDs...)
	if s.refreshCompanies != nil {
		return slices.Clone(s.refreshCompanies), nil
	}
	companies := make([]string, len(externalIDs))
	for i := range companies {
		companies[i] = "acme"
	}
	return companies, nil
}

// fakeCoverage answers NonAggregatorCompanies from a canned covered set, and records every
// batch of company slugs it was asked about — so a test can prove how often (per board vs
// per distinct company) the pipeline calls it.
type fakeCoverage struct {
	mu             sync.Mutex
	covered        map[string]bool // exact company_slug -> covered
	calls          [][]string      // each call's companySlugs argument, in order
	aggregatorArgs [][]string      // each call's aggregators argument, in order
	countryArgs    [][]string      // each call's countries argument, in order
	err            error
}

func (f *fakeCoverage) NonAggregatorCompanies(_ context.Context, companySlugs, aggregators, countries []string) (map[string]bool, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string(nil), companySlugs...))
	f.aggregatorArgs = append(f.aggregatorArgs, append([]string(nil), aggregators...))
	f.countryArgs = append(f.countryArgs, countries)
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

// fakeDescriberSource implements sources.CompanyDescriber alongside the plain
// fakeSource: CompanyDescription returns a canned (description, err) and counts how
// many times it was called, so a test can prove it happens once per board regardless
// of how many postings the board carries.
type fakeDescriberSource struct {
	fakeSource
	description    string
	descriptionErr error
	descCalls      int
}

func (f *fakeDescriberSource) CompanyDescription(context.Context, sources.CompanyEntry) (string, error) {
	f.descCalls++
	return f.description, f.descriptionErr
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

func TestRunStreamingFailureWithoutProgressRecordsBoardFailure(t *testing.T) {
	src := fakeStreamingSource{provider: "teamtailor", failAfter: 0, jobs: []sources.Job{
		{ExternalID: "1", Title: "A", Company: "C", URL: "u"},
	}}
	health := &fakeHealth{cooldowns: map[string]time.Time{}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "teamtailor", Board: "gone-board"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Total().Failed != 1 || stats.Total().Ingested != 0 {
		t.Fatalf("stats = %+v, want one failed board with no progress", stats.Total())
	}
	if len(health.failures) != 1 || health.failures[0] != "teamtailor/gone-board/" {
		t.Fatalf("recorded failures = %v, want [teamtailor/gone-board/]", health.failures)
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

// A board crawled without its seen-set proves nothing about what is gone: every stored posting
// reads as new, so a budgeted adapter yields only its budget of them and the rest go untouched.
// The run says so (SeenUnknown), the board does not qualify for the board scope, and its
// companies are withheld from the company scope — otherwise the sweep would close live postings
// the crawl listed but never refreshed.
func TestRunSeenLookupErrorWithholdsTheBoardsCloses(t *testing.T) {
	src := &fakeHydratingSource{provider: "justjoin", jobs: []sources.Job{
		{ExternalID: "a", Title: "A", Company: "Acme", URL: "u"},
	}}
	store := &fakeStore{seenErr: errors.New("db down")}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "justjoin", Board: "b1"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	st := stats["justjoin"]
	if st.SeenUnknown != 1 {
		t.Errorf("SeenUnknown = %d, want 1", st.SeenUnknown)
	}
	if len(st.QualifyingBoards) != 0 {
		t.Errorf("QualifyingBoards = %v, want none: a board crawled without its seen-set proves nothing", st.QualifyingBoards)
	}
	if !slices.Contains(st.UnprovenCompanies, "acme") {
		t.Errorf("UnprovenCompanies = %v, want acme withheld from the company scope", st.UnprovenCompanies)
	}
	if !slices.Contains(st.SkippedBoards, externalid.BoardPattern("b1")) {
		t.Errorf("SkippedBoards = %v, want b1 excluded from the company and source scopes", st.SkippedBoards)
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

	geoJob, err := normalizeJob(e, sources.Job{ExternalID: "1", Title: "Dev", Company: "Acme", Location: "Remote - Germany"}, nil, false)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	geo := geoJob.Fields()
	if !reflect.DeepEqual(geo.Countries, []string{"de"}) || !reflect.DeepEqual(geo.Regions, []string{"eu"}) {
		t.Errorf("geography = %v/%v, want [de]/[eu]", geo.Countries, geo.Regions)
	}

	// A bare "Remote" resolves no country and no region: it states nothing about where
	// it is open to, so it stays in the "region not specified" bucket, not global (its
	// remoteness stays on WorkMode; see location.Parse).
	bareJob, err := normalizeJob(e, sources.Job{ExternalID: "2", Title: "Dev", Company: "Acme", Location: "Remote"}, nil, false)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	bare := bareJob.Fields()
	if len(bare.Countries) != 0 || len(bare.Regions) != 0 {
		t.Errorf("bare remote geography = %v/%v, want []/[]", bare.Countries, bare.Regions)
	}
}

func TestNormalizeJobPrefersAdapterWorkModeOverParser(t *testing.T) {
	e := sources.CompanyEntry{Company: "Acme", Provider: "greenhouse", Board: "acme"}

	// The adapter states hybrid structurally; the location text would parse as
	// remote. The structured signal wins.
	structured, err := normalizeJob(e, sources.Job{ExternalID: "1", Title: "Dev", Company: "Acme", Location: "Remote", WorkMode: "hybrid"}, nil, false)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	if structured.Fields().WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid (adapter structured wins over parser)", structured.Fields().WorkMode)
	}

	// No structured signal: the parser fills from the location text.
	parsed, err := normalizeJob(e, sources.Job{ExternalID: "2", Title: "Dev", Company: "Acme", Location: "Remote"}, nil, false)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	if parsed.Fields().WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (parser fallback)", parsed.Fields().WorkMode)
	}
}

// TestNormalizeJobPassesIsTechHint pins that an adapter's structured
// "confirmed technical" signal (see issue #2601 — Profession's dedicated IT
// boards) reaches jobderive and therefore is_tech, even for a title neither
// the tech nor the non-tech dictionary resolves.
func TestNormalizeJobPassesIsTechHint(t *testing.T) {
	e := sources.CompanyEntry{Company: "Acme", Provider: "profession", Board: "itdev"}

	hinted, err := normalizeJob(e, sources.Job{ExternalID: "1", Title: "Yard Coordinator", Company: "Acme", IsTechHint: true}, nil, false)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	if got := hinted.Fields().IsTech; got == nil || !*got {
		t.Errorf("IsTech = %v, want true (adapter's structured IsTechHint wins)", got)
	}

	unhinted, err := normalizeJob(e, sources.Job{ExternalID: "2", Title: "Yard Coordinator", Company: "Acme"}, nil, false)
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	if got := unhinted.Fields().IsTech; got != nil {
		t.Errorf("IsTech = %v, want nil (no hint, dictionary resolves neither)", *got)
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

func TestRunClassifiesOnlyExplicitBoardGoneFailures(t *testing.T) {
	health := &fakeHealth{}
	r := Runner{
		Registry:    registry(fakeSource{provider: "greenhouse", err: sources.ErrBoardGone}, fakeSource{provider: "lever", err: errors.New("403 forbidden")}),
		Store:       &fakeStore{},
		BoardHealth: health,
	}
	_, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Gone", Provider: "greenhouse", Board: "gone"},
		{Company: "Refused", Provider: "lever", Board: "refused"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !reflect.DeepEqual(health.boardGone, []string{"greenhouse/gone/"}) {
		t.Fatalf("boardGone evidence = %v, want only the explicit disappearance", health.boardGone)
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
		nil, false,
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
		nil, false,
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
