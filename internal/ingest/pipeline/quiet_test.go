package pipeline

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

func TestQuietFor(t *testing.T) {
	tests := []struct {
		streak int
		want   time.Duration
		paused bool
	}{
		{0, 0, false},
		{1, 4 * time.Hour, true},
		{2, 8 * time.Hour, true},
		{3, 16 * time.Hour, true},
		{4, 32 * time.Hour, true},
		{5, 64 * time.Hour, true},
		{6, 72 * time.Hour, true},
		{7, 72 * time.Hour, true},
		{200, 72 * time.Hour, true},
	}
	for _, tt := range tests {
		got, paused := QuietFor(tt.streak)
		if got != tt.want || paused != tt.paused {
			t.Errorf("QuietFor(%d) = (%v, %v), want (%v, %v)", tt.streak, got, paused, tt.want, tt.paused)
		}
	}
}

func TestListingFingerprint(t *testing.T) {
	base := listingFingerprint([]string{"b:1", "b:2", "b:3"})
	if base == "" {
		t.Fatal("fingerprint of a non-empty listing is empty")
	}
	if got := listingFingerprint([]string{"b:3", "b:1", "b:2"}); got != base {
		t.Errorf("order changed the fingerprint: %q vs %q", got, base)
	}
	if got := listingFingerprint([]string{"b:1", "b:2", "b:2", "b:3"}); got != base {
		t.Errorf("a duplicate changed the fingerprint: %q vs %q", got, base)
	}
	if got := listingFingerprint([]string{"b:1", "b:2", "b:3", "b:4"}); got == base {
		t.Error("an added posting did not change the fingerprint")
	}
	if got := listingFingerprint([]string{"b:1", "b:2"}); got == base {
		t.Error("a removed posting did not change the fingerprint")
	}
	if empty := listingFingerprint(nil); empty == "" || empty == base {
		t.Errorf("an empty listing must have its own non-empty fingerprint, got %q", empty)
	}
	// Ids must not run together: {"ab","c"} and {"a","bc"} are different listings.
	if listingFingerprint([]string{"ab", "c"}) == listingFingerprint([]string{"a", "bc"}) {
		t.Error("ids concatenated without a separator")
	}
}

func jobsWithIDs(company string, ids ...string) []sources.Job {
	out := make([]sources.Job, len(ids))
	for i, id := range ids {
		out[i] = sources.Job{ExternalID: id, Title: "Backend Engineer", Company: company}
	}
	return out
}

// A complete buffered crawl records the fingerprint of the ids its listing named.
func TestRunRecordsTheListingFingerprint(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: jobsWithIDs("Acme", "2", "1", "2")}
	health := &fakeHealth{cooldowns: map[string]time.Time{}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Acme", Provider: "greenhouse", Board: "acme"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := health.fingerprints["greenhouse/acme/"]
	if want := listingFingerprint([]string{"1", "2"}); len(got) != 1 || got[0] != want {
		t.Errorf("fingerprints = %v, want [%s]", got, want)
	}
}

