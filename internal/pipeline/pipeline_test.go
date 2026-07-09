package pipeline

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/sources"
)

// fakeStore records every saved job and every (source, external_id) closed. It implements
// the optional closer capability, so it is safe for the runner's concurrent Save/Close calls.
type fakeStore struct {
	mu     sync.Mutex
	saved  []job.Job
	closed [][2]string
	err    error
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
	if stats.Total().Ingested != 2 || stats.Total().Failed != 0 {
		t.Fatalf("stats = %+v, want Ingested=2 Failed=0 (one save + one close)", stats.Total())
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
	if j.CompanySlug != "acme-inc" {
		t.Errorf("CompanySlug = %q, want %q", j.CompanySlug, "acme-inc")
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

	geoJob, err := normalizeJob(e, sources.Job{ExternalID: "1", Title: "Dev", Company: "Acme", Location: "Remote - Germany"})
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	geo := geoJob.Fields()
	if !reflect.DeepEqual(geo.Countries, []string{"de"}) || !reflect.DeepEqual(geo.Regions, []string{"eu"}) {
		t.Errorf("geography = %v/%v, want [de]/[eu]", geo.Countries, geo.Regions)
	}

	// A bare "Remote" resolves no country, so it falls into the open-anywhere global
	// region (its remoteness stays on WorkMode; see location.Parse).
	bareJob, err := normalizeJob(e, sources.Job{ExternalID: "2", Title: "Dev", Company: "Acme", Location: "Remote"})
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
	structured, err := normalizeJob(e, sources.Job{ExternalID: "1", Title: "Dev", Company: "Acme", Location: "Remote", WorkMode: "hybrid"})
	if err != nil {
		t.Fatalf("normalizeJob: %v", err)
	}
	if structured.Fields().WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid (adapter structured wins over parser)", structured.Fields().WorkMode)
	}

	// No structured signal: the parser fills from the location text.
	parsed, err := normalizeJob(e, sources.Job{ExternalID: "2", Title: "Dev", Company: "Acme", Location: "Remote"})
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
