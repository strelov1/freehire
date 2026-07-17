// Package contribution is the crowdsourced "contribute a board" flow: a signed-in user
// pastes a job link from a supported multi-tenant ATS — a vacancy URL or a bare board-listing
// URL — and, without any network fetch, the service derives the company board (source, board)
// from the URL alone, rejects a board we already crawl or someone already contributed, and
// otherwise records the board and awards the submitter one point. The ingest side later
// onboards the board and scrapes all its vacancies, so the unit of a contribution is the
// board, not a single vacancy; this package records and rewards.
package contribution

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrUnsupportedATS is a link that is not a supported multi-tenant ATS board — a
	// single-tenant source, an unknown host, or a URL with no board segment (mapped to 422).
	ErrUnsupportedATS = errors.New("contribution: unsupported ATS board link")
	// ErrBoardAlreadyTracked is a board the catalogue already crawls (a job exists for it) —
	// contributing it adds nothing, so no point (409).
	ErrBoardAlreadyTracked = errors.New("contribution: board already in catalogue")
	// ErrBoardAlreadyContributed is a board already recorded by any user — the repository maps
	// the unique violation to this (409).
	ErrBoardAlreadyContributed = errors.New("contribution: board already contributed")
)

// Contribution is a stored contribution row, decoupled from the generated db row. CreatedAt
// is *time.Time because the handler serializes it.
type Contribution struct {
	ID          int64
	SubmittedBy int64
	URL         string
	Source      string
	Board       string
	Status      string
	CreatedAt   *time.Time
}

// RecordInput is the validated, deduped board the service asks the repository to persist
// (with the point award) in one transaction.
type RecordInput struct {
	SubmittedBy int64
	URL         string
	Source      string
	Board       string
}

// Repository is the persistence contract, expressed in package domain types. Record inserts
// the contribution and increments the submitter's points atomically, mapping a duplicate
// board to ErrBoardAlreadyContributed.
type Repository interface {
	BoardTracked(ctx context.Context, source, board string) (bool, error)
	CompanyForBoard(ctx context.Context, source, board string) (name, slug string, ok bool, err error)
	Record(ctx context.Context, in RecordInput) (Contribution, error)
	ListByUser(ctx context.Context, userID int64) ([]Contribution, error)
}

// Service implements the contribution use cases over a Repository. Board recognition is a
// pure, network-free URL parse (see board.go).
type Service struct {
	repo Repository
}

// New creates a Service backed by the given Repository.
func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// Submit validates and records a contributed board, awarding the submitter a point for a novel
// one. The checks run cheapest-first: unsupported ATS (422) before any DB read;
// already-tracked (409) before any write; the record+point transaction last, where a
// duplicate-board race surfaces as ErrBoardAlreadyContributed (409).
func (s *Service) Submit(ctx context.Context, submittedBy int64, rawURL string) (Contribution, error) {
	source, board, canonical, ok := recognizeBoard(rawURL)
	if !ok {
		return Contribution{}, ErrUnsupportedATS
	}

	tracked, err := s.repo.BoardTracked(ctx, source, board)
	if err != nil {
		return Contribution{}, err
	}
	if tracked {
		return Contribution{}, ErrBoardAlreadyTracked
	}

	return s.repo.Record(ctx, RecordInput{
		SubmittedBy: submittedBy,
		URL:         canonical,
		Source:      source,
		Board:       board,
	})
}

// ListMine returns the given user's contributions, newest first.
func (s *Service) ListMine(ctx context.Context, userID int64) ([]Contribution, error) {
	return s.repo.ListByUser(ctx, userID)
}

// TrackedCompany resolves the company already tracked on the board a link points to — for the
// "we already cover this" reply, so the caller can link to the company page. ok=false when the
// link is unrecognized or the board has no resolved company.
func (s *Service) TrackedCompany(ctx context.Context, rawURL string) (name, slug string, ok bool) {
	source, board, _, recognized := recognizeBoard(rawURL)
	if !recognized {
		return "", "", false
	}
	name, slug, found, err := s.repo.CompanyForBoard(ctx, source, board)
	if err != nil || !found {
		return "", "", false
	}
	return name, slug, true
}
