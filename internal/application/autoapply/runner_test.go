package autoapply

import (
	"context"
	"errors"
	"testing"
)

type fakeStore struct {
	waves [][]Claimed
	calls int

	submitted []int64
	submitErr error

	parked         []int64
	parkedUnmapped map[int64][]UnmappedField
	parkErr        error

	failed       []int64
	failAttempts map[int64]int
	failMax      int
	failErr      error
}

func (f *fakeStore) Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error) {
	f.calls++
	if f.calls > len(f.waves) {
		return nil, nil
	}
	return f.waves[f.calls-1], nil
}

func (f *fakeStore) Submit(ctx context.Context, c Claimed) error {
	if f.submitErr != nil {
		return f.submitErr
	}
	f.submitted = append(f.submitted, c.QueueID)
	return nil
}

func (f *fakeStore) Park(ctx context.Context, queueID int64, unmapped []UnmappedField, reason string) error {
	if f.parkErr != nil {
		return f.parkErr
	}
	f.parked = append(f.parked, queueID)
	if f.parkedUnmapped == nil {
		f.parkedUnmapped = map[int64][]UnmappedField{}
	}
	f.parkedUnmapped[queueID] = unmapped
	return nil
}

func (f *fakeStore) Fail(ctx context.Context, queueID int64, errMsg string, maxAttempts int) (bool, error) {
	if f.failErr != nil {
		return false, f.failErr
	}
	f.failed = append(f.failed, queueID)
	if f.failAttempts == nil {
		f.failAttempts = map[int64]int{}
	}
	f.failAttempts[queueID]++
	f.failMax = maxAttempts
	return f.failAttempts[queueID] >= maxAttempts, nil
}

type fakeAnswers struct {
	answers map[string]string
	err     error
}

func (f *fakeAnswers) Answers(ctx context.Context, userID int64) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.answers, nil
}

type fakeSidecar struct {
	result SidecarResult
	err    error
	calls  int
}

func (f *fakeSidecar) Submit(ctx context.Context, c Claimed, answers map[string]string) (SidecarResult, error) {
	f.calls++
	if f.err != nil {
		return SidecarResult{}, f.err
	}
	return f.result, nil
}

func opts() RunOptions {
	return RunOptions{BatchSize: 10, LeaseSeconds: 3600, MaxAttempts: 3, Concurrency: 1}
}

func TestRunSubmitsAFullyResolvedAttempt(t *testing.T) {
	store := &fakeStore{waves: [][]Claimed{{{QueueID: 1, UserID: 10, JobID: 100}}}}
	answers := &fakeAnswers{answers: map[string]string{"full_name": "Ada Lovelace"}}
	sidecar := &fakeSidecar{result: SidecarResult{Status: StatusApplied}}

	stats, err := Run(context.Background(), store, answers, sidecar, opts())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Applied != 1 {
		t.Errorf("Applied = %d, want 1", stats.Applied)
	}
	if len(store.submitted) != 1 || store.submitted[0] != 1 {
		t.Errorf("Store.Submit calls = %v, want [1]", store.submitted)
	}
}

func TestRunParksAnUnresolvedAttemptWithoutRetrying(t *testing.T) {
	store := &fakeStore{waves: [][]Claimed{{{QueueID: 2, UserID: 10, JobID: 100}}}}
	answers := &fakeAnswers{answers: map[string]string{}}
	unmapped := []UnmappedField{{ID: "question_1", Label: "Why us?", Required: true, Reason: "no known answer"}}
	sidecar := &fakeSidecar{result: SidecarResult{Status: StatusParked, Unmapped: unmapped, Reason: "1 required question unanswered"}}

	stats, err := Run(context.Background(), store, answers, sidecar, opts())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Blocked != 1 {
		t.Errorf("Blocked = %d, want 1", stats.Blocked)
	}
	if len(store.parked) != 1 || store.parked[0] != 2 {
		t.Errorf("Store.Park calls = %v, want [2]", store.parked)
	}
	if got := store.parkedUnmapped[2]; len(got) != 1 || got[0].ID != "question_1" {
		t.Errorf("parked unmapped = %+v, want the sidecar's list carried through", got)
	}
	if len(store.submitted) != 0 {
		t.Error("a parked attempt must never call Store.Submit")
	}
}

