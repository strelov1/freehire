package pipeline

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/sources"
)

// fakeHealth records the outcome calls the Runner makes and serves canned cooldowns. Its
// cooldowns map is the single source of truth for both the recovery probe's candidates
// (any board with a future cooldown) and the per-board gate, so ClearCooldowns removing a
// provider's entries lets the subsequent crawl actually reach those boards — modelling the
// real DB round-trip end to end.
type fakeHealth struct {
	cooldowns map[string]time.Time // "provider/board" → cooldown_until
	successes []string
	failures  []string
	cleared   []string // providers passed to ClearCooldowns, in call order
}

func (f *fakeHealth) Cooldown(_ context.Context, provider, board string) (time.Time, bool, error) {
	t, ok := f.cooldowns[provider+"/"+board]
	return t, ok, nil
}

func (f *fakeHealth) RecordSuccess(_ context.Context, provider, board string, _ int) error {
	f.successes = append(f.successes, provider+"/"+board)
	return nil
}

func (f *fakeHealth) RecordFailure(_ context.Context, provider, board, _ string) error {
	f.failures = append(f.failures, provider+"/"+board)
	return nil
}

// CooledBoards serves up to limit boards of the provider whose canned cooldown is still in
// the future, soonest-to-expire first — mirroring ListCooledBoards.
func (f *fakeHealth) CooledBoards(_ context.Context, provider string, limit int) ([]string, error) {
	type cand struct {
		board string
		until time.Time
	}
	var cands []cand
	for k, until := range f.cooldowns {
		p, board, ok := strings.Cut(k, "/")
		if !ok || p != provider || !until.After(time.Now()) {
			continue
		}
		cands = append(cands, cand{board, until})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].until.Equal(cands[j].until) {
			return cands[i].board < cands[j].board
		}
		return cands[i].until.Before(cands[j].until)
	})
	boards := make([]string, 0, limit)
	for _, c := range cands {
		if len(boards) == limit {
			break
		}
		boards = append(boards, c.board)
	}
	return boards, nil
}

// ClearCooldowns drops the provider's cooldown entries (so the gate then treats those
// boards as eligible) and records the call.
func (f *fakeHealth) ClearCooldowns(_ context.Context, provider string) (int, error) {
	f.cleared = append(f.cleared, provider)
	n := 0
	for k := range f.cooldowns {
		if p, _, ok := strings.Cut(k, "/"); ok && p == provider {
			delete(f.cooldowns, k)
			n++
		}
	}
	return n, nil
}

// boardKeyedSource answers every board except those named in failBoards, which return an
// error — so a test can place a genuinely-dead board among a provider's cooled set and
// prove the probe tries past it.
type boardKeyedSource struct {
	provider   string
	failBoards map[string]bool
}

func (s boardKeyedSource) Provider() string { return s.provider }
func (s boardKeyedSource) Fetch(_ context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	if s.failBoards[e.Board] {
		return nil, errors.New("board down")
	}
	return []sources.Job{{ExternalID: "1", Title: "Dev", Company: e.Company}}, nil
}

// spySource counts Fetch calls (and can error), so a test can tell a probe fetch from a
// main-loop crawl of the same board.
type spySource struct {
	provider string
	fetches  *int
	err      error
}

func (s spySource) Provider() string { return s.provider }
func (s spySource) Fetch(_ context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	*s.fetches++
	if s.err != nil {
		return nil, s.err
	}
	return []sources.Job{{ExternalID: "1", Title: "Dev", Company: e.Company}}, nil
}

// A single-board provider is never probed: probing it would crawl the whole provider (for a
// boardless aggregator, its entire dataset) only for the main loop to crawl it again, and there
// are no other boards to clear. So even with a healthy adapter, recovery leaves the lone cooled
// board untouched — the adapter is never called — and the normal cooldown gate recovers it at
// expiry instead. This is the guard against the boardless double-crawl.
func TestRecoverSkipsSingleBoardProvider(t *testing.T) {
	fetches := 0
	src := spySource{provider: "gulftalent", fetches: &fetches} // healthy, but must not be probed
	health := &fakeHealth{cooldowns: map[string]time.Time{
		"gulftalent/": time.Now().Add(24 * time.Hour), // one boardless entry, cooled
	}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "GulfTalent", Provider: "gulftalent", Board: ""},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fetches != 0 {
		t.Errorf("adapter fetched %d times, want 0 (a single-board provider must not be probed)", fetches)
	}
	if len(health.cleared) != 0 {
		t.Errorf("a single-board provider must not be cleared by a probe; cleared=%v", health.cleared)
	}
	if stats.Total().Cooled != 1 || stats.Total().Ingested != 0 {
		t.Errorf("stats = %+v, want Cooled=1 Ingested=0 (left cooled for the normal gate)", stats.Total())
	}
}

