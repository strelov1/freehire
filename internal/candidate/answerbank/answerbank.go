// Package answerbank is the candidate's accumulating record of their answers to
// employers' screening questions: the open-ended remainder that the six typed facts in
// screening_answers do not cover.
//
// It is a store, not a cache: only its owner removes anything. No reconciler prunes it and
// no import replaces it, as in internal/candidate/experience.
package answerbank

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/dict/answertopic"
)

// MaxAnswerLen bounds one stored answer, in characters (runes, not bytes: Cyrillic costs two
// bytes per character). Longer is a cover letter; see internal/candidate/coverletter.
// Enforced here, not only in the form, because an API key reaches the write route too.
const MaxAnswerLen = 2000

// Provenance is who asserted an answer. Only the candidate's own may be sent to an
// employer — an answer on an application form is a claim made in their name.
type Provenance string

const (
	// AuthorCandidate is the person, answering from their own session or API key.
	AuthorCandidate Provenance = "candidate"
	// AuthorAgent is a model's reading, stored as a suggestion and never sent. Nothing writes
	// it yet; it exists so the send gate is built before there is anything to let through.
	AuthorAgent Provenance = "agent_inferred"
)

// sendable reports whether an answer with this provenance may go to an employer. Anything
// unrecognised is not sendable (fail closed), as in internal/candidate/experience.
func (p Provenance) sendable() bool { return p == AuthorCandidate }

// known reports whether this is one of the two members of the vocabulary. Save refuses
// anything else: a stored typo would fail silently at the read, since sendable() declines it.
func (p Provenance) known() bool { return p == AuthorCandidate || p == AuthorAgent }

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
	// ErrEmptyAnswer refuses a blank answer, which would mark the question answered forever
	// with nothing to send.
	ErrEmptyAnswer = errors.New("answerbank: the answer is empty")
	// ErrUnkeyable refuses a question that folds to no topic (an empty or punctuation-only
	// label), which could never be recalled.
	ErrUnkeyable = errors.New("answerbank: the question cannot be keyed")
	// ErrTooLong refuses an answer past MaxAnswerLen.
	ErrTooLong = errors.New("answerbank: the answer is too long")
	// ErrNotFound reports a delete that matched nothing. A missing id and another candidate's
	// id look alike, so a probing caller learns nothing.
	ErrNotFound = errors.New("answerbank: no such answer")
	// ErrUnknownProvenance refuses a provenance outside the vocabulary. See Provenance.known.
	ErrUnknownProvenance = errors.New("answerbank: unknown provenance")
)

// Repository is the storage this package needs, an interface so the store's rules are
// testable without a database.
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
// by comes from the entry point that received the request, never from a request body (as
// experience.Author and cvedit.Actor).
func (s *Store) Save(ctx context.Context, userID int64, question, answer string, by Provenance) error {
	if !by.known() {
		return ErrUnknownProvenance
	}
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

// List returns the candidate's whole bank, whatever its provenance, so an agent's suggestion
// can be confirmed or corrected. What may be sent is Sendable's question.
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

// Delete removes one of the candidate's own answers. Nothing else removes from this bank.
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
