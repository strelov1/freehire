package answerbank

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeRepo is an in-memory Repository. Keyed the way the table is — by (user, topic) — so
// the double cannot disagree with the UNIQUE constraint about what an upsert replaces.
type fakeRepo struct {
	rows map[int64]map[string]Answer
	next int64
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[int64]map[string]Answer{}} }

func (r *fakeRepo) Upsert(_ context.Context, userID int64, a Answer) error {
	if r.rows[userID] == nil {
		r.rows[userID] = map[string]Answer{}
	}
	existing, ok := r.rows[userID][a.Topic]
	if ok {
		a.ID = existing.ID
	} else {
		r.next++
		a.ID = r.next
	}
	r.rows[userID][a.Topic] = a
	return nil
}

func (r *fakeRepo) List(_ context.Context, userID int64) ([]Answer, error) {
	var out []Answer
	for _, a := range r.rows[userID] {
		out = append(out, a)
	}
	return out, nil
}

func (r *fakeRepo) Delete(_ context.Context, userID, id int64) (int64, error) {
	for topic, a := range r.rows[userID] {
		if a.ID == id {
			delete(r.rows[userID], topic)
			return 1, nil
		}
	}
	return 0, nil
}

func TestSave_TheSameQuestionPhrasedTwiceReplacesRatherThanDuplicates(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	if err := s.Save(ctx, 1, "What is your desired salary?", "5000 USD per year", AuthorCandidate); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save(ctx, 1, "Desired salary", "6000 USD per year", AuthorCandidate); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.List(ctx, 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("bank holds %d answers, want 1 — the two phrasings are one question: %+v", len(got), got)
	}
	if got[0].Answer != "6000 USD per year" {
		t.Errorf("answer = %q, want the newer one", got[0].Answer)
	}
}

// The gate the whole design rests on: an answer a model asserted is stored, listed, and
// never sent to an employer.
func TestSendable_OmitsAnAnswerTheCandidateDidNotGive(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	if err := s.Save(ctx, 1, "Which state do you currently reside in?", "Santa Catarina", AuthorCandidate); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save(ctx, 1, "How many years of Go do you have?", "8", AuthorAgent); err != nil {
		t.Fatalf("Save: %v", err)
	}

	sendable, err := s.Sendable(ctx, 1)
	if err != nil {
		t.Fatalf("Sendable: %v", err)
	}
	if len(sendable) != 1 {
		t.Fatalf("sendable = %+v, want only the candidate's own answer", sendable)
	}
	for _, v := range sendable {
		if v != "Santa Catarina" {
			t.Errorf("sendable carries %q, want the candidate-authored answer", v)
		}
	}

	all, err := s.List(ctx, 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("List returned %d, want both — an agent's answer is stored and shown, just not sent", len(all))
	}
}

// A caller naming its own provenance is not evidence of who it is: anything unrecognised
// falls to the label that cannot be sent. Failing closed is the point.
func TestSave_AnUnrecognisedAuthorIsNotSendable(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	if err := s.Save(ctx, 1, "Do you have a work permit?", "Yes", Provenance("totally-the-candidate-honest")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	sendable, err := s.Sendable(ctx, 1)
	if err != nil {
		t.Fatalf("Sendable: %v", err)
	}
	if len(sendable) != 0 {
		t.Errorf("sendable = %+v, want empty — an unrecognised author must fail closed", sendable)
	}
}

func TestSave_RefusesWhatCannotBeStoredHonestly(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	// An empty answer is not an answer. Storing one marks the question answered forever
	// with nothing to send.
	if err := s.Save(ctx, 1, "Which state?", "   ", AuthorCandidate); !errors.Is(err, ErrEmptyAnswer) {
		t.Errorf("Save(blank answer) = %v, want ErrEmptyAnswer", err)
	}
	// A question that cannot be keyed cannot be recalled.
	if err := s.Save(ctx, 1, "???", "Yes", AuthorCandidate); !errors.Is(err, ErrUnkeyable) {
		t.Errorf("Save(unkeyable question) = %v, want ErrUnkeyable", err)
	}
	// An essay is a cover letter, and there is a separate path for those.
	if err := s.Save(ctx, 1, "Why us?", strings.Repeat("a", MaxAnswerLen+1), AuthorCandidate); !errors.Is(err, ErrTooLong) {
		t.Errorf("Save(oversize answer) = %v, want ErrTooLong", err)
	}
}

func TestDelete_AForeignAnswerIsNotFound(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	if err := s.Save(ctx, 1, "Which state?", "SC", AuthorCandidate); err != nil {
		t.Fatalf("Save: %v", err)
	}
	mine, _ := s.List(ctx, 1)

	if err := s.Delete(ctx, 2, mine[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete(another user's answer) = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, 1, mine[0].ID); err != nil {
		t.Errorf("Delete(own answer) = %v, want nil", err)
	}
}

// The bound is on CHARACTERS, as the design says and as the form's own counter would show.
// Counting bytes refuses a ~660-character Cyrillic answer as though it were a pasted essay
// — every rune costs two bytes — and that becomes reachable the moment the fold accepts
// non-ASCII text at all.
func TestSave_BoundsTheAnswerInRunesNotBytes(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	atTheLimit := strings.Repeat("я", MaxAnswerLen)
	if err := s.Save(ctx, 1, "Расскажите о себе", atTheLimit, AuthorCandidate); err != nil {
		t.Fatalf("Save refused an answer of exactly %d characters: %v", MaxAnswerLen, err)
	}

	overTheLimit := strings.Repeat("я", MaxAnswerLen+1)
	if err := s.Save(ctx, 1, "Расскажите о себе", overTheLimit, AuthorCandidate); !errors.Is(err, ErrTooLong) {
		t.Fatalf("Save(%d characters) = %v, want ErrTooLong", MaxAnswerLen+1, err)
	}
}

// A question in a language the dictionary is not written in still banks and still recalls.
func TestSave_KeysANonLatinQuestion(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	if err := s.Save(ctx, 1, "Какой у вас желаемый доход?", "5000 USD в год", AuthorCandidate); err != nil {
		t.Fatalf("Save: %v", err)
	}
	sendable, err := s.Sendable(ctx, 1)
	if err != nil {
		t.Fatalf("Sendable: %v", err)
	}
	if len(sendable) != 1 {
		t.Fatalf("Sendable = %v, want the banked answer to a readable question", sendable)
	}
}
