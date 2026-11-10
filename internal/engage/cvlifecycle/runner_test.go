package cvlifecycle

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/maildaily"
	"github.com/strelov1/freehire/internal/platform/db"
)

type sendKey struct {
	user int64
	step int16
}

// fakeStore keeps the step ledger and the daily claim in memory and records the order in
// which the runner touched them. Who is due is SQL's business and is tested against
// Postgres; here `users` is simply whom the list returns for step 1 until their step row
// exists.
type fakeStore struct {
	maildaily.Memory

	events []string
	users  []int64
	sends  map[sendKey]string
	// paywall is whom the paywall list returns, by account, until their step-4 row exists.
	paywall map[int64]db.ListPaywallLetterDueRow

	// ineligible makes the claim refuse an account, as the statement does once it
	// unsubscribes or pays between the list and the claim.
	ineligible map[int64]bool
	// stealDay takes the account's day for another sender right after its step row is
	// written, the race the daily claim exists for.
	stealDay int64

	bookkeepErr error
	claimErr    error
	recordErr   error
	counts      map[string]int64
}

func newFakeStore(users ...int64) *fakeStore {
	return &fakeStore{users: users, sends: map[sendKey]string{}, ineligible: map[int64]bool{}, counts: map[string]int64{}}
}

func (s *fakeStore) log(format string, args ...any) {
	s.events = append(s.events, fmt.Sprintf(format, args...))
}

func (s *fakeStore) ClaimMailDay(ctx context.Context, arg db.ClaimMailDayParams) (pgtype.Date, error) {
	day, err := s.Memory.ClaimMailDay(ctx, arg)
	if err == nil {
		s.log("claim-day:%d", arg.UserID)
	}
	return day, err
}

func (s *fakeStore) ObserveCVLifecyclePaid(context.Context, int32) (int64, error) {
	return s.counts["paid"], s.bookkeepErr
}
func (s *fakeStore) ObserveCVLifecycleEntitled(context.Context, int32) (int64, error) {
	return s.counts["entitled"], nil
}
func (s *fakeStore) EnrollCVLifecycleUsers(context.Context, db.EnrollCVLifecycleUsersParams) (int64, error) {
	s.log("enroll")
	return s.counts["enrolled"], nil
}
func (s *fakeStore) CountCVLifecycleEnrollable(context.Context, int32) (int64, error) {
	return s.counts["enrollable"], nil
}
func (s *fakeStore) ExitCVLifecycleUnsubscribed(context.Context) (int64, error) {
	return s.counts["unsubscribed"], nil
}
func (s *fakeStore) PruneMailDayClaims(context.Context) (int64, error) {
	s.log("prune")
	return s.counts["pruned"], nil
}

func (s *fakeStore) ListCVLifecycleDue(_ context.Context, arg db.ListCVLifecycleDueParams) ([]db.ListCVLifecycleDueRow, error) {
	var rows []db.ListCVLifecycleDueRow
	if arg.Step != int16(StepATS) {
		return rows, nil
	}
	ids := append([]int64(nil), s.users...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if id <= arg.AfterUserID || int32(len(rows)) == arg.MaxRows {
			continue
		}
		if _, sent := s.sends[sendKey{id, arg.Step}]; sent || s.Holder(id) != "" {
			continue
		}
		rows = append(rows, db.ListCVLifecycleDueRow{UserID: id, Email: fmt.Sprintf("user%d@example.test", id)})
	}
	return rows, nil
}

func (s *fakeStore) ClaimCVLifecycleSend(_ context.Context, arg db.ClaimCVLifecycleSendParams) (int64, error) {
	key := sendKey{arg.UserID, arg.Step}
	if s.claimErr != nil {
		return 0, s.claimErr
	}
	if _, exists := s.sends[key]; exists || s.ineligible[arg.UserID] {
		return 0, pgx.ErrNoRows
	}
	s.sends[key] = ""
	s.log("claim-step:%d:%d", arg.Step, arg.UserID)
	if s.stealDay == arg.UserID {
		s.Hold(arg.UserID, maildaily.SourceOnboarding)
	}
	return arg.UserID, nil
}

