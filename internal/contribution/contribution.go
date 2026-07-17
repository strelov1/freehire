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
	"log"
	"net/url"
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
	BoardByGreenhouseJobID(ctx context.Context, jobID string) (board string, ok bool, err error)
	Record(ctx context.Context, in RecordInput) (Contribution, error)
	ListByUser(ctx context.Context, userID int64) ([]Contribution, error)
}

// Resolver is the network fallback for links whose host is not a recognized ATS: it fetches
// the page and detects an embedded board (a company careers page on its own domain — see
// internal/boardresolve). Optional — a nil resolver keeps the flow network-free.
type Resolver interface {
	Resolve(ctx context.Context, rawURL string) (source, board, canonical string, ok bool)
}

// Service implements the contribution use cases over a Repository. Board recognition is a
// pure, network-free URL parse (board.go); an optional Resolver adds a network fallback for
// vanity domains with an embedded ATS.
type Service struct {
	repo     Repository
	resolver Resolver
}

// New creates a Service backed by the given Repository and an optional network Resolver (nil =
// network-free recognition only).
func New(repo Repository, resolver Resolver) *Service {
	return &Service{repo: repo, resolver: resolver}
}

// Submit validates and records a contributed board, awarding the submitter a point for a novel
// one. The checks run cheapest-first: unsupported ATS (422) before any DB read;
// already-tracked (409) before any write; the record+point transaction last, where a
// duplicate-board race surfaces as ErrBoardAlreadyContributed (409).
func (s *Service) Submit(ctx context.Context, submittedBy int64, rawURL string) (rec Contribution, source, board string, err error) {
	source, board, canonical, ok := RecognizeBoard(rawURL)
	if !ok && s.resolver != nil {
		// Unknown host — the link may be a company careers page with an embedded ATS. Fetch it
		// and detect the board (network fallback).
		source, board, canonical, ok = s.resolver.Resolve(ctx, rawURL)
	}
	if !ok {
		// Server-side embeds expose only the Greenhouse job id (no board token in URL or page).
		// Find the board by that id — only resolves boards we already track (network-free).
		if id, has := greenhouseJobID(rawURL); has {
			if b, found, err := s.repo.BoardByGreenhouseJobID(ctx, id); err == nil && found {
				source, board, canonical, ok = "greenhouse", b, stripQueryFragment(rawURL), true
			}
		}
	}
	if !ok {
		// Log an unrecognized-but-plausible link (a valid http(s) URL) so a maintainer can
		// review the feed and add support for a missed ATS. Garbage (non-URLs) is skipped.
		logUnrecognized(submittedBy, rawURL)
		return Contribution{}, "", "", ErrUnsupportedATS
	}

	tracked, err := s.repo.BoardTracked(ctx, source, board)
	if err != nil {
		return Contribution{}, source, board, err
	}
	if tracked {
		return Contribution{}, source, board, ErrBoardAlreadyTracked
	}

	rec, err = s.repo.Record(ctx, RecordInput{
		SubmittedBy: submittedBy,
		URL:         canonical,
		Source:      source,
		Board:       board,
	})
	return rec, source, board, err
}

// ListMine returns the given user's contributions, newest first.
func (s *Service) ListMine(ctx context.Context, userID int64) ([]Contribution, error) {
	return s.repo.ListByUser(ctx, userID)
}

// logUnrecognized emits a log line for a rejected link that parses as an http(s) URL — a
// review feed for missed ATS. Grep prod for "contribution: unrecognized". Non-URL garbage is
// skipped so the feed stays signal.
func logUnrecognized(submittedBy int64, rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return
	}
	log.Printf("contribution: unrecognized link (user=%d): %s", submittedBy, rawURL)
}

// CompanyForBoard resolves the company already tracked on a (source, board) — for the "we
// already cover this" reply, so the caller can link to the company page. ok=false when the
// board has no resolved company. The (source, board) come from Submit, so no re-recognition
// (or re-fetch, for a resolved vanity domain) is needed.
func (s *Service) CompanyForBoard(ctx context.Context, source, board string) (name, slug string, ok bool) {
	name, slug, found, err := s.repo.CompanyForBoard(ctx, source, board)
	if err != nil || !found {
		return "", "", false
	}
	return name, slug, true
}
