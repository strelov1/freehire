package autoapply

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/hardconstraint"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeQueue is an in-memory EnqueueStore. existing is the pair's queue row before the call,
// when there is one; every write the use case attempts is counted, and the *Err fields make
// the matching read fail.
type fakeQueue struct {
	applied     bool
	existing    *db.GetAutoApplyQueueEntryForJobRow
	stage       pgtype.Text
	appliedErr  error
	existingErr error
	insertErr   error

	inserts int
	reads   int
}

func (q *fakeQueue) GetUserJobApplied(context.Context, db.GetUserJobAppliedParams) (bool, error) {
	return q.applied, q.appliedErr
}

func (q *fakeQueue) EnqueueAutoApply(context.Context, db.EnqueueAutoApplyParams) (int64, error) {
	q.inserts++
	switch {
	case q.insertErr != nil:
		return 0, q.insertErr
	case q.existing != nil:
		return 0, pgx.ErrNoRows // the pair is already queued: ON CONFLICT DO NOTHING
	}
	return 77, nil
}

func (q *fakeQueue) GetAutoApplyQueueEntryForJob(context.Context, db.GetAutoApplyQueueEntryForJobParams) (db.GetAutoApplyQueueEntryForJobRow, error) {
	q.reads++
	if q.existingErr != nil {
		return db.GetAutoApplyQueueEntryForJobRow{}, q.existingErr
	}
	if q.existing == nil {
		return db.GetAutoApplyQueueEntryForJobRow{}, pgx.ErrNoRows
	}
	return *q.existing, nil
}

func (q *fakeQueue) GetApplicationStage(context.Context, db.GetApplicationStageParams) (pgtype.Text, error) {
	return q.stage, nil
}

// meter records the charge and answers with its fields.
type meter struct {
	decision plan.Decision
	err      error
	ref      string
	used     bool
}

func (m *meter) Consume(_ context.Context, _ int64, _ plan.Feature, ref string) (plan.Decision, error) {
	m.ref, m.used = ref, true
	return m.decision, m.err
}

// board records the tracking call.
type board struct {
	tracked *string
	err     error
}

func (b *board) Track(_ context.Context, _ int64, _ string, stage, _ *string, _ string) error {
	b.tracked = stage
	return b.err
}

// hasCV says whether the candidate has a base CV and a résumé to seed one from.
type hasCV struct{ base, resume bool }

func (s hasCV) baseCV(context.Context, int64) (bool, error) { return s.base, nil }

func (s hasCV) Structured(context.Context, int64) (resumeextract.Structured, bool, error) {
	return resumeextract.Structured{}, s.resume, nil
}

// resumeFault is a Seeder whose read fails.
type resumeFault struct{ err error }

func (f resumeFault) Structured(context.Context, int64) (resumeextract.Structured, bool, error) {
	return resumeextract.Structured{}, false, f.err
}

// startFixture is an enqueuer whose every gate passes, over a fake store, with the ports
// the tests inspect.
type startFixture struct {
	e         *Enqueuer
	q         *fakeQueue
	m         *meter
	b         *board
	published []int64
}

// newStart builds the fixture; the job is Greenhouse #42 unless a test replaces FindJob.
func newStart() *startFixture {
	f := &startFixture{q: &fakeQueue{}, m: &meter{}, b: &board{}}
	cvGate := hasCV{base: true}
	f.e = &Enqueuer{
		FindJob: func(context.Context, string) (db.Job, error) {
			return db.Job{ID: 42, Source: "greenhouse", PublicSlug: "acme-1"}, nil
		},
		Queue:         f.q,
		BaseCV:        cvGate.baseCV,
		Seeder:        cvGate,
		Plans:         f.m,
		Tracker:       f.b,
		PublishSubmit: func(_ context.Context, id int64) { f.published = append(f.published, id) },
	}
	return f
}

func (f *startFixture) start(t *testing.T) error {
	t.Helper()
	return f.e.Start(context.Background(), 7, "acme-1")
}

