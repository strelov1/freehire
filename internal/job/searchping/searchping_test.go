package searchping

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepo struct {
	candidates []Candidate
	sentToday  int64
	recorded   []int64
	listLimit  int32
	recordErr  error
}

func (f *fakeRepo) JobsToPing(_ context.Context, _ string, limit int32) ([]Candidate, error) {
	f.listLimit = limit
	if int(limit) < len(f.candidates) {
		return f.candidates[:limit], nil
	}
	return f.candidates, nil
}

func (f *fakeRepo) RecordPing(_ context.Context, jobID int64, _ string) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	f.recorded = append(f.recorded, jobID)
	return nil
}

func (f *fakeRepo) PingsSince(_ context.Context, _ string, _ time.Time) (int64, error) {
	return f.sentToday, nil
}

type fakeEngine struct {
	name     string
	budget   int
	offered  []string
	accept   int // how many of the offered URLs to accept
	err      error
	callSeen bool
}

func (f *fakeEngine) Name() string     { return f.name }
func (f *fakeEngine) DailyBudget() int { return f.budget }

func (f *fakeEngine) Announce(_ context.Context, urls []string) ([]string, error) {
	f.callSeen = true
	f.offered = urls
	n := f.accept
	if n > len(urls) {
		n = len(urls)
	}
	return urls[:n], f.err
}

func candidates(n int) []Candidate {
	out := make([]Candidate, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, Candidate{JobID: int64(i), Slug: "job-" + string(rune('a'+i-1))})
	}
	return out
}

func TestRunAnnouncesAndRecords(t *testing.T) {
	repo := &fakeRepo{candidates: candidates(3)}
	engine := &fakeEngine{name: "test", accept: 3}
	runner := New(repo, "https://freehire.me/", engine)

	reports := runner.Run(context.Background(), 10)

	if len(reports) != 1 {
		t.Fatalf("want one report, got %d", len(reports))
	}
	r := reports[0]
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if r.Offered != 3 || r.Accepted != 3 || r.Recorded != 3 {
		t.Fatalf("offered/accepted/recorded = %d/%d/%d, want 3/3/3", r.Offered, r.Accepted, r.Recorded)
	}
	// The trailing slash on the origin must not survive into the URL — a doubled slash
	// is a different URL to a search engine, and spends budget teaching it one.
	if got := engine.offered[0]; got != "https://freehire.me/jobs/job-a" {
		t.Fatalf("first url = %q", got)
	}
}

// The ledger must record what an engine TOOK, never what it was offered: a partial
// batch that is recorded in full silently drops the postings that were not sent, and
// they are never selected again.
func TestRunRecordsOnlyWhatTheEngineAccepted(t *testing.T) {
	repo := &fakeRepo{candidates: candidates(3)}
	engine := &fakeEngine{name: "test", accept: 2}
	runner := New(repo, "https://freehire.me", engine)

	r := runner.Run(context.Background(), 10)[0]

	if r.Recorded != 2 {
		t.Fatalf("recorded = %d, want 2", r.Recorded)
	}
	if len(repo.recorded) != 2 || repo.recorded[0] != 1 || repo.recorded[1] != 2 {
		t.Fatalf("recorded job ids = %v, want [1 2]", repo.recorded)
	}
}

// A send that happened and was not written down costs the budget twice — so the
// accepted prefix is recorded even when the batch as a whole failed.
func TestRunRecordsAcceptedEvenWhenTheBatchFailed(t *testing.T) {
	repo := &fakeRepo{candidates: candidates(3)}
	engine := &fakeEngine{name: "test", accept: 1, err: errors.New("quota exhausted")}
	runner := New(repo, "https://freehire.me", engine)

	r := runner.Run(context.Background(), 10)[0]

	if r.Err == nil {
		t.Fatal("want the engine's error to survive")
	}
	if r.Recorded != 1 {
		t.Fatalf("recorded = %d, want 1 — the accepted url must be written down", r.Recorded)
	}
}

