// Package answerbank is the candidate's accumulating record of what they answer to
// employers' screening questions.
//
// It exists because the alternative does not scale: internal/api/atsapply matches a
// question to a stored fact through hand-written rules, and employers author questions
// faster than anyone writes rules. A production application parked on "What is your desired
// salary?" while the candidate's own figure sat in screening_answers, because no rule
// joined the two (freehire, 2026-09-08).
//
// It is a store, not a cache: it accumulates, and only its owner removes anything. No
// reconciler prunes it and no import replaces it — the same rule internal/candidate/
// experience states for the same reason, that a sweeper here would silently discard
// answers the candidate expects to still hold.
//
// It does NOT replace screening_answers, which holds six typed facts the product compares
// and validates (see migration 0155's own comment). This takes the open-ended remainder.
package answerbank

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/dict/answertopic"
)

// MaxAnswerLen bounds one stored answer, in CHARACTERS — what the design states and what a
// form's own counter would show. Long enough for any screening answer a form actually asks
// for, short enough that a pasted essay is refused — that is a cover letter, and
// internal/candidate/coverletter is the path for those. Enforced here rather than only in
// the form because an API key reaches the write route too.
//
// Counted in runes, not bytes. Every Cyrillic character costs two bytes, so a byte bound
// refuses a ~660-character Russian answer as an essay while allowing an English one three
// times its length — and internal/dict/answertopic keys such a question perfectly well, so
// there is nothing else to stop the candidate reaching this.
const MaxAnswerLen = 2000

// Provenance is who asserted an answer. Only the candidate's own may be sent to an
// employer — an answer on an application form is a claim made in their name.
type Provenance string

const (
	// AuthorCandidate is the person, answering from their own session or API key.
	AuthorCandidate Provenance = "candidate"
	// AuthorAgent is a model's reading, stored as a suggestion and never sent. Nothing
	// writes it yet; the value exists so the gate is built before there is anything to let
	// through — adding it later would mean adding the enforcement later too, to code that
	// had never needed it.
	AuthorAgent Provenance = "agent_inferred"
)

// sendable reports whether an answer with this provenance may go to an employer. Anything
// unrecognised — a new entry point that forgets to name itself — is not sendable. Failing
// closed is the point, and it is the same rule internal/candidate/experience applies to its
// own provenance.
func (p Provenance) sendable() bool { return p == AuthorCandidate }

// Answer is one banked answer.
type Answer struct {
	ID         int64
	Topic      string
	Question   string
	Answer     string
	Provenance string
	UpdatedAt  time.Time
}

var (
	// ErrEmptyAnswer refuses a blank answer. A blank is indistinguishable from an
	// unanswered question, and storing one would mark the question answered forever with
	// nothing to send.
	ErrEmptyAnswer = errors.New("answerbank: the answer is empty")
	// ErrUnkeyable refuses a question that folds to no topic — an empty or
	// punctuation-only label, both of which real forms produce. It could never be recalled.
	ErrUnkeyable = errors.New("answerbank: the question cannot be keyed")
	// ErrTooLong refuses an answer past MaxAnswerLen.
	ErrTooLong = errors.New("answerbank: the answer is too long")
	// ErrNotFound reports a delete that matched nothing — a missing id and another
	// candidate's id alike, so a probing caller learns nothing about which.
	ErrNotFound = errors.New("answerbank: no such answer")
)

// Repository is the storage this package needs. An interface so the store's own rules are
// testable without a database, matching how internal/candidate/experience separates the two.
type Repository interface {
	Upsert(ctx context.Context, userID int64, a Answer) error
	List(ctx context.Context, userID int64) ([]Answer, error)
	Delete(ctx context.Context, userID, id int64) (int64, error)
}

// Store is the bank's own rules over a Repository.
type Store struct{ repo Repository }

// NewStore builds a Store.
func NewStore(repo Repository) *Store { return &Store{repo: repo} }

// Save records an answer under the question's topic, replacing any earlier answer on the
// same topic.
//
// by is taken from the ENTRY POINT that received the request, never from a request body: a
// caller naming itself is not evidence of who it is. This is the same rule — and the same
// hazard — as experience.Author and cvedit.Actor.
func (s *Store) Save(ctx context.Context, userID int64, question, answer string, by Provenance) error {
	trimmed := strings.TrimSpace(answer)
	if trimmed == "" {
		return ErrEmptyAnswer
	}
	if utf8.RuneCountInString(trimmed) > MaxAnswerLen {
		return ErrTooLong
	}
	topic, ok := answertopic.Of(question)
	if !ok {
		return ErrUnkeyable
	}
	return s.repo.Upsert(ctx, userID, Answer{
		Topic:      topic,
		Question:   strings.TrimSpace(question),
		Answer:     trimmed,
		Provenance: string(by),
	})
}

// List returns the candidate's whole bank, whatever its provenance — the management surface
// shows an agent's suggestion so the candidate can confirm or correct it. What may be SENT
// is Sendable's question, not this one.
func (s *Store) List(ctx context.Context, userID int64) ([]Answer, error) {
	return s.repo.List(ctx, userID)
}

// Sendable returns topic → answer for the answers that may reach an employer.
func (s *Store) Sendable(ctx context.Context, userID int64) (map[string]string, error) {
	all, err := s.repo.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(all))
	for _, a := range all {
		if Provenance(a.Provenance).sendable() {
			out[a.Topic] = a.Answer
		}
	}
	return out, nil
}

// Delete removes one of the candidate's own answers. Nothing else ever removes from this
// bank.
func (s *Store) Delete(ctx context.Context, userID, id int64) error {
	affected, err := s.repo.Delete(ctx, userID, id)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