// nothingHappened fails the test when a refused or failed start charged, wrote or published.
func (f *startFixture) nothingHappened(t *testing.T) {
	t.Helper()
	if f.m.used || f.q.inserts != 0 || f.q.reads != 0 || len(f.published) != 0 {
		t.Errorf("a refused start did something (charged %v, inserts %d, reads %d, published %v)",
			f.m.used, f.q.inserts, f.q.reads, f.published)
	}
}

func refusalReason(t *testing.T, err error) (RefusalReason, string) {
	t.Helper()
	var r *Refusal
	if !errors.As(err, &r) {
		t.Fatalf("err = %v, want *Refusal", err)
	}
	return r.Reason, r.Detail
}

// The gate order is part of the contract: an earlier refusal must cost nothing, so neither
// the allowance nor the queue may be touched before every earlier gate has passed. The
// eligibility row also pins that the refusal carries the blocker's own reason.
func TestStart_GatesInOrderAndCostNothingBeforeTheCharge(t *testing.T) {
	t.Setenv("AUTO_APPLY_ELIGIBILITY_ENFORCE", "1")
	for _, tc := range []struct {
		name     string
		source   string
		applied  bool
		cv       hasCV
		blockers []hardconstraint.Blocker
		want     RefusalReason
		wantText string
	}{
		{name: "unsupported source", source: "smartrecruiters", cv: hasCV{base: true}, want: RefusalUnsupportedSource},
		{name: "no base CV and no résumé", source: "greenhouse", cv: hasCV{}, want: RefusalNoResume},
		{name: "already applied", source: "greenhouse", cv: hasCV{base: true}, applied: true, want: RefusalAlreadyApplied},
		{
			name: "eligibility mismatch", source: "greenhouse", cv: hasCV{base: true},
			blockers: []hardconstraint.Blocker{{Category: hardconstraint.CategoryWorkAuth, Reason: "no visa sponsorship"}},
			want:     RefusalEligibilityMismatch, wantText: "no visa sponsorship",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newStart()
			f.q.applied = tc.applied
			f.e.FindJob = func(context.Context, string) (db.Job, error) {
				return db.Job{ID: 42, Source: tc.source, PublicSlug: "acme-1"}, nil
			}
			f.e.BaseCV, f.e.Seeder = tc.cv.baseCV, tc.cv
			f.e.Blockers = func(context.Context, int64, db.Job) []hardconstraint.Blocker { return tc.blockers }
			got, detail := refusalReason(t, f.start(t))
			if got != tc.want || detail != tc.wantText {
				t.Errorf("refusal = %q %q, want %q %q", got, detail, tc.want, tc.wantText)
			}
			f.nothingHappened(t)
		})
	}
}

// In shadow mode (the default) the same mismatch is only logged and the attempt is enqueued.
func TestStart_EligibilityMismatchInShadowModeEnqueues(t *testing.T) {
	t.Setenv("AUTO_APPLY_ELIGIBILITY_ENFORCE", "")
	f := newStart()
	f.e.Blockers = func(context.Context, int64, db.Job) []hardconstraint.Blocker {
		return []hardconstraint.Blocker{{Category: hardconstraint.CategoryLocationWorkMode, Reason: "on-site in Austin"}}
	}
	if err := f.start(t); err != nil {
		t.Fatalf("shadow mode refused: %v", err)
	}
	if f.q.inserts != 1 || len(f.published) != 1 {
		t.Errorf("inserts = %d, published = %v, want one enqueue and one publish", f.q.inserts, f.published)
	}
}