// The budget is a DAY's, not a run's. The timer fires more than once a day, so a run
// that assumed the whole allowance would spend it again every hour.
func TestRunBoundsTheBatchByWhatIsLeftOfTheDay(t *testing.T) {
	repo := &fakeRepo{candidates: candidates(10), sentToday: 197}
	engine := &fakeEngine{name: "google", budget: 200, accept: 10}
	runner := New(repo, "https://freehire.me", engine)

	r := runner.Run(context.Background(), 50)[0]

	if repo.listLimit != 3 {
		t.Fatalf("asked for %d candidates, want 3 (200 budget - 197 already sent)", repo.listLimit)
	}
	if r.Offered != 3 {
		t.Fatalf("offered = %d, want 3", r.Offered)
	}
}

func TestRunSendsNothingOnceTheDayIsSpent(t *testing.T) {
	repo := &fakeRepo{candidates: candidates(10), sentToday: 200}
	engine := &fakeEngine{name: "google", budget: 200, accept: 10}
	runner := New(repo, "https://freehire.me", engine)

	r := runner.Run(context.Background(), 50)[0]

	if engine.callSeen {
		t.Fatal("the engine must not be called once the day's budget is spent")
	}
	if r.Offered != 0 || r.Remaining != 0 {
		t.Fatalf("offered=%d remaining=%d, want 0/0", r.Offered, r.Remaining)
	}
}

// An unbounded engine reports -1 rather than 0, because 0 is a real state for a bounded
// one ("spent") and a log that prints the same word for both hides it.
func TestUnboundedEngineIsNotConfusedWithASpentOne(t *testing.T) {
	repo := &fakeRepo{candidates: candidates(2)}
	engine := &fakeEngine{name: "indexnow", budget: 0, accept: 2}

	r := New(repo, "https://freehire.me", engine).Run(context.Background(), 50)[0]

	if r.Remaining != -1 {
		t.Fatalf("remaining = %d, want -1 for an unbounded engine", r.Remaining)
	}
}

// One engine's failure is not another's: they are independent services.
func TestOneEngineFailingDoesNotStopAnother(t *testing.T) {
	repo := &fakeRepo{candidates: candidates(2)}
	broken := &fakeEngine{name: "broken", accept: 0, err: errors.New("boom")}
	working := &fakeEngine{name: "working", accept: 2}

	reports := New(repo, "https://freehire.me", broken, working).Run(context.Background(), 10)

	if len(reports) != 2 {
		t.Fatalf("want two reports, got %d", len(reports))
	}
	if reports[0].Err == nil || reports[1].Err != nil {
		t.Fatalf("errors = %v / %v, want the first only", reports[0].Err, reports[1].Err)
	}
	if reports[1].Recorded != 2 {
		t.Fatalf("second engine recorded %d, want 2", reports[1].Recorded)
	}
}

func TestPreviewSendsNothing(t *testing.T) {
	repo := &fakeRepo{candidates: candidates(2)}
	engine := &fakeEngine{name: "test", accept: 2}

	r := New(repo, "https://freehire.me", engine).Preview(context.Background(), 10)[0]

	if engine.callSeen {
		t.Fatal("preview must not call the engine")
	}
	if len(repo.recorded) != 0 {
		t.Fatalf("preview recorded %v, want nothing", repo.recorded)
	}
	// The URLs, not a count: a dry run exists to catch an address that is wrong.
	if len(r.URLs) != 2 || r.URLs[0] != "https://freehire.me/jobs/job-a" {
		t.Fatalf("urls = %v", r.URLs)
	}
}

// Google's quota day resets at midnight Pacific. Measuring it in UTC would let a run
// just after 00:00 UTC spend an allowance Google still counts against yesterday — for
// most of the year, that is a whole extra day's budget every day.
func TestBudgetDayStartIsPacificNotUTC(t *testing.T) {
	// 03:00 UTC on 10 September is still 20:00 on the 9th in Los Angeles.
	now := time.Date(2026, 9, 10, 3, 0, 0, 0, time.UTC)

	start := budgetDayStart(now)

	if got := start.In(pacific).Day(); got != 9 {
		t.Fatalf("budget day started on the %dth, want the 9th", got)
	}
	if h, m := start.In(pacific).Hour(), start.In(pacific).Minute(); h != 0 || m != 0 {
		t.Fatalf("budget day starts at %02d:%02d Pacific, want midnight", h, m)
	}
}