// A parked result whose form could not be scanned at all (openspec/changes/
// auto-apply-whitelabel-greenhouse) carries a whole-form Reason with no Unmapped list — the
// same shape the pre-existing Lever captcha short-circuit already produces. Run does not
// branch on Reason at all, so this is a regression guard for that "no special-casing" fact
// as much as a behavior test: any future change that starts inspecting Reason here must not
// start treating a Reason it does not otherwise care about differently for these values,
// naming the literal strings internal/atsapply produces
// (unrecognized_form_layout/form_captcha_protected) so a rename there fails visibly here.
func TestRunParksAnUnscannableFormWithoutRetrying(t *testing.T) {
	for _, reason := range []string{"unrecognized_form_layout", "form_captcha_protected"} {
		store := &fakeStore{waves: [][]Claimed{{{QueueID: 20, UserID: 10, JobID: 100}}}}
		answers := &fakeAnswers{answers: map[string]string{}}
		sidecar := &fakeSidecar{result: SidecarResult{Status: StatusParked, Reason: reason}}

		stats, err := Run(context.Background(), store, answers, sidecar, opts())
		if err != nil {
			t.Fatalf("reason %q: %v", reason, err)
		}
		if stats.Blocked != 1 || stats.Degraded() {
			t.Errorf("reason %q: stats = %+v, want Blocked=1 and not Degraded", reason, stats)
		}
		if len(store.parked) != 1 || store.parked[0] != 20 {
			t.Errorf("reason %q: Store.Park calls = %v, want [20]", reason, store.parked)
		}
		if len(store.failed) != 0 {
			t.Errorf("reason %q: Store.Fail calls = %v, want none — an unscannable form must never spend the retry/dead-letter budget", reason, store.failed)
		}
	}
}

func TestRunRetriesATransientSidecarFailure(t *testing.T) {
	store := &fakeStore{waves: [][]Claimed{{{QueueID: 3, UserID: 10, JobID: 100}}}}
	answers := &fakeAnswers{answers: map[string]string{}}
	sidecar := &fakeSidecar{err: errors.New("sidecar unreachable")}

	stats, err := Run(context.Background(), store, answers, sidecar, opts())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1", stats.Failed)
	}
	if len(store.failed) != 1 || store.failed[0] != 3 {
		t.Errorf("Store.Fail calls = %v, want [3]", store.failed)
	}
	if store.failMax != opts().MaxAttempts {
		t.Errorf("Fail called with maxAttempts=%d, want the run's configured %d", store.failMax, opts().MaxAttempts)
	}
}

// A sidecar call that fails because the candidate's own answers could not be assembled
// (a local read, before anything ever reaches the sidecar) is a transient failure like any
// other — it never touches the sidecar at all.
func TestRunFailsWhenAnswersCannotBeAssembled(t *testing.T) {
	store := &fakeStore{waves: [][]Claimed{{{QueueID: 4, UserID: 10, JobID: 100}}}}
	answers := &fakeAnswers{err: errors.New("db unavailable")}
	sidecar := &fakeSidecar{result: SidecarResult{Status: StatusApplied}}

	stats, err := Run(context.Background(), store, answers, sidecar, opts())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1", stats.Failed)
	}
	if sidecar.calls != 0 {
		t.Error("the sidecar must not be called when answers could not be assembled")
	}
}