func (s *fakeStore) ListPaywallLetterDue(_ context.Context, arg db.ListPaywallLetterDueParams) ([]db.ListPaywallLetterDueRow, error) {
	var ids []int64
	for id := range s.paywall {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var rows []db.ListPaywallLetterDueRow
	for _, id := range ids {
		if id <= arg.AfterUserID || int32(len(rows)) == arg.MaxRows {
			continue
		}
		if _, sent := s.sends[sendKey{id, int16(StepPaywall)}]; sent || s.Holder(id) != "" {
			continue
		}
		rows = append(rows, s.paywall[id])
	}
	return rows, nil
}

func (s *fakeStore) ClaimPaywallLetterSend(_ context.Context, arg db.ClaimPaywallLetterSendParams) (int64, error) {
	key := sendKey{arg.UserID, int16(StepPaywall)}
	if _, exists := s.sends[key]; exists || s.ineligible[arg.UserID] {
		return 0, pgx.ErrNoRows
	}
	s.sends[key] = ""
	s.log("claim-step:%d:%d", StepPaywall, arg.UserID)
	return arg.UserID, nil
}

func (s *fakeStore) RecordCVLifecycleSendError(_ context.Context, arg db.RecordCVLifecycleSendErrorParams) error {
	if s.recordErr != nil {
		return s.recordErr
	}
	s.sends[sendKey{arg.UserID, arg.Step}] = arg.Error
	return nil
}

func (s *fakeStore) ReleaseCVLifecycleSend(_ context.Context, arg db.ReleaseCVLifecycleSendParams) error {
	key := sendKey{arg.UserID, arg.Step}
	if msg, ok := s.sends[key]; ok && msg == "" {
		delete(s.sends, key)
	}
	return nil
}

type fakeComposer struct {
	err map[int64]error
}

func (c *fakeComposer) Compose(_ context.Context, step Step, userID int64) (Letter, error) {
	if err := c.err[userID]; err != nil {
		return Letter{}, err
	}
	return Letter{Step: step, Overall: 50, Potential: 80}, nil
}

type fakeSender struct {
	store   *fakeStore
	err     map[int64]error
	sent    []string
	letters []Letter
}

func (s *fakeSender) Send(_ context.Context, l Letter, userID int64, to string) error {
	s.store.log("send:%d:%d", l.Step, userID)
	if err := s.err[userID]; err != nil {
		return err
	}
	s.sent = append(s.sent, fmt.Sprintf("%d:%d:%s", l.Step, userID, to))
	s.letters = append(s.letters, l)
	return nil
}

type rig struct {
	store    *fakeStore
	composer *fakeComposer
	sender   *fakeSender
	runner   *Runner
}

func newRig(t *testing.T, maxPerRun int32, users ...int64) *rig {
	t.Helper()
	store := newFakeStore(users...)
	r := &rig{store: store, composer: &fakeComposer{err: map[int64]error{}}, sender: &fakeSender{store: store, err: map[int64]error{}}}
	cfg := DefaultConfig()
	cfg.MaxPerRun = maxPerRun
	runner, err := New(store, r.composer, r.sender, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	r.runner = runner
	return r
}

func (r *rig) run(t *testing.T) Stats {
	t.Helper()
	stats, err := r.runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return stats
}

func (r *rig) events() string { return strings.Join(r.store.events, " ") }

func TestRun_ClaimsStepThenDayThenSends(t *testing.T) {
	r := newRig(t, 10, 7)
	stats := r.run(t)

	want := "enroll prune claim-step:1:7 claim-day:7 send:1:7"
	if got := r.events(); got != want {
		t.Fatalf("events = %q, want %q", got, want)
	}
	if stats.Sent[StepATS] != 1 {
		t.Errorf("Sent = %v, want one step-1 letter", stats.Sent)
	}
	if got := r.sender.sent[0]; got != "1:7:user7@example.test" {
		t.Errorf("sent %q", got)
	}
}

func TestRun_RepeatedRunDoesNotSendAgain(t *testing.T) {
	r := newRig(t, 10, 7)
	r.run(t)
	r.store.Memory = maildaily.Memory{} // a new day: the claim is free, the step row is not
	r.run(t)

	if len(r.sender.sent) != 1 {
		t.Fatalf("sent %v, want exactly one letter across two runs", r.sender.sent)
	}
}

func TestRun_FailedSendBurnsStepAndReleasesDay(t *testing.T) {
	r := newRig(t, 10, 1, 2)
	r.sender.err[1] = errors.New("ses: rejected")
	stats := r.run(t)

	if stats.Failed[StepATS] != 1 || stats.Sent[StepATS] != 1 {
		t.Fatalf("stats = sent %v failed %v, want one of each", stats.Sent, stats.Failed)
	}
	if got := r.store.sends[sendKey{1, 1}]; got != "ses: rejected" {
		t.Errorf("step row error = %q, want the transport error", got)
	}
	if r.store.Holder(1) != "" {
		t.Error("the day claim was kept after a definite failure")
	}
	if r.store.Holder(2) != maildaily.SourceCVLifecycle {
		t.Error("the day claim of the delivered letter was not kept")
	}

	// The failed account is not retried, and the delivered one is not mailed twice.
	r.store.Memory = maildaily.Memory{}
	r.sender.err = map[int64]error{}
	r.run(t)
	if len(r.sender.sent) != 1 {
		t.Fatalf("sent %v after the second run, want the single earlier letter", r.sender.sent)
	}
}

func TestRun_LostDayLeavesStepPending(t *testing.T) {
	r := newRig(t, 10, 7)
	r.store.stealDay = 7
	stats := r.run(t)

	if len(r.sender.sent) != 0 {
		t.Fatalf("sent %v although another sender held the day", r.sender.sent)
	}
	if _, burned := r.store.sends[sendKey{7, 1}]; burned {
		t.Error("the step row was kept for a letter that was never sent")
	}
	if stats.Deferred[StepATS] != 1 {
		t.Errorf("Deferred = %v, want 1", stats.Deferred)
	}

	r.store.stealDay = 0
	r.store.Memory = maildaily.Memory{}
	r.run(t)
	if len(r.sender.sent) != 1 {
		t.Fatalf("sent %v on the next day, want the pending step delivered", r.sender.sent)
	}
}

func TestRun_AccountHoldingTodayIsNotListed(t *testing.T) {
	r := newRig(t, 10, 7)
	r.store.Hold(7, maildaily.SourceBroadcast)
	r.run(t)
	if len(r.sender.sent) != 0 || r.store.Holder(7) != maildaily.SourceBroadcast {
		t.Fatalf("sent %v, holder %q: another sender's day was not respected", r.sender.sent, r.store.Holder(7))
	}
}

func TestRun_UnbuildableLetterLeavesStepPending(t *testing.T) {
	r := newRig(t, 10, 1, 2)
	r.composer.err[1] = fmt.Errorf("%w: no stored CV", ErrSkip)
	r.composer.err[2] = errors.New("meili is down")
	stats := r.run(t)

	if len(r.sender.sent) != 0 {
		t.Fatalf("sent %v for letters that could not be built", r.sender.sent)
	}
	if len(r.store.sends) != 0 || r.store.Holder(1) != "" || r.store.Holder(2) != "" {
		t.Error("a claim was taken for a letter that could not be built")
	}
	if stats.Skipped[StepATS] != 2 {
		t.Errorf("Skipped = %v, want 2", stats.Skipped)
	}
	if stats.Errors != 1 {
		t.Errorf("Errors = %d, want only the unreadable input counted", stats.Errors)
	}
}

func TestRun_AccountThatStoppedBeingEligibleIsSkipped(t *testing.T) {
	r := newRig(t, 10, 7)
	r.store.ineligible[7] = true
	stats := r.run(t)

	if len(r.sender.sent) != 0 || stats.Skipped[StepATS] != 1 || stats.Errors != 0 {
		t.Fatalf("sent %v skipped %v errors %d", r.sender.sent, stats.Skipped, stats.Errors)
	}
	if r.store.Holder(7) != "" {
		t.Error("a day claim was taken for an ineligible account")
	}
}

func TestRun_StepClaimErrorIsCountedAndSendsNothing(t *testing.T) {
	r := newRig(t, 10, 7)
	r.store.claimErr = errors.New("connection reset")
	stats := r.run(t)

	if len(r.sender.sent) != 0 || stats.Errors != 1 {
		t.Fatalf("sent %v errors %d", r.sender.sent, stats.Errors)
	}
}

func TestRun_DayClaimErrorGivesTheStepBack(t *testing.T) {
	r := newRig(t, 10, 7)
	r.store.ClaimErr = errors.New("connection reset")
	stats := r.run(t)

	if len(r.sender.sent) != 0 || stats.Errors != 1 {
		t.Fatalf("sent %v errors %d", r.sender.sent, stats.Errors)
	}
	if _, kept := r.store.sends[sendKey{7, 1}]; kept {
		t.Error("the step row was kept although the day could not be claimed")
	}
}

func TestRun_PausedSESGivesBothClaimsBackAndStops(t *testing.T) {
	r := newRig(t, 10, 1, 2)
	r.sender.err[1] = fmt.Errorf("%w: account paused", emailnotify.ErrSendingPaused)
	stats := r.run(t)

	if !stats.Paused {
		t.Error("Paused not set")
	}
	if strings.Contains(r.events(), "send:1:2") {
		t.Errorf("the pass went on after SES paused: %s", r.events())
	}
	if len(r.store.sends) != 0 || r.store.Holder(1) != "" {
		t.Errorf("a claim outlived the pause: sends %v holder %q", r.store.sends, r.store.Holder(1))
	}
}

func TestRun_MaxPerRunCountsLettersAndPagesPastSkips(t *testing.T) {
	r := newRig(t, 2, 1, 2, 3, 4, 5, 6)
	r.composer.err[1] = ErrSkip
	r.composer.err[2] = ErrSkip
	stats := r.run(t)

	if stats.Sent[StepATS] != 2 {
		t.Fatalf("Sent = %v, want 2: skipped accounts must not spend the cap, and must not hide the ones behind them", stats.Sent)
	}
	if got := strings.Join(r.sender.sent, ","); got != "1:3:user3@example.test,1:4:user4@example.test" {
		t.Errorf("sent %s", got)
	}
}

func TestRun_StopsAtMaxPerRun(t *testing.T) {
	r := newRig(t, 1, 1, 2)
	stats := r.run(t)
	if total := attempted(&stats); total != 1 {
		t.Fatalf("sent %d letters with MaxPerRun 1", total)
	}
}

func TestRun_BookkeepingFailureAbortsBeforeAnySend(t *testing.T) {
	r := newRig(t, 10, 7)
	r.store.bookkeepErr = errors.New("db down")
	if _, err := r.runner.Run(context.Background()); err == nil {
		t.Fatal("Run succeeded although the paying observation failed")
	}
	if len(r.sender.sent) != 0 {
		t.Fatalf("sent %v on top of an unknown enrollment state", r.sender.sent)
	}
}

func TestRun_ReportsBookkeepingCounts(t *testing.T) {
	r := newRig(t, 10)
	r.store.counts = map[string]int64{"paid": 1, "entitled": 2, "enrolled": 3, "unsubscribed": 4, "pruned": 5}
	stats := r.run(t)
	if stats.Paid != 1 || stats.Entitled != 2 || stats.Enrolled != 3 || stats.Unsubscribed != 4 || stats.Pruned != 5 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestDryRun_CountsAndWritesNothing(t *testing.T) {
	r := newRig(t, 2, 1, 2, 3)
	r.store.counts["enrollable"] = 9
	stats, err := r.runner.DryRun(context.Background())
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if stats.Enrollable != 9 || stats.Due[StepATS] != 3 {
		t.Fatalf("stats = enrollable %d due %v", stats.Enrollable, stats.Due)
	}
	if len(r.store.events) != 0 || len(r.store.sends) != 0 || len(r.sender.sent) != 0 {
		t.Fatalf("a dry run wrote: events %v sends %v", r.store.events, r.store.sends)
	}
}

func TestNew_RefusesAScheduleThatCouldSendOutOfOrder(t *testing.T) {
	for _, after := range [][3]int32{{6, 6, 13}, {1, 13, 6}, {-1, 6, 13}} {
		cfg := DefaultConfig()
		cfg.AfterDays = after
		if _, err := New(newFakeStore(), &fakeComposer{}, &fakeSender{}, cfg); err == nil {
			t.Errorf("New accepted AfterDays %v", after)
		}
	}
	cfg := DefaultConfig()
	cfg.MaxPerRun = 0
	if _, err := New(newFakeStore(), &fakeComposer{}, &fakeSender{}, cfg); err == nil {
		t.Error("New accepted MaxPerRun 0")
	}
}

// paywallRow is what the paywall list returns for a hit on a posting.
func paywallRow(id int64, feature string) db.ListPaywallLetterDueRow {
	return db.ListPaywallLetterDueRow{
		UserID: id, Email: fmt.Sprintf("user%d@example.test", id), Feature: feature,
		JobTitle: "Go Engineer", JobCompany: "Acme", JobSlug: "go-engineer-acme-1",
	}
}

func TestRun_PaywallLetterWinsTheDayOverACVLetter(t *testing.T) {
	r := newRig(t, 10, 7)
	r.store.paywall = map[int64]db.ListPaywallLetterDueRow{7: paywallRow(7, "tailor")}
	stats := r.run(t)

	want := "enroll prune claim-step:4:7 claim-day:7 send:4:7"
	if got := r.events(); got != want {
		t.Fatalf("events = %q, want %q: the paywall letter must be sent first and take the day", got, want)
	}
	if stats.Sent[StepPaywall] != 1 || stats.Sent[StepATS] != 0 {
		t.Errorf("Sent = %v, want the paywall letter and not the CV letter", stats.Sent)
	}

	// The CV letter is not lost: it goes out when the day is free again.
	r.store.Memory = maildaily.Memory{}
	r.run(t)
	if got := strings.Join(r.sender.sent, ","); got != "4:7:user7@example.test,1:7:user7@example.test" {
		t.Errorf("sent %s, want the paywall letter then, a day later, the CV letter", got)
	}
}

func TestRun_PaywallLetterCarriesTheAttemptAndGoesOutOnce(t *testing.T) {
	r := newRig(t, 10)
	r.store.paywall = map[int64]db.ListPaywallLetterDueRow{7: paywallRow(7, "cover-letter")}
	r.run(t)
	r.store.Memory = maildaily.Memory{}
	r.run(t)

	if len(r.sender.letters) != 1 {
		t.Fatalf("sent %v, want exactly one paywall letter across two runs", r.sender.sent)
	}
	l := r.sender.letters[0]
	if l.Step != StepPaywall || l.Paywall == nil || *l.Paywall != (Paywall{
		Feature: "cover-letter", JobTitle: "Go Engineer", Company: "Acme", JobSlug: "go-engineer-acme-1",
	}) {
		t.Errorf("letter = %+v, want the hit's feature and posting", l)
	}
}

func TestRun_PaywallLetterBehindAnotherSendersDayStaysPending(t *testing.T) {
	r := newRig(t, 10)
	r.store.paywall = map[int64]db.ListPaywallLetterDueRow{7: paywallRow(7, "tailor")}
	r.store.Hold(7, maildaily.SourceOnboarding)
	r.run(t)
	if len(r.sender.sent) != 0 || r.store.Holder(7) != maildaily.SourceOnboarding {
		t.Fatalf("sent %v, holder %q: another sender's day was not respected", r.sender.sent, r.store.Holder(7))
	}

	r.store.Memory = maildaily.Memory{}
	r.run(t)
	if len(r.sender.sent) != 1 {
		t.Fatalf("sent %v on the next day, want the pending paywall letter", r.sender.sent)
	}
}

func TestRun_PaywallLetterForAnAccountThatStoppedBeingEligibleIsSkipped(t *testing.T) {
	r := newRig(t, 10)
	r.store.paywall = map[int64]db.ListPaywallLetterDueRow{7: paywallRow(7, "tailor")}
	r.store.ineligible[7] = true
	stats := r.run(t)

	if len(r.sender.sent) != 0 || stats.Skipped[StepPaywall] != 1 || r.store.Holder(7) != "" {
		t.Fatalf("sent %v skipped %v holder %q", r.sender.sent, stats.Skipped, r.store.Holder(7))
	}
}

func TestRun_PaywallLettersSpendTheSameCapAsCVLetters(t *testing.T) {
	r := newRig(t, 1, 5)
	r.store.paywall = map[int64]db.ListPaywallLetterDueRow{7: paywallRow(7, "tailor")}
	stats := r.run(t)
	if total := attempted(&stats); total != 1 || stats.Sent[StepPaywall] != 1 {
		t.Fatalf("sent %v with MaxPerRun 1, want only the paywall letter", stats.Sent)
	}
}

func TestDryRun_CountsDuePaywallLetters(t *testing.T) {
	r := newRig(t, 10)
	r.store.paywall = map[int64]db.ListPaywallLetterDueRow{7: paywallRow(7, "tailor"), 8: paywallRow(8, "match")}
	stats, err := r.runner.DryRun(context.Background())
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if stats.Due[StepPaywall] != 2 || len(r.store.sends) != 0 || len(r.sender.sent) != 0 {
		t.Fatalf("due %v sends %v sent %v", stats.Due, r.store.sends, r.sender.sent)
	}
}

func TestNew_RefusesAPaywallWindowThatNeverOpens(t *testing.T) {
	for _, c := range []struct{ after, within int32 }{{-1, 24}, {60, 0}, {1440, 24}, {2000, 24}} {
		cfg := DefaultConfig()
		cfg.PaywallAfterMinutes, cfg.PaywallWithinHours = c.after, c.within
		if _, err := New(newFakeStore(), &fakeComposer{}, &fakeSender{}, cfg); err == nil {
			t.Errorf("New accepted a paywall window of %d min / %d h", c.after, c.within)
		}
	}
}

func TestStep_Campaign(t *testing.T) {
	for step, want := range map[Step]string{StepATS: "cv-letter-1", StepPotential: "cv-letter-2", StepOffer: "cv-letter-3", StepPaywall: "paywall-letter"} {
		if got := step.Campaign(); got != want {
			t.Errorf("Step(%d).Campaign() = %q, want %q", step, got, want)
		}
	}
}