// A fit category (experience, education, ...) is never a gate, enforced or not. A met
// work-authorization blocker is not a gate either: only an unmet one conflicts.
func TestStart_IgnoresBlockersThatAreNotAnEligibilityConflict(t *testing.T) {
	t.Setenv("AUTO_APPLY_ELIGIBILITY_ENFORCE", "1")
	for _, tc := range []struct {
		name string
		b    hardconstraint.Blocker
	}{
		{"fit category", hardconstraint.Blocker{Category: hardconstraint.CategoryExperience, Reason: "3 years of Go"}},
		{"met requirement", hardconstraint.Blocker{Category: hardconstraint.CategoryWorkAuth, Met: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newStart()
			f.e.Blockers = func(context.Context, int64, db.Job) []hardconstraint.Blocker { return []hardconstraint.Blocker{tc.b} }
			if err := f.start(t); err != nil {
				t.Errorf("refused on a %s: %v", tc.name, err)
			}
			if f.q.inserts != 1 {
				t.Errorf("inserts = %d, want the attempt enqueued", f.q.inserts)
			}
		})
	}
}

// A résumé alone is enough: the tailoring step seeds the base CV from it.
func TestStart_QueuesWithOnlyAResume(t *testing.T) {
	f := newStart()
	f.e.BaseCV, f.e.Seeder = hasCV{}.baseCV, hasCV{resume: true}
	if err := f.start(t); err != nil {
		t.Fatalf("résumé-only start = %v, want success", err)
	}
	if f.q.inserts != 1 {
		t.Errorf("inserts = %d, want 1", f.q.inserts)
	}
}

// The happy path, end to end: one charge keyed by the posting, the job put on the board at
// `preparing`, one insert, and exactly one publish of the new entry.
func TestStart_ChargesTracksEnqueuesAndPublishes(t *testing.T) {
	f := newStart()
	if err := f.start(t); err != nil {
		t.Fatalf("start = %v", err)
	}
	if f.m.ref != "job:42" {
		t.Errorf("charge ref = %q, want the posting's id", f.m.ref)
	}
	if f.b.tracked == nil || *f.b.tracked != "preparing" {
		t.Errorf("tracked stage = %v, want preparing", f.b.tracked)
	}
	// The published id is the one the INSERT returned, not the job's.
	if f.q.inserts != 1 || !slices.Equal(f.published, []int64{77}) {
		t.Errorf("inserts = %d, published = %v, want 1 and [77] (the entry id)", f.q.inserts, f.published)
	}
}

// The INSERT loses the race (the pair is already queued): the existing row decides, and
// nothing is published — a replay publishing too would start a second run.
func TestStart_WhenTheInsertLoses_TheExistingEntryDecides(t *testing.T) {
	at := pgtype.Timestamptz{Valid: true}
	declined := pgtype.Text{String: "declined", Valid: true}
	for _, tc := range []struct {
		name     string
		existing db.GetAutoApplyQueueEntryForJobRow
		want     RefusalReason
	}{
		{"declined", db.GetAutoApplyQueueEntryForJobRow{ReviewDecision: declined, BlockedAt: at}, RefusalAlreadyDeclined},
		{"submit uncertain", db.GetAutoApplyQueueEntryForJobRow{FailedAt: at, SubmitUncertainAt: at}, RefusalSubmitUncertain},
		{"dead-lettered", db.GetAutoApplyQueueEntryForJobRow{FailedAt: at}, RefusalStuck},
		{"parked", db.GetAutoApplyQueueEntryForJobRow{BlockedAt: at}, RefusalStuck},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newStart()
			f.q.existing = &tc.existing
			if got, _ := refusalReason(t, f.start(t)); got != tc.want {
				t.Errorf("reason = %q, want %q", got, tc.want)
			}
			if len(f.published) != 0 {
				t.Errorf("published = %v, want nothing for a lost insert", f.published)
			}
		})
	}

	// A live, undecided entry (page reload, second tab) is an idempotent success, and it is
	// not published either: the first request's run is still in flight.
	f := newStart()
	f.q.existing = &db.GetAutoApplyQueueEntryForJobRow{}
	if err := f.start(t); err != nil {
		t.Fatalf("a live entry = %v, want idempotent success", err)
	}
	if len(f.published) != 0 {
		t.Errorf("published = %v, want nothing for a replay", f.published)
	}
}