// The one outcome that must never retry through the ordinary path: the sidecar reports a
// real submission happened, but recording it locally then fails. Retrying normally would
// eventually re-arm the queue row and risk a second real submission to the employer, which
// the spec forbids outright — so this forces an immediate dead-letter instead of spending
// the run's configured attempts budget.
func TestRunDeadLettersImmediatelyWhenRecordingASuccessfulSubmitFails(t *testing.T) {
	store := &fakeStore{
		waves:     [][]Claimed{{{QueueID: 5, UserID: 10, JobID: 100}}},
		submitErr: errors.New("db unavailable"),
	}
	answers := &fakeAnswers{answers: map[string]string{}}
	sidecar := &fakeSidecar{result: SidecarResult{Status: StatusApplied}}

	stats, err := Run(context.Background(), store, answers, sidecar, opts())
	if err != nil {
		t.Fatal(err)
	}
	if stats.DeadLettered != 1 {
		t.Errorf("DeadLettered = %d, want 1 — a lost post-submit record must never be silently retried", stats.DeadLettered)
	}
	if got := store.failAttempts[5]; got != 1 {
		t.Errorf("Fail called %d times for the lost record, want exactly 1 (forced dead-letter, not the normal retry budget)", got)
	}
}

// The symmetric case on the browser side: the sidecar pressed submit but could not tell
// whether the employer accepted it. Retrying this normally risks a second real submission
// the same way a lost post-submit record does, so it must take the same forced-dead-letter
// path rather than the ordinary attempts budget.
func TestRunDeadLettersImmediatelyOnAnUnconfirmedSubmission(t *testing.T) {
	store := &fakeStore{waves: [][]Claimed{{{QueueID: 6, UserID: 10, JobID: 100}}}}
	answers := &fakeAnswers{answers: map[string]string{}}
	sidecar := &fakeSidecar{result: SidecarResult{Status: StatusUnconfirmed}}

	stats, err := Run(context.Background(), store, answers, sidecar, opts())
	if err != nil {
		t.Fatal(err)
	}
	if stats.DeadLettered != 1 {
		t.Errorf("DeadLettered = %d, want 1 — an unconfirmed submission must never be silently retried", stats.DeadLettered)
	}
	if len(store.submitted) != 0 {
		t.Error("Store.Submit must not be called for an unconfirmed result — it is not a known success")
	}
	if got := store.failAttempts[6]; got != 1 {
		t.Errorf("Fail called %d times, want exactly 1 (forced dead-letter, not the normal retry budget)", got)
	}
}

// Found by a PR review pass: if the write that was supposed to make an unconfirmed
// attempt terminal itself fails, the row stays claimed (its lease still runs out on its
// own) — a later run can reclaim it as an ordinary pending attempt and resubmit for real.
// Run must not then report it as DeadLettered, which would claim a terminal state that
// never actually landed.
func TestRunReportsFailedNotDeadLetteredWhenTheDeadLetterWriteItselfFails(t *testing.T) {
	store := &fakeStore{
		waves:   [][]Claimed{{{QueueID: 7, UserID: 10, JobID: 100}}},
		failErr: errors.New("db unavailable"),
	}
	answers := &fakeAnswers{answers: map[string]string{}}
	sidecar := &fakeSidecar{result: SidecarResult{Status: StatusUnconfirmed}}

	stats, err := Run(context.Background(), store, answers, sidecar, opts())
	if err != nil {
		t.Fatal(err)
	}
	if stats.DeadLettered != 0 {
		t.Errorf("DeadLettered = %d, want 0 — the write that would have made this terminal failed", stats.DeadLettered)
	}
	if stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1 — an honest report that the row is still live, not terminal", stats.Failed)
	}
}

func TestRunDegradedOnDeadLetterOrTotalFailure(t *testing.T) {
	cases := []struct {
		name string
		s    RunStats
		want bool
	}{
		{"clean run", RunStats{Applied: 5}, false},
		{"some parked, none failed", RunStats{Applied: 3, Blocked: 2}, false},
		{"a dead letter", RunStats{Applied: 5, DeadLettered: 1}, true},
		{"failed everything", RunStats{Failed: 3}, true},
		{"failed some, applied some", RunStats{Applied: 2, Failed: 1}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.s.Degraded(); got != c.want {
				t.Errorf("Degraded() = %v, want %v", got, c.want)
			}
		})
	}
}