// A complete stream records the fingerprint of everything it emitted.
func TestRunRecordsTheStreamFingerprint(t *testing.T) {
	src := fakeStreamingSource{provider: "eightfold", jobs: jobsWithIDs("Acme", "a", "b"), failAfter: -1}
	health := &fakeHealth{cooldowns: map[string]time.Time{}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Acme", Provider: "eightfold", Board: "acme"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := health.fingerprints["eightfold/acme/"]
	if want := listingFingerprint([]string{"a", "b"}); len(got) != 1 || got[0] != want {
		t.Errorf("fingerprints = %v, want [%s]", got, want)
	}
}

// A stream that failed midway saw only part of the listing, so it vouches for nothing.
func TestRunPassesNoFingerprintForAPartialStream(t *testing.T) {
	src := fakeStreamingSource{provider: "eightfold", jobs: jobsWithIDs("Acme", "a", "b", "c"), failAfter: 2}
	health := &fakeHealth{cooldowns: map[string]time.Time{}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Acme", Provider: "eightfold", Board: "acme"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := health.fingerprints["eightfold/acme/"]; len(got) != 1 || got[0] != "" {
		t.Errorf("fingerprints = %q, want one neutral \"\"", got)
	}
}

// A stream the run's deadline cut short is incomplete too, even though it returned no error.
func TestRunPassesNoFingerprintForAStreamTheDeadlineCutShort(t *testing.T) {
	health := &fakeHealth{cooldowns: map[string]time.Time{}}
	r := Runner{Registry: registry(deadlineStreamingSource{provider: "eightfold"}), Store: &fakeStore{}, BoardHealth: health}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, _ = r.Run(ctx, []sources.CompanyEntry{{Company: "Acme", Provider: "eightfold", Board: "acme"}}) // the deadline is the point
	if len(health.successes) != 1 {
		t.Fatalf("successes = %v, want the board recorded once (fixture assumption)", health.successes)
	}
	for _, fp := range health.fingerprints["eightfold/acme/"] {
		if fp != "" {
			t.Errorf("an interrupted stream recorded fingerprint %q, want neutral", fp)
		}
	}
}

// A board in its quiet pause is skipped before its adapter is called, and counted as quiet —
// not as cooled, which means a failing board.
func TestRunSkipsAQuietBoard(t *testing.T) {
	fetches := 0
	src := spySource{provider: "greenhouse", fetches: &fetches}
	health := &fakeHealth{
		cooldowns: map[string]time.Time{},
		quiet:     map[string]time.Time{"greenhouse/calm/": time.Now().Add(time.Hour)},
	}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}
	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Calm", Provider: "greenhouse", Board: "calm"},
		{Company: "Busy", Provider: "greenhouse", Board: "busy"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fetches != 1 {
		t.Errorf("fetches = %d, want 1 (only the board that is not quiet)", fetches)
	}
	got := stats.Total()
	if got.Quiet != 1 || got.Cooled != 0 {
		t.Errorf("Quiet=%d Cooled=%d, want Quiet=1 Cooled=0", got.Quiet, got.Cooled)
	}
}

// A quiet pause that has already ended does not hold the board back.
func TestRunCrawlsABoardWhoseQuietPauseEnded(t *testing.T) {
	fetches := 0
	src := spySource{provider: "greenhouse", fetches: &fetches}
	health := &fakeHealth{
		cooldowns: map[string]time.Time{},
		quiet:     map[string]time.Time{"greenhouse/calm/": time.Now().Add(-time.Minute)},
	}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Calm", Provider: "greenhouse", Board: "calm"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fetches != 1 {
		t.Errorf("fetches = %d, want 1", fetches)
	}
}

// The check fails open: a health store that cannot answer crawls the board.
func TestRunCrawlsWhenPausesCannotBeRead(t *testing.T) {
	fetches := 0
	src := spySource{provider: "greenhouse", fetches: &fetches}
	health := &fakeHealth{cooldowns: map[string]time.Time{}, pausesErr: errors.New("db down")}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Calm", Provider: "greenhouse", Board: "calm"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fetches != 1 {
		t.Errorf("fetches = %d, want 1 (fail open)", fetches)
	}
}

// Every board the run skipped — quiet or cooled — reaches the sweep as its BoardPattern, so the
// company- and source-scoped closes leave its unseen rows alone.
func TestRunReportsSkippedBoardsForTheSweep(t *testing.T) {
	// "broken" really is down, so the recovery probe cannot clear its cooldown before the main loop.
	src := boardKeyedSource{provider: "greenhouse", failBoards: map[string]bool{"broken": true}}
	health := &fakeHealth{
		cooldowns: map[string]time.Time{"greenhouse/broken/": time.Now().Add(time.Hour)},
		quiet:     map[string]time.Time{"greenhouse/calm_one/": time.Now().Add(time.Hour)},
	}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}
	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Calm", Provider: "greenhouse", Board: "calm_one"},
		{Company: "Broken", Provider: "greenhouse", Board: "broken"},
		{Company: "Busy", Provider: "greenhouse", Board: "busy"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := slices.Sorted(slices.Values(stats["greenhouse"].SkippedBoards))
	// The underscore is escaped: unescaped, LIKE would read it as "any one character".
	want := []string{"broken:%", `calm\_one:%`}
	if !slices.Equal(got, want) {
		t.Errorf("SkippedBoards = %q, want %q", got, want)
	}
}