// A spent allowance stops the start before anything is created.
func TestStart_SpentAllowanceCreatesNothing(t *testing.T) {
	f := newStart()
	f.m.err = plan.ErrRefused
	var spent *AllowanceRefusal
	if err := f.start(t); !errors.As(err, &spent) {
		t.Fatalf("err = %v, want *AllowanceRefusal", err)
	}
	if f.q.inserts != 0 || len(f.published) != 0 {
		t.Errorf("a spent allowance created an attempt (inserts %d, published %v)", f.q.inserts, f.published)
	}
}

// A fault anywhere is passed through as a fault, with its cause, and nothing is published: a
// broken store or meter is never dressed up as a refusal the candidate could act on. (An
// unknown slug is the job lookup's own answer, pgx.ErrNoRows, and passes the same way.)
func TestStart_FaultsPassThroughAndCreateNothing(t *testing.T) {
	cause := errors.New("statement timeout")
	for _, tc := range []struct {
		name   string
		break_ func(f *startFixture)
	}{
		{"job lookup", func(f *startFixture) {
			f.e.FindJob = func(context.Context, string) (db.Job, error) { return db.Job{}, cause }
		}},
		{"base CV read", func(f *startFixture) { f.e.BaseCV = func(context.Context, int64) (bool, error) { return false, cause } }},
		{"résumé read", func(f *startFixture) { f.e.BaseCV, f.e.Seeder = hasCV{}.baseCV, resumeFault{err: cause} }},
		{"applied check", func(f *startFixture) { f.q.appliedErr = cause }},
		{"charge", func(f *startFixture) { f.m.err = cause }},
		{"insert", func(f *startFixture) { f.q.insertErr = cause }},
		{"existing entry read", func(f *startFixture) { f.q.existing, f.q.existingErr = &db.GetAutoApplyQueueEntryForJobRow{}, cause }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newStart()
			tc.break_(f)
			err := f.start(t)
			var r *Refusal
			var spent *AllowanceRefusal
			if errors.As(err, &r) || errors.As(err, &spent) {
				t.Fatalf("a fault was rendered as a refusal: %v", err)
			}
			if !errors.Is(err, cause) {
				t.Fatalf("err = %v, want the cause kept", err)
			}
			if len(f.published) != 0 {
				t.Errorf("published = %v, want nothing for a fault", f.published)
			}
		})
	}

	f := newStart()
	f.e.FindJob = func(context.Context, string) (db.Job, error) { return db.Job{}, pgx.ErrNoRows }
	if err := f.start(t); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("an unknown slug = %v, want pgx.ErrNoRows", err)
	}
}

// Tracking is best-effort, optional, and never moves a job already on the board.
func TestStart_TrackingIsBestEffortAndNeverMovesAStagedJob(t *testing.T) {
	f := newStart()
	f.q.stage = pgtype.Text{String: "interview", Valid: true}
	if err := f.start(t); err != nil {
		t.Fatalf("start = %v", err)
	}
	if f.b.tracked != nil {
		t.Errorf("tracked stage = %q, want none for a job already on the board", *f.b.tracked)
	}

	f = newStart()
	f.b.err = errors.New("board unavailable")
	if err := f.start(t); err != nil {
		t.Fatalf("a tracker failure refused the start: %v", err)
	}

	f = newStart()
	f.e.Tracker = nil
	if err := f.start(t); err != nil {
		t.Fatalf("start without a tracker = %v", err)
	}
}

// No publisher wired: the enqueue still commits and nothing is published.
func TestStart_WithoutAPublisherEnqueuesAnyway(t *testing.T) {
	f := newStart()
	f.e.PublishSubmit = nil
	if err := f.start(t); err != nil {
		t.Fatalf("start = %v", err)
	}
	if f.q.inserts != 1 {
		t.Errorf("inserts = %d, want 1", f.q.inserts)
	}
}
