package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/sources"
)

// fakeHealth records the outcome calls the Runner makes and serves canned cooldowns.
type fakeHealth struct {
	cooldowns map[string]time.Time // "provider/board" → cooldown_until
	successes []string
	failures  []string
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

// spySource records whether Fetch was called, to prove a cooled board is never crawled.
type spySource struct {
	provider string
	fetched  *bool
}

func (s spySource) Provider() string { return s.provider }
func (s spySource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	*s.fetched = true
	return []sources.Job{{ExternalID: "1", Title: "Dev", Company: "C"}}, nil
}

// A board whose cooldown_until is in the future is skipped before its adapter is
// invoked: Fetch is never called, it is counted Cooled (not Failed), and no outcome
// is recorded (a skip is not an outcome).
func TestRunSkipsCooledBoard(t *testing.T) {
	fetched := false
	src := spySource{provider: "greenhouse", fetched: &fetched}
	health := &fakeHealth{cooldowns: map[string]time.Time{
		"greenhouse/acme": time.Now().Add(6 * time.Hour),
	}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "greenhouse", Board: "acme"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fetched {
		t.Error("cooled board must not be fetched")
	}
	if stats.Total().Cooled != 1 || stats.Total().Failed != 0 || stats.Total().Ingested != 0 {
		t.Errorf("stats = %+v, want Cooled=1 Failed=0 Ingested=0", stats.Total())
	}
	if len(health.successes) != 0 || len(health.failures) != 0 {
		t.Errorf("a cooled skip records no outcome; got successes=%v failures=%v", health.successes, health.failures)
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
