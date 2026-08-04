// Package contribution is the crowdsourced "contribute a board" flow: a signed-in user
// pastes a job link from a supported multi-tenant ATS — a vacancy URL or a bare board-listing
// URL — and, without any network fetch, the service derives the company board (source, board)
// from the URL alone, rejects a board we already crawl or someone already contributed, and
// otherwise records the board (the handler then awards the submitter AI credits). The ingest
// side later onboards the board and scrapes all its vacancies, so the unit of a contribution is
// the board, not a single vacancy; this package records, and the handler rewards.
package contribution

import (
	"context"
	"errors"
	"log"
	"net/url"
	"time"

	"github.com/strelov1/freehire/internal/atsboard"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrUnsupportedATS is a link that is not a supported multi-tenant ATS board — a
	// single-tenant source, an unknown host, or a URL with no board segment (mapped to 422).
	ErrUnsupportedATS = errors.New("contribution: unsupported ATS board link")
	// ErrBoardAlreadyTracked is a board the catalogue already crawls (a job exists for it) —
	// contributing it adds nothing, so no reward (409).
	ErrBoardAlreadyTracked = errors.New("contribution: board already in catalogue")
	// ErrBoardAlreadyContributed is a board already recorded by any user and still live in the
	// queue (pending/review/onboarded) — the repository maps the unique violation to this (409).
	// A rejected row does not reserve its board: that one can be contributed again.
	ErrBoardAlreadyContributed = errors.New("contribution: board already contributed")
)

// Contribution statuses. A recognized novel board is recorded StatusPending (credit awarded);
// an unrecognized-but-valid link is recorded StatusReview (no credit) for manual triage.
const (
	StatusPending = "pending"
	StatusReview  = "review"
)

// Surfaces — the door an intake came through. Stored alongside the submitter so repeated or
// abusive use is visible in the data rather than only in logs. SurfaceUnknown covers a client
// that sends no tag (or one we do not know): an intake is served, never refused, for it.
const (
	SurfaceWeb       = "web"
	SurfaceTelegram  = "telegram"
	SurfaceDiscord   = "discord"
	SurfaceExtension = "extension"
	SurfaceCLI       = "cli"
	SurfaceUnknown   = "unknown"
)

// NormalizeSurface maps a caller-supplied surface tag to one this package stores, falling back
// to SurfaceUnknown. It is deliberately lenient: an older client build, or one we have not
// heard of, still gets its link processed — the tag is for our own visibility, not a gate.
// The repository applies it too, on the way to a column with a CHECK constraint: an intake must
// never be REFUSED over a tag, which is exactly what an unnormalised empty string would do.
func NormalizeSurface(s string) string {
	switch s {
	case SurfaceWeb, SurfaceTelegram, SurfaceDiscord, SurfaceExtension, SurfaceCLI:
		return s
	default:
		return SurfaceUnknown
	}
}

// Contribution is a stored contribution row, decoupled from the generated db row. CreatedAt
// is *time.Time because the handler serializes it.
type Contribution struct {
	ID          int64
	SubmittedBy int64
	URL         string
	Source      string
	Board       string
	Status      string
	Surface     string
	CreatedAt   *time.Time
}

// RecordInput is the validated board the service asks the repository to persist.
type RecordInput struct {
	SubmittedBy int64
	URL         string
	Source      string
	Board       string
	Surface     string
}

