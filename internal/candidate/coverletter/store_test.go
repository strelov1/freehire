package coverletter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeRepo struct {
	row      *Stored
	getErr   error
	saveErr  error
	gotUser  int64
	gotJob   int64
	saved    *Stored
	savedFor [2]int64
}

func (f *fakeRepo) Get(_ context.Context, userID, jobID int64) (Stored, error) {
	f.gotUser, f.gotJob = userID, jobID
	if f.getErr != nil {
		return Stored{}, f.getErr
	}
	if f.row == nil {
		return Stored{}, pgx.ErrNoRows
	}
	return *f.row, nil
}

func (f *fakeRepo) Upsert(_ context.Context, userID, jobID int64, s Stored) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.savedFor = [2]int64{userID, jobID}
	f.saved = &s
	return nil
}

// A pair that was never drafted is not an error. The read path reports "none" and calls no
// model — the whole point of separating GET from POST.
func TestGetReportsNoDraftWithoutAnError(t *testing.T) {
	got, err := NewStore(&fakeRepo{}).Get(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Errorf("Stored = %v, want nil", got)
	}
}

// Ownership is a WHERE clause, and the caller's id has to actually reach it. A store that
// dropped the user id would serve another candidate's letter and look perfectly healthy.
func TestGetScopesToTheCaller(t *testing.T) {
	repo := &fakeRepo{}
	if _, err := NewStore(repo).Get(context.Background(), 42, 7); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if repo.gotUser != 42 || repo.gotJob != 7 {
		t.Errorf("repo saw (user %d, job %d), want (42, 7)", repo.gotUser, repo.gotJob)
	}
}

func TestGetPropagatesARealFailure(t *testing.T) {
	repo := &fakeRepo{getErr: errors.New("connection reset")}
	if _, err := NewStore(repo).Get(context.Background(), 1, 2); err == nil {
		t.Fatal("err = nil, want the underlying failure — a broken read is not an absent draft")
	}
}

func storedWith(model, language string) *Stored {
	return &Stored{
		Letter:    Letter{Body: "hello", Language: language, Cited: []uuid.UUID{uuid.New()}},
		Model:     model,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestStaleWhenTheModelMoved(t *testing.T) {
	s := storedWith("gpt-old", "de")

	if !s.Stale("gpt-new", "de") {
		t.Error("a draft written by a retired model should read stale")
	}
	if s.Stale("gpt-old", "de") {
		t.Error("same model and same language should not read stale")
	}
}

// The language stamp is compared against the VACANCY's language, resolved the same way the
// chain resolves it — otherwise a posting with no detected language would report stale
// forever against a letter correctly written in English.
func TestStaleWhenThePostingLanguageMoved(t *testing.T) {
	s := storedWith("m", "de")

	if !s.Stale("m", "fr") {
		t.Error("a posting re-detected into another language should mark the letter stale")
	}
}

func TestNotStaleWhenThePostingLosesItsLanguageAndTheLetterIsEnglish(t *testing.T) {
	s := storedWith("m", "en")

	if s.Stale("m", "") {
		t.Error("an empty posting language resolves to English, which is what the letter already is")
	}
}

func TestSaveWritesForTheCallerAndTheJob(t *testing.T) {
	repo := &fakeRepo{}
	letter := Letter{Body: "hello", Language: "de"}

	if err := NewStore(repo).Save(context.Background(), 42, 7, letter, "gpt-x"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if repo.savedFor != [2]int64{42, 7} {
		t.Errorf("saved for %v, want [42 7]", repo.savedFor)
	}
	if repo.saved.Model != "gpt-x" {
		t.Errorf("Model = %q, want the model that produced it", repo.saved.Model)
	}
	if repo.saved.Body != "hello" {
		t.Errorf("Body = %q, want the letter's", repo.saved.Body)
	}
}

// A failed write must be a failed write. Swallowing it would leave the caller believing a
// letter is stored when the next read returns the old one, or none.
func TestSavePropagatesAFailure(t *testing.T) {
	repo := &fakeRepo{saveErr: errors.New("disk full")}
	if err := NewStore(repo).Save(context.Background(), 1, 2, Letter{Body: "x"}, "m"); err == nil {
		t.Fatal("err = nil, want the underlying failure")
	}
}