// A still-down multi-board provider is never cleared or stampeded: every probe fails, so the
// cooldowns stand and the main loop skips the cooled boards.
func TestRecoverLeavesDownMultiBoardProviderCooled(t *testing.T) {
	fetches := 0
	src := spySource{provider: "workday", fetches: &fetches, err: errors.New("provider down")}
	health := &fakeHealth{cooldowns: map[string]time.Time{
		"workday/a": time.Now().Add(6 * time.Hour),
		"workday/b": time.Now().Add(6 * time.Hour),
	}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "A", Provider: "workday", Board: "a"},
		{Company: "B", Provider: "workday", Board: "b"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(health.cleared) != 0 {
		t.Errorf("a down provider must not be cleared; cleared=%v", health.cleared)
	}
	if stats.Total().Cooled != 2 || stats.Total().Ingested != 0 {
		t.Errorf("stats = %+v, want Cooled=2 Ingested=0 (both stay cooled, main loop skips them)", stats.Total())
	}
}

// A multi-board provider whose boards were mass-cooled by a since-resolved outage recovers this
// cycle: the pre-crawl probe reaches a cooled board, clears the provider's cooldowns, and the
// main loop then crawls the boards it would otherwise have skipped. Without recovery they stay
// Cooled/Ingested=0, so this result is unique to the half-open transition.
func TestRecoverProbeRecoversProvider(t *testing.T) {
	fetches := 0
	src := spySource{provider: "breezy", fetches: &fetches}
	health := &fakeHealth{cooldowns: map[string]time.Time{
		"breezy/acme": time.Now().Add(24 * time.Hour),
		"breezy/beta": time.Now().Add(24 * time.Hour),
	}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "breezy", Board: "acme"},
		{Company: "Beta", Provider: "breezy", Board: "beta"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(health.cleared) != 1 || health.cleared[0] != "breezy" {
		t.Errorf("cleared = %v, want [breezy] (a successful probe recovers the provider)", health.cleared)
	}
	if stats.Total().Ingested != 2 || stats.Total().Cooled != 0 {
		t.Errorf("stats = %+v, want Ingested=2 Cooled=0 (both boards crawled after the probe cleared them)", stats.Total())
	}
}

// One genuinely-dead board among the cooled set must not mask a recovered provider: the
// probe tries past the dead candidate to a live one, then clears. This exercises
// maxRecoveryProbes > 1 — with a single probe (the dead board, first by cooldown order)
// the provider would never recover.
func TestRecoverProbeTriesPastDeadBoard(t *testing.T) {
	src := boardKeyedSource{provider: "join", failBoards: map[string]bool{"dead": true}}
	health := &fakeHealth{cooldowns: map[string]time.Time{
		"join/dead": time.Now().Add(1 * time.Hour),  // probed first (soonest to expire)
		"join/live": time.Now().Add(12 * time.Hour), // probed second
	}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Dead", Provider: "join", Board: "dead"},
		{Company: "Live", Provider: "join", Board: "live"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(health.cleared) != 1 || health.cleared[0] != "join" {
		t.Errorf("cleared = %v, want [join] (probe must try past the dead board to the live one)", health.cleared)
	}
	// After clearing, the main loop crawls both: the live board ingests, the dead one
	// fails — but the provider recovered, which the per-board backoff alone could not do.
	if stats.Total().Ingested != 1 || stats.Total().Cooled != 0 {
		t.Errorf("stats = %+v, want Ingested=1 Cooled=0", stats.Total())
	}
	if len(health.successes) != 1 || health.successes[0] != "join/live" {
		t.Errorf("successes = %v, want [join/live]", health.successes)
	}
}

// A crawl that succeeds records success; an unknown provider or a fetch error records
// failure — the signals the cooldown backoff runs on.
func TestRunRecordsBoardOutcome(t *testing.T) {
	good := fakeSource{provider: "greenhouse", jobs: []sources.Job{{ExternalID: "1", Title: "Dev", Company: "C"}}}
	bad := fakeSource{provider: "lever", err: errors.New("boom")}
	health := &fakeHealth{cooldowns: map[string]time.Time{}}
	r := Runner{Registry: registry(good, bad), Store: &fakeStore{}, BoardHealth: health}

	_, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Good", Provider: "greenhouse", Board: "good"},
		{Company: "Bad", Provider: "lever", Board: "bad"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(health.successes) != 1 || health.successes[0] != "greenhouse/good" {
		t.Errorf("successes = %v, want [greenhouse/good]", health.successes)
	}
	if len(health.failures) != 1 || health.failures[0] != "lever/bad" {
		t.Errorf("failures = %v, want [lever/bad]", health.failures)
	}
}

// A nil BoardHealth port keeps today's behavior: no cooldown checks, no recording.
func TestRunWithoutBoardHealth(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{{ExternalID: "1", Title: "Dev", Company: "C"}}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}} // BoardHealth nil
	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Total().Ingested != 1 || stats.Total().Cooled != 0 {
		t.Errorf("stats = %+v, want Ingested=1 Cooled=0", stats.Total())
	}
}
