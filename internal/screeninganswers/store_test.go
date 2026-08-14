package screeninganswers_test

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/screeninganswers"
)

// fakeRepo records the params it is handed and returns canned records/errors, so the
// store tests run without a database.
type fakeRepo struct {
	getRet screeninganswers.Answers
	getErr error

	upserted     screeninganswers.Answers
	upsertCalled bool
	upsertRet    screeninganswers.Answers
	upsertErr    error
}

func (f *fakeRepo) Get(_ context.Context, _ int64) (screeninganswers.Answers, error) {
	return f.getRet, f.getErr
}

func (f *fakeRepo) Upsert(_ context.Context, _ int64, a screeninganswers.Answers) (screeninganswers.Answers, error) {
	f.upserted = a
	f.upsertCalled = true
	return f.upsertRet, f.upsertErr
}

func ptrBool(b bool) *bool { return &b }
func ptrInt(n int) *int    { return &n }

func TestUpdate_FirstUpdateTreatsNoExistingRecordAsFullyUnstated(t *testing.T) {
	repo := &fakeRepo{getErr: screeninganswers.ErrNotFound}
	store := screeninganswers.New(repo)

	_, err := store.Update(context.Background(), 1, screeninganswers.Answers{WillingToRelocate: ptrBool(true)})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !repo.upsertCalled {
		t.Fatal("repo.Upsert was not called")
	}
	if repo.upserted.WillingToRelocate == nil || *repo.upserted.WillingToRelocate != true {
		t.Errorf("WillingToRelocate = %v, want true", repo.upserted.WillingToRelocate)
	}
}

func TestUpdate_MergesOverTheExistingRecord(t *testing.T) {
	repo := &fakeRepo{getRet: screeninganswers.Answers{
		NoticePeriodDays:  ptrInt(30),
		WillingToRelocate: ptrBool(false),
	}}
	store := screeninganswers.New(repo)

	_, err := store.Update(context.Background(), 1, screeninganswers.Answers{WillingToRelocate: ptrBool(true)})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if repo.upserted.NoticePeriodDays == nil || *repo.upserted.NoticePeriodDays != 30 {
		t.Errorf("NoticePeriodDays = %v, want 30 (untouched)", repo.upserted.NoticePeriodDays)
	}
	if repo.upserted.WillingToRelocate == nil || *repo.upserted.WillingToRelocate != true {
		t.Errorf("WillingToRelocate = %v, want true (updated)", repo.upserted.WillingToRelocate)
	}
}

func TestUpdate_RejectsAnInvalidFieldWithAValidationError(t *testing.T) {
	repo := &fakeRepo{getErr: screeninganswers.ErrNotFound}
	store := screeninganswers.New(repo)

	_, err := store.Update(context.Background(), 1, screeninganswers.Answers{NoticePeriodDays: ptrInt(-1)})
	var ve *screeninganswers.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Update with a negative notice period: err = %v, want a *ValidationError", err)
	}
}

func TestUpdate_ARepositoryFailureIsNotAValidationError(t *testing.T) {
	repo := &fakeRepo{getErr: errors.New("boom")}
	store := screeninganswers.New(repo)

	_, err := store.Update(context.Background(), 1, screeninganswers.Answers{})
	var ve *screeninganswers.ValidationError
	if errors.As(err, &ve) {
		t.Error("a repository failure was wrapped as a *ValidationError, want it passed through as-is")
	}
}

func TestUpdate_RejectsAnInvalidFieldWithoutCallingUpsert(t *testing.T) {
	repo := &fakeRepo{getErr: screeninganswers.ErrNotFound}
	store := screeninganswers.New(repo)

	_, err := store.Update(context.Background(), 1, screeninganswers.Answers{NoticePeriodDays: ptrInt(-1)})
	if err == nil {
		t.Fatal("Update with a negative notice period = nil error, want one")
	}
	if repo.upsertCalled {
		t.Error("repo.Upsert was called despite the invalid input")
	}
}

func TestUpdate_RejectsAnUnrecognizedCountryCodeWithoutCallingUpsert(t *testing.T) {
	repo := &fakeRepo{getErr: screeninganswers.ErrNotFound}
	store := screeninganswers.New(repo)

	_, err := store.Update(context.Background(), 1, screeninganswers.Answers{AuthorizedCountries: []string{"not-a-code"}})
	if err == nil {
		t.Fatal("Update with an unrecognized country code = nil error, want one")
	}
	if repo.upsertCalled {
		t.Error("repo.Upsert was called despite the invalid input")
	}
}

func TestUpdate_PropagatesAGetErrorOtherThanNotFound(t *testing.T) {
	wantErr := errors.New("boom")
	repo := &fakeRepo{getErr: wantErr}
	store := screeninganswers.New(repo)

	_, err := store.Update(context.Background(), 1, screeninganswers.Answers{})
	if !errors.Is(err, wantErr) {
		t.Errorf("Update error = %v, want %v", err, wantErr)
	}
	if repo.upsertCalled {
		t.Error("repo.Upsert was called despite the Get error")
	}
}
