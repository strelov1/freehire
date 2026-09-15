package searchping

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepo struct {
	candidates    []Candidate
	closed        []Candidate
	sentToday     int64
	recorded      []int64
	recordedAs    []Kind
	listLimit     int32
	closededLimit int32
	recordErr     error
}

func take(all []Candidate, limit int32) []Candidate {
	if int(limit) < len(all) {
		return all[:limit]
	}
	return all
}

func (f *fakeRepo) JobsToPing(_ context.Context, _ string, limit int32) ([]Candidate, error) {
	f.listLimit = limit
	return take(f.candidates, limit), nil
}

func (f *fakeRepo) ClosedJobsToPing(_ context.Context, _ string, limit int32) ([]Candidate, error) {
	f.closededLimit = limit
	return take(f.closed, limit), nil
}

func (f *fakeRepo) RecordPing(_ context.Context, jobID int64, _ string, kind Kind) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	f.recorded = append(f.recorded, jobID)
	f.recordedAs = append(f.recordedAs, kind)
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

// pick is the report for one engine's one pass. Every run now produces a report per
// (engine, event), so a test that indexed by position would quietly assert about the
// wrong pass the next time the order changes.
func pick(t *testing.T, reports []Report, engine string, kind Kind) Report {
	t.Helper()
	for _, r := range reports {
		if r.Engine == engine && r.Kind == kind {
			return r
		}
	}
	t.Fatalf("no report for %s/%s in %d reports", engine, kind, len(reports))
	return Report{}
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

	r := pick(t, reports, "test", KindCreated)
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

	r := pick(t, runner.Run(context.Background(), 10), "test", KindCreated)

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

	r := pick(t, runner.Run(context.Background(), 10), "test", KindCreated)

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

	r := pick(t, runner.Run(context.Background(), 50), "google", KindCreated)

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

	r := pick(t, runner.Run(context.Background(), 50), "google", KindCreated)

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

	r := pick(t, New(repo, "https://freehire.me", engine).Run(context.Background(), 50), "indexnow", KindCreated)

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

	brokenReport := pick(t, reports, "broken", KindCreated)
	workingReport := pick(t, reports, "working", KindCreated)
	if brokenReport.Err == nil || workingReport.Err != nil {
		t.Fatalf("errors = %v / %v, want the first only", brokenReport.Err, workingReport.Err)
	}
	if workingReport.Recorded != 2 {
		t.Fatalf("second engine recorded %d, want 2", workingReport.Recorded)
	}
}

func TestPreviewSendsNothing(t *testing.T) {
	repo := &fakeRepo{candidates: candidates(2)}
	engine := &fakeEngine{name: "test", accept: 2}

	r := pick(t, New(repo, "https://freehire.me", engine).Preview(context.Background(), 10), "test", KindCreated)

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

// The zone must carry its daylight saving rule. A fixed -8 offset standing in for
// Pacific puts the summer boundary an hour LATE, so pings sent in that hour go
// uncounted while Google counts them — the run reads more allowance left than it has
// and overspends. The failure is silent, which is why it is pinned to an instant.
func TestBudgetDayFollowsDaylightSaving(t *testing.T) {
	summer := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC) // PDT, UTC-7
	winter := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC) // PST, UTC-8

	if got := budgetDayStart(summer).UTC(); !got.Equal(time.Date(2026, 7, 15, 7, 0, 0, 0, time.UTC)) {
		t.Fatalf("summer budget day starts at %s, want 07:00 UTC (midnight PDT)", got)
	}
	if got := budgetDayStart(winter).UTC(); !got.Equal(time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("winter budget day starts at %s, want 08:00 UTC (midnight PST)", got)
	}
}

// New postings come first, and the closure pass gets what is left. This is the whole
// reason the two events share one allowance rather than each having their own: a new
// posting is what brings a visitor, a closure only tidies an index we do not own.
func TestNewPostingsTakeTheBudgetBeforeClosures(t *testing.T) {
	repo := &fakeRepo{candidates: candidates(10), closed: candidates(10), sentToday: 197}
	engine := &fakeEngine{name: "google", budget: 200, accept: 10}

	reports := New(repo, "https://freehire.me", engine).Run(context.Background(), 50)

	created := pick(t, reports, "google", KindCreated)
	closed := pick(t, reports, "google", KindClosed)
	if created.Recorded != 3 {
		t.Fatalf("created recorded %d, want the 3 left of the day", created.Recorded)
	}
	if closed.Offered != 0 || closed.Recorded != 0 {
		t.Fatalf("closures offered=%d recorded=%d, want nothing — the day was spent on new postings",
			closed.Offered, closed.Recorded)
	}
}

