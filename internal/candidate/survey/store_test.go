package survey

import (
	"context"
	"errors"
	"testing"
)

type fakeRepo struct {
	stored   Responses
	getErr   error
	upserted Responses
	upsertN  int
}

func (f *fakeRepo) Get(context.Context, int64) (Responses, error) {
	if f.getErr != nil {
		return Responses{}, f.getErr
	}
	return f.stored, nil
}

func (f *fakeRepo) Upsert(_ context.Context, _ int64, a Responses) (Responses, error) {
	f.upsertN++
	f.upserted = a
	return a, nil
}

func TestGetOnAnUnansweredAccountReportsUnstatedRatherThanFailing(t *testing.T) {
	// Answering nothing is a normal state. A caller should not have to distinguish
	// "no row" from "no answers" — there is no difference worth making them handle.
	store := New(&fakeRepo{getErr: ErrNotFound})

	got, err := store.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("Get() = %v, want nil", err)
	}
	if got != (Responses{}) {
		t.Errorf("Get() = %+v, want a fully unstated record", got)
	}
}

func TestGetPropagatesAGenuineFailure(t *testing.T) {
	boom := errors.New("connection refused")
	store := New(&fakeRepo{getErr: boom})

	if _, err := store.Get(context.Background(), 1); !errors.Is(err, boom) {
		t.Fatalf("Get() = %v, want the underlying failure", err)
	}
}

func TestUpdateMergesOverWhatIsStored(t *testing.T) {
	repo := &fakeRepo{stored: Responses{JobSearchStage: s("searching")}}
	store := New(repo)

	got, err := store.Update(context.Background(), 1, Responses{BiggestChallenge: s("english")})
	if err != nil {
		t.Fatalf("Update() = %v, want nil", err)
	}
	if got.JobSearchStage == nil || *got.JobSearchStage != "searching" {
		t.Errorf("JobSearchStage = %v, want the stored value preserved", got.JobSearchStage)
	}
	if got.BiggestChallenge == nil || *got.BiggestChallenge != "english" {
		t.Errorf("BiggestChallenge = %v, want the update applied", got.BiggestChallenge)
	}
}

func TestUpdateOnAnUnansweredAccountCreates(t *testing.T) {
	repo := &fakeRepo{getErr: ErrNotFound}
	store := New(repo)

	if _, err := store.Update(context.Background(), 1, Responses{JobSearchStage: s("not_started")}); err != nil {
		t.Fatalf("Update() = %v, want nil", err)
	}
	if repo.upsertN != 1 {
		t.Fatalf("Upsert called %d times, want 1", repo.upsertN)
	}
}

func TestUpdateRejectsInvalidInputAsAValidationError(t *testing.T) {
	// The endpoint has to tell a bad request from a broken database. Everything Update can
	// return would otherwise look the same to it.
	repo := &fakeRepo{}
	store := New(repo)

	_, err := store.Update(context.Background(), 1, Responses{JobSearchStage: s("nope")})
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("Update() = %v, want a *ValidationError", err)
	}
	if repo.upsertN != 0 {
		t.Errorf("Upsert called %d times on invalid input, want 0", repo.upsertN)
	}
}

func TestUpdateValidatesTheMergedRecordNotJustThePatch(t *testing.T) {
	// "A field the body omits keeps its stored value" has to hold for the note's gate too.
	// A caller who already stored `other` and now sends only the note is completing an
	// answer, not contradicting one — validating the patch alone would reject it because
	// the patch carries no challenge.
	repo := &fakeRepo{stored: Responses{BiggestChallenge: s("other")}}
	store := New(repo)

	got, err := store.Update(context.Background(), 1, Responses{BiggestChallengeNote: s("visa paperwork")})
	if err != nil {
		t.Fatalf("Update() = %v, want nil", err)
	}
	if got.BiggestChallengeNote == nil || *got.BiggestChallengeNote != "visa paperwork" {
		t.Errorf("note = %v, want it stored", got.BiggestChallengeNote)
	}
}

func TestUpdateRejectsAPatchThatMakesTheMergedRecordInvalid(t *testing.T) {
	// The mirror image: moving off `other` while a note is stored must not persist a record
	// Validate would reject. Merge drops the stale note, so this is really a guard that the
	// two agree.
	repo := &fakeRepo{stored: Responses{BiggestChallenge: s("other"), BiggestChallengeNote: s("visa paperwork")}}
	store := New(repo)

	got, err := store.Update(context.Background(), 1, Responses{BiggestChallenge: s("english")})
	if err != nil {
		t.Fatalf("Update() = %v, want nil", err)
	}
	if got.BiggestChallengeNote != nil {
		t.Errorf("note = %q, want it dropped with the challenge it belonged to", *got.BiggestChallengeNote)
	}
}

func TestUpdateSanitizesBeforeValidating(t *testing.T) {
	// " usd " is a valid currency the moment it is normalized, and rejecting it would be
	// rejecting a value the wizard could plausibly send.
	repo := &fakeRepo{}
	store := New(repo)

	got, err := store.Update(context.Background(), 1, Responses{CurrentIncomeCurrency: s(" usd ")})
	if err != nil {
		t.Fatalf("Update() = %v, want nil", err)
	}
	if got.CurrentIncomeCurrency == nil || *got.CurrentIncomeCurrency != "USD" {
		t.Errorf("CurrentIncomeCurrency = %v, want USD", got.CurrentIncomeCurrency)
	}
}