// deferringSource is a hydrating adapter that asks seen() about every posting it lists but only
// returns the new ones it had budget to hydrate — the shape of every detail-budgeted adapter.
type deferringSource struct {
	provider string
	listed   []string
	hydrate  int // how many NEW postings it returns before its budget runs out
}

func (s deferringSource) Provider() string { return s.provider }
func (s deferringSource) Fetch(ctx context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}
func (s deferringSource) FetchNew(_ context.Context, e sources.CompanyEntry, seen func(string) bool) ([]sources.Job, error) {
	var out []sources.Job
	budget := s.hydrate
	for _, id := range s.listed {
		if seen(id) {
			out = append(out, sources.Job{ExternalID: id, Title: "Backend Engineer", Company: e.Company, SeenRefresh: true})
			continue
		}
		if budget == 0 {
			continue // deferred to the next run, silently — as the real adapters do
		}
		budget--
		out = append(out, sources.Job{ExternalID: id, Title: "Backend Engineer", Company: e.Company})
	}
	return out, nil
}

// A crawl that left new postings un-hydrated has work pending, so it must not count as quiet:
// otherwise a board whose budget keeps running out looks unchanged and its backlog starves.
func TestRunPassesNoFingerprintWhenNewPostingsWereDeferred(t *testing.T) {
	src := deferringSource{provider: "seek", listed: []string{"1", "2", "3"}, hydrate: 1}
	health := &fakeHealth{cooldowns: map[string]time.Time{}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Acme", Provider: "seek", Board: "6287"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := health.fingerprints["seek/6287/"]; len(got) != 1 || got[0] != "" {
		t.Errorf("fingerprints = %q, want one neutral \"\" for a crawl that deferred postings", got)
	}
}

// A hydrating crawl that hydrated every new posting is complete, and the fingerprint covers
// every id it listed — the seen ones included.
func TestRunFingerprintsACompleteHydratingCrawl(t *testing.T) {
	src := deferringSource{provider: "seek", listed: []string{"1", "2"}, hydrate: 2}
	store := &fakeStore{seenByBrd: map[string]map[string]bool{"6287": {"6287:1": true}}}
	health := &fakeHealth{cooldowns: map[string]time.Time{}}
	r := Runner{Registry: registry(src), Store: store, BoardHealth: health}
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Acme", Provider: "seek", Board: "6287"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := health.fingerprints["seek/6287/"], listingFingerprint([]string{"1", "2"}); len(got) != 1 || got[0] != want {
		t.Errorf("fingerprints = %q, want [%s]", got, want)
	}
}

// A posting the listing named but whose detail could not be read is retried every cycle; a
// quiet pause would postpone that retry, so the crawl vouches for nothing.
func TestRunPassesNoFingerprintWhenAPostingWasUnreadable(t *testing.T) {
	jobs := jobsWithIDs("Acme", "1", "2")
	jobs = append(jobs, sources.Job{ExternalID: "3", Unreadable: true})
	src := fakeSource{provider: "greenhouse", jobs: jobs}
	health := &fakeHealth{cooldowns: map[string]time.Time{}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Acme", Provider: "greenhouse", Board: "acme"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := health.fingerprints["greenhouse/acme/"]; len(got) != 1 || got[0] != "" {
		t.Errorf("fingerprints = %q, want neutral for a crawl with an unreadable posting", got)
	}
}

// A posting that failed to save is retried by the next crawl only if one happens soon.
func TestRunPassesNoFingerprintWhenASaveFailed(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: jobsWithIDs("Acme", "1")}
	health := &fakeHealth{cooldowns: map[string]time.Time{}}
	r := Runner{Registry: registry(src), Store: &fakeStore{err: errors.New("db down")}, BoardHealth: health}
	_, _ = r.Run(context.Background(), []sources.CompanyEntry{{Company: "Acme", Provider: "greenhouse", Board: "acme"}})
	if got := health.fingerprints["greenhouse/acme/"]; len(got) != 1 || got[0] != "" {
		t.Errorf("fingerprints = %q, want neutral for a crawl whose save failed", got)
	}
}