// Repository is the persistence contract, expressed in package domain types. Record inserts
// the contribution, mapping a duplicate board to ErrBoardAlreadyContributed.
type Repository interface {
	BoardTracked(ctx context.Context, source, board string) (bool, error)
	CompanyForBoard(ctx context.Context, source, board string) (name, slug string, ok bool, err error)
	BoardByGreenhouseJobID(ctx context.Context, jobID string) (board string, ok bool, err error)
	BoardByAshbyJobID(ctx context.Context, jobID string) (board string, ok bool, err error)
	Record(ctx context.Context, in RecordInput) (Contribution, error)
	RecordReview(ctx context.Context, submittedBy int64, url, surface string) (Contribution, error)
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

// SubmitInput is one intake: who pasted the link, the link, and the surface it arrived
// through.
type SubmitInput struct {
	SubmittedBy int64
	URL         string
	Surface     string
}

// SubmitResult is a recorded intake. Source and Board name the board the link resolved to and
// are set even when recording was refused (ErrBoardAlreadyTracked), so the caller can name the
// company it already covers. Rewardable is true when this submission is the one that opened the
// board — the caller grants the AI credits, this package decides eligibility. A duplicate never
// gets that far: the unique index refuses the insert, surfaced as ErrBoardAlreadyContributed.
type SubmitResult struct {
	Contribution Contribution
	Source       string
	Board        string
	Rewardable   bool
}

// Intake is what a link is, before anything is written: the board it belongs to, and whether
// the catalogue already crawls that board. Recognized is false for a link no board can be
// derived from, which is destined for the review queue rather than the board queue.
type Intake struct {
	Source     string
	Board      string
	Canonical  string
	Recognized bool
	Tracked    bool
}

// Inspect resolves a link to its board and reports whether the catalogue already crawls it,
// WITHOUT writing anything. It exists because the answer changes the moment a vacancy from
// that board is imported: a caller that imports first and asks afterwards is told the board is
// tracked by the very posting it just wrote. Ask first, import second, record third.
func (s *Service) Inspect(ctx context.Context, rawURL string) (Intake, error) {
	source, board, canonical, ok := s.resolveBoard(ctx, rawURL)
	if !ok {
		return Intake{}, nil
	}
	tracked, err := s.repo.BoardTracked(ctx, source, board)
	if err != nil {
		return Intake{}, err
	}
	return Intake{Source: source, Board: board, Canonical: canonical, Recognized: true, Tracked: tracked}, nil
}

// RecordIntake persists a link already inspected, so the board is resolved once even when
// resolving it cost a page fetch. A tracked board is not recorded — it needs no onboarding —
// and an unrecognised link goes to the review queue.
func (s *Service) RecordIntake(ctx context.Context, in SubmitInput, it Intake) (SubmitResult, error) {
	surface := NormalizeSurface(in.Surface)
	if !it.Recognized {
		if !isHTTPURL(in.URL) {
			return SubmitResult{}, ErrUnsupportedATS
		}
		logUnrecognized(in.SubmittedBy, in.URL)
		rec, err := s.repo.RecordReview(ctx, in.SubmittedBy, stripQueryFragment(in.URL), surface)
		return SubmitResult{Contribution: rec}, err
	}
	if it.Tracked {
		return SubmitResult{Source: it.Source, Board: it.Board}, ErrBoardAlreadyTracked
	}
	rec, err := s.repo.Record(ctx, RecordInput{
		SubmittedBy: in.SubmittedBy,
		URL:         it.Canonical,
		Source:      it.Source,
		Board:       it.Board,
		Surface:     surface,
	})
	return SubmitResult{Contribution: rec, Source: it.Source, Board: it.Board, Rewardable: err == nil}, err
}

// Submit inspects and records in one call, for the callers that do not import first. The
// checks run cheapest-first: unsupported ATS before any DB read; already-tracked before any
// write; the record insert last, which no longer competes with a unique constraint — a repeat
// board is recorded, it simply is not rewardable.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (SubmitResult, error) {
	surface := NormalizeSurface(in.Surface)
	source, board, canonical, ok := s.resolveBoard(ctx, in.URL)
	if !ok {
		// No board resolved. A non-URL is garbage → 422. A valid http(s) link is a plausible
		// missed ATS: log it for maintainer visibility and record it in the review queue (no
		// board, no credit) so it can be triaged by hand instead of being lost.
		if !isHTTPURL(in.URL) {
			return SubmitResult{}, ErrUnsupportedATS
		}
		logUnrecognized(in.SubmittedBy, in.URL)
		rec, err := s.repo.RecordReview(ctx, in.SubmittedBy, stripQueryFragment(in.URL), surface)
		return SubmitResult{Contribution: rec}, err
	}

	tracked, err := s.repo.BoardTracked(ctx, source, board)
	if err != nil {
		return SubmitResult{Source: source, Board: board}, err
	}
	if tracked {
		return SubmitResult{Source: source, Board: board}, ErrBoardAlreadyTracked
	}

	rec, err := s.repo.Record(ctx, RecordInput{
		SubmittedBy: in.SubmittedBy,
		URL:         canonical,
		Source:      source,
		Board:       board,
		Surface:     surface,
	})
	return SubmitResult{Contribution: rec, Source: source, Board: board, Rewardable: err == nil}, err
}

// resolveBoard finds the (source, board, canonical URL) a pasted link belongs to, trying the
// cheapest strategy first: the network-free URL recognizer; then a page fetch that detects an
// ATS embedded on a company's own careers domain; then a Greenhouse or Ashby job-id lookup, for
// embeds that expose only the job id (resolves boards we already track). ok=false when none
// resolve.
func (s *Service) resolveBoard(ctx context.Context, rawURL string) (source, board, canonical string, ok bool) {
	if source, board, canonical, ok := atsboard.Recognize(rawURL); ok {
		return source, board, canonical, true
	}
	if s.resolver != nil {
		if source, board, canonical, ok := s.resolver.Resolve(ctx, rawURL); ok {
			return source, board, canonical, true
		}
	}
	if id, has := greenhouseJobID(rawURL); has {
		if b, found, err := s.repo.BoardByGreenhouseJobID(ctx, id); err == nil && found {
			return "greenhouse", b, stripQueryFragment(rawURL), true
		}
	}
	if id, has := ashbyJobID(rawURL); has {
		if b, found, err := s.repo.BoardByAshbyJobID(ctx, id); err == nil && found {
			return "ashby", b, stripQueryFragment(rawURL), true
		}
	}
	return "", "", "", false
}

// ListMine returns the given user's contributions, newest first.
func (s *Service) ListMine(ctx context.Context, userID int64) ([]Contribution, error) {
	return s.repo.ListByUser(ctx, userID)
}

// isHTTPURL reports whether rawURL parses as an http(s) URL with a host. It is the review-queue
// gate: a valid link is recorded for triage, non-URL garbage is rejected (422).
func isHTTPURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// logUnrecognized emits a log line for an unrecognized link that parses as an http(s) URL — a
// review feed for missed ATS. Grep prod for "contribution: unrecognized". Non-URL garbage is
// skipped so the feed stays signal.
func logUnrecognized(submittedBy int64, rawURL string) {
	if !isHTTPURL(rawURL) {
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
