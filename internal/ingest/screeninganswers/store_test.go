package screeninganswers_test

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/screeninganswers"
)

// fakeRepo records the params it is handed and returns canned records/errors, so the
// store tests run without a database. UpdateLocked simulates the real QueriesRepository's
// locked read-merge-write: it feeds getRet/getErr to merge exactly as a real transaction's
// locked SELECT would (ErrNotFound reading as a fully-unstated record), records what merge
// returned, and answers with upsertRet/upsertErr — so Store.Update's callers here never
// call Get and Upsert as two separate, unlocked steps.
type fakeRepo struct {
	getRet screeninganswers.Answers
	getErr error

	updateLockedCalls int
	upserted          screeninganswers.Answers
	upsertCalled      bool
	upsertRet         screeninganswers.Answers
	upsertErr         error
}

func (f *fakeRepo) Get(_ context.Context, _ int64) (screeninganswers.Answers, error) {
	return f.getRet, f.getErr
}

func (f *fakeRepo) UpdateLocked(_ context.Context, _ int64, merge func(screeninganswers.Answers) screeninganswers.Answers) (screeninganswers.Answers, error) {
	f.updateLockedCalls++
	existing := f.getRet
	if f.getErr != nil {
		if !errors.Is(f.getErr, screeninganswers.ErrNotFound) {
			return screeninganswers.Answers{}, f.getErr
		}
		existing = screeninganswers.Answers{}
	}
	f.upserted = merge(existing)
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

// spyRepo records the order Store.Update calls its Repository methods in, so this test can
// prove the read-merge-write happens as one call into the Repository rather than as a
// separate Get followed by a separate write — the shape that let two concurrent Update
// calls for the same userID both read the same "existing" record and lose one's fields to
// the other's write (the bug this fix closes). A repository that still called Get from
// Store.Update, instead of doing its own locked read inside UpdateLocked, would show up
// here as more than one recorded call.
type spyRepo struct {
	calls []string
}

func (s *spyRepo) Get(_ context.Context, _ int64) (screeninganswers.Answers, error) {
	s.calls = append(s.calls, "Get")
	return screeninganswers.Answers{}, screeninganswers.ErrNotFound
}

func (s *spyRepo) UpdateLocked(_ context.Context, _ int64, merge func(screeninganswers.Answers) screeninganswers.Answers) (screeninganswers.Answers, error) {
	s.calls = append(s.calls, "UpdateLocked")
	return merge(screeninganswers.Answers{}), nil
}

func TestUpdate_ReadsMergesAndWritesAsOneRepositoryCallNotGetThenUpsert(t *testing.T) {
	repo := &spyRepo{}
	store := screeninganswers.New(repo)

	if _, err := store.Update(context.Background(), 1, screeninganswers.Answers{WillingToRelocate: ptrBool(true)}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	want := []string{"UpdateLocked"}
	if len(repo.calls) != len(want) || repo.calls[0] != want[0] {
		t.Errorf("repo calls = %v, want %v — Store.Update must not read and write as two separate, unlocked Repository calls", repo.calls, want)
	}
}

func TestUpdate_RefusesAFieldBothStatedAndCleared(t *testing.T) {
	repo := &fakeRepo{}
	store := screeninganswers.New(repo)
	salary := 100000

	_, err := store.Update(context.Background(), 1,
		screeninganswers.Answers{DesiredSalaryAmount: &salary}, screeninganswers.FieldDesiredSalaryAmount)

	var ve *screeninganswers.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want a ValidationError", err)
	}
	if repo.updateLockedCalls != 0 {
		t.Errorf("UpdateLocked called %d times, want 0", repo.updateLockedCalls)
	}
}

func TestUpdate_ClearsAStoredField(t *testing.T) {
	salary := 100000
	repo := &fakeRepo{getRet: screeninganswers.Answers{DesiredSalaryAmount: &salary, AuthorizedCountries: []string{"de"}}}
	store := screeninganswers.New(repo)

	if _, err := store.Update(context.Background(), 1, screeninganswers.Answers{}, screeninganswers.FieldDesiredSalaryAmount); err != nil {
		t.Fatalf("Update: %v", err)
	}

	if repo.upserted.DesiredSalaryAmount != nil {
		t.Errorf("upserted salary = %v, want nil", *repo.upserted.DesiredSalaryAmount)
	}
	if len(repo.upserted.AuthorizedCountries) != 1 {
		t.Errorf("upserted countries = %v, want [de] untouched", repo.upserted.AuthorizedCountries)
	}
}