func TestClosuresGetWhatTheNewPostingsLeave(t *testing.T) {
	// Two new postings against a budget of five leaves three for the closure pass.
	repo := &fakeRepo{candidates: candidates(2), closed: candidates(10), sentToday: 195}
	engine := &fakeEngine{name: "google", budget: 200, accept: 10}

	reports := New(repo, "https://freehire.me", engine).Run(context.Background(), 50)

	if got := pick(t, reports, "google", KindCreated).Recorded; got != 2 {
		t.Fatalf("created recorded %d, want 2", got)
	}
	if got := pick(t, reports, "google", KindClosed).Recorded; got != 3 {
		t.Fatalf("closures recorded %d, want the 3 the new postings left", got)
	}
	if repo.closededLimit != 3 {
		t.Fatalf("asked for %d closures, want 3", repo.closededLimit)
	}
}

// An unbounded engine is not spending anything shared, so its closure pass gets a full
// batch rather than the leftovers of the pass before — otherwise IndexNow, which has no
// quota at all, would silently stop announcing closures as soon as new postings filled
// one batch.
func TestUnboundedEngineGivesEachPassAFullBatch(t *testing.T) {
	repo := &fakeRepo{candidates: candidates(10), closed: candidates(10)}
	engine := &fakeEngine{name: "indexnow", budget: 0, accept: 10}

	reports := New(repo, "https://freehire.me", engine).Run(context.Background(), 10)

	if got := pick(t, reports, "indexnow", KindCreated).Recorded; got != 10 {
		t.Fatalf("created recorded %d, want 10", got)
	}
	if got := pick(t, reports, "indexnow", KindClosed).Recorded; got != 10 {
		t.Fatalf("closures recorded %d, want a full batch of its own", got)
	}
}

// The ledger has to record WHICH event, or a closure would look like the announcement
// the posting already had and would never be selected — the row for 'created' is what
// the closure query joins against.
func TestTheLedgerRecordsWhichEvent(t *testing.T) {
	repo := &fakeRepo{candidates: candidates(1), closed: candidates(1)}
	engine := &fakeEngine{name: "indexnow", budget: 0, accept: 1}

	New(repo, "https://freehire.me", engine).Run(context.Background(), 10)

	if len(repo.recordedAs) != 2 {
		t.Fatalf("recorded %d pings, want 2", len(repo.recordedAs))
	}
	if repo.recordedAs[0] != KindCreated || repo.recordedAs[1] != KindClosed {
		t.Fatalf("recorded kinds = %v, want [created closed]", repo.recordedAs)
	}
}

func TestPreviewShowsBothPasses(t *testing.T) {
	repo := &fakeRepo{candidates: candidates(2), closed: candidates(3)}
	engine := &fakeEngine{name: "indexnow", budget: 0, accept: 5}

	reports := New(repo, "https://freehire.me", engine).Preview(context.Background(), 10)

	if engine.callSeen {
		t.Fatal("preview must not call the engine")
	}
	if got := len(pick(t, reports, "indexnow", KindCreated).URLs); got != 2 {
		t.Fatalf("created preview showed %d urls, want 2", got)
	}
	if got := len(pick(t, reports, "indexnow", KindClosed).URLs); got != 3 {
		t.Fatalf("closure preview showed %d urls, want 3", got)
	}
}

// A call is spent when the ENGINE takes the URL, not when the ledger write that follows
// succeeds. Charging the next pass for recorded announcements only would let a failed
// write hand the closure pass budget Google has already counted against the day.
func TestBudgetIsChargedForAcceptedNotRecorded(t *testing.T) {
	repo := &fakeRepo{
		candidates: candidates(3),
		closed:     candidates(10),
		sentToday:  195, // five left of a 200 budget
		recordErr:  errors.New("database is down"),
	}
	engine := &fakeEngine{name: "google", budget: 200, accept: 3}

	reports := New(repo, "https://freehire.me", engine).Run(context.Background(), 50)

	created := pick(t, reports, "google", KindCreated)
	if created.Accepted != 3 || created.Recorded != 0 {
		t.Fatalf("created accepted=%d recorded=%d, want 3/0", created.Accepted, created.Recorded)
	}
	if got := pick(t, reports, "google", KindClosed).Offered; got != 2 {
		t.Fatalf("closures offered %d, want 2 — the 3 accepted calls are spent even though none were recorded", got)
	}
}
