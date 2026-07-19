// Package referral is the employee-referral use case: members offer to refer into a
// company (proof CV + manual moderation), and seekers ask that company's pool of
// approved referrers for a referral. A request is company-scoped — every approved
// referrer of the company sees it and whichever one acts records the outcome — so the
// referrer stays anonymous to the seeker until they reach out over the contact the
// seeker provided. This package owns the offer/request lifecycle, its validation, and
// the best-effort ping to referrers; persistence is a Repository, delivery a Pinger.
package referral

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"
)

// Offer status vocabulary: an offer waits pending until a moderator approves or rejects
// it, and only an approved offer makes a company referral-eligible.
const (
	OfferPending  = "pending"
	OfferApproved = "approved"
	OfferRejected = "rejected"
)

// Request status vocabulary: a request is sent until a referrer marks it contacted or
// declined, which frees the seeker to request that company again.
const (
	RequestSent      = "sent"
	RequestContacted = "contacted"
	RequestDeclined  = "declined"
)

// CV attachment kinds: the seeker's stored original résumé, or a builder CV (cv_id).
const (
	CVOriginal = "original"
	CVBuilt    = "built"
)

// DefaultDailyRequestCap bounds how many referral requests one seeker may create in a
// rolling 24h window. Requests are free but spam-resistant; tune as usage shows.
const DefaultDailyRequestCap = 10

// Sentinel errors, mapped to HTTP statuses by the handler.
var (
	// ErrProofRequired is an offer submitted without a proof CV (422).
	ErrProofRequired = errors.New("referral: proof CV required")
	// ErrAlreadyOffered is a second offer for a company the member already offered for;
	// the repository maps the unique violation to this (409).
	ErrAlreadyOffered = errors.New("referral: already offered for this company")
	// ErrCompanyNotFound is an offer for a company slug not in the catalogue; the
	// repository maps the company foreign-key violation to this (404).
	ErrCompanyNotFound = errors.New("referral: company not found")
	// ErrOfferNotPending is a decision on an offer that is not pending — already decided,
	// or absent; the repository maps the no-row update to this (409).
	ErrOfferNotPending = errors.New("referral: offer is not pending")
	// ErrNoContact is a request with neither a Telegram handle nor an email (422).
	ErrNoContact = errors.New("referral: at least one contact required")
	// ErrInvalidCVChoice is a CV choice that violates the kind/id invariant: an original
	// carrying a cv_id, a built without one, or an unknown kind (422).
	ErrInvalidCVChoice = errors.New("referral: invalid CV choice")
	// ErrNoResume is an 'original' CV choice by a seeker who has no stored CV (422).
	ErrNoResume = errors.New("referral: no stored CV to attach")
	// ErrCompanyNotEligible is a request into a company with no approved referrer (409).
	ErrCompanyNotEligible = errors.New("referral: company has no approved referrer")
	// ErrDailyCapReached is a seeker exceeding the rolling-24h request cap (429).
	ErrDailyCapReached = errors.New("referral: daily request cap reached")
	// ErrAlreadyRequested is a second active request for a company the seeker already has a
	// sent request for; the repository maps the partial-unique violation to this (409).
	ErrAlreadyRequested = errors.New("referral: an active request already exists")
	// ErrRequestNotFound is an action addressed to a request id that does not exist (404).
	ErrRequestNotFound = errors.New("referral: request not found")
	// ErrRequestNotOpen is a mark on a request that is no longer sent — already resolved by
	// another referrer; the repository maps the no-row update to this (409).
	ErrRequestNotOpen = errors.New("referral: request is not open")
	// ErrNotAuthorized is a referrer acting on / viewing a request for a company they are not
	// an approved referrer of (403).
	ErrNotAuthorized = errors.New("referral: not an approved referrer for this company")
)

// Offer is a stored referral offer, decoupled from the generated db row. The pointer
// timestamps and DecidedBy are nil until a moderator decides.
type Offer struct {
	ID          int64
	UserID      int64
	CompanySlug string
	ProofKey    string
	Status      string
	DecidedBy   *int64
	DecidedAt   *time.Time
	CreatedAt   *time.Time
}

// Request is a stored referral request, decoupled from the generated db row. JobID and
// CVID are nil for "no source vacancy" and an original-CV attachment respectively; ActedBy
// and ActedAt are nil until a referrer marks it.
type Request struct {
	ID              int64
	SeekerUserID    int64
	CompanySlug     string
	JobID           *int64
	CVKind          string
	CVID            *int64
	ContactTelegram string
	ContactEmail    string
	Note            string
	Status          string
	ActedBy         *int64
	ActedAt         *time.Time
	CreatedAt       *time.Time
}

// Recipient is one approved referrer's reachable channels for a ping. ChatID is 0 when the
// referrer has no linked Telegram; email is always present.
type Recipient struct {
	UserID int64
	Email  string
	ChatID int64
}

// OfferInput is the offer the service asks the repository to persist.
type OfferInput struct {
	UserID      int64
	CompanySlug string
	ProofKey    string
}

// RequestInput is the referral request the service validates and persists. CVID is set iff
// CVKind is CVBuilt; JobID is the optional source vacancy.
type RequestInput struct {
	SeekerUserID    int64
	CompanySlug     string
	JobID           *int64
	CVKind          string
	CVID            *int64
	ContactTelegram string
	ContactEmail    string
	Note            string
}

// Repository is the persistence contract in package domain types. CreateOffer maps the
// (user, company) unique violation to ErrAlreadyOffered; CreateRequest maps the active
// partial-unique violation to ErrAlreadyRequested; DecideOffer and ResolveRequest map a
// no-row update (the status guard) to ErrOfferNotPending / ErrRequestNotOpen.
type Repository interface {
	CreateOffer(ctx context.Context, in OfferInput) (Offer, error)
	DecideOffer(ctx context.Context, offerID, moderatorID int64, status string) (Offer, error)
	GetOffer(ctx context.Context, offerID int64) (Offer, bool, error)
	ListOffersByUser(ctx context.Context, userID int64) ([]Offer, error)
	ListPendingOffers(ctx context.Context) ([]Offer, error)
	CompanyHasApprovedReferrer(ctx context.Context, companySlug string) (bool, error)
	ReferrerApprovedForCompany(ctx context.Context, userID int64, companySlug string) (bool, error)
	ApprovedReferrerRecipients(ctx context.Context, companySlug string) ([]Recipient, error)
	CVBelongsToUser(ctx context.Context, cvID, userID int64) (bool, error)
	UserHasResume(ctx context.Context, userID int64) (bool, error)

	CreateRequest(ctx context.Context, in RequestInput) (Request, error)
	CountRequestsSince(ctx context.Context, seekerID int64, since time.Time) (int64, error)
	GetRequest(ctx context.Context, id int64) (Request, bool, error)
	ResolveRequest(ctx context.Context, id, actorID int64, status string) (Request, error)
	ListRequestsBySeeker(ctx context.Context, seekerID int64) ([]Request, error)
	ListIncomingRequests(ctx context.Context, referrerID int64) ([]Request, error)
}

// Pinger delivers a short "you have a new referral request" notice to one referrer,
// linking to their cabinet inbox. Delivery is best-effort — a Ping error must not fail the
// seeker's request — so the service logs and moves on.
type Pinger interface {
	PingReferrer(ctx context.Context, r Recipient, cabinetURL string) error
}

// Config tunes a Service.
type Config struct {
	// DailyRequestCap is the rolling-24h per-seeker request limit; 0 → DefaultDailyRequestCap.
	DailyRequestCap int
	// CabinetURL is the referrer inbox URL a ping links to (frontend origin + inbox path).
	CabinetURL string
	// Now is the clock, injectable for tests; nil → time.Now.
	Now func() time.Time
}

// Service implements the referral use cases over a Repository and a Pinger.
type Service struct {
	repo       Repository
	pinger     Pinger
	dailyCap   int
	cabinetURL string
	now        func() time.Time
}

// New builds a Service. A zero DailyRequestCap falls back to DefaultDailyRequestCap and a
// nil Now to time.Now.
func New(repo Repository, pinger Pinger, cfg Config) *Service {
	cap := cfg.DailyRequestCap
	if cap <= 0 {
		cap = DefaultDailyRequestCap
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, pinger: pinger, dailyCap: cap, cabinetURL: cfg.CabinetURL, now: now}
}

// SubmitOffer records a member's offer to refer into a company, awaiting moderation. It
// rejects a missing proof CV up front and a duplicate offer via ErrAlreadyOffered.
func (s *Service) SubmitOffer(ctx context.Context, in OfferInput) (Offer, error) {
	if strings.TrimSpace(in.ProofKey) == "" {
		return Offer{}, ErrProofRequired
	}
	return s.repo.CreateOffer(ctx, in)
}

// DecideOffer approves or rejects a pending offer, recording the moderator. A decision on an
// already-decided or absent offer returns ErrOfferNotPending.
func (s *Service) DecideOffer(ctx context.Context, offerID, moderatorID int64, approve bool) (Offer, error) {
	status := OfferApproved
	if !approve {
		status = OfferRejected
	}
	return s.repo.DecideOffer(ctx, offerID, moderatorID, status)
}

// GetOffer returns one offer by id — for the moderator's proof-CV view. ok is false when
// the offer does not exist.
func (s *Service) GetOffer(ctx context.Context, offerID int64) (Offer, bool, error) {
	return s.repo.GetOffer(ctx, offerID)
}

// ListMyOffers returns a member's offers, newest first.
func (s *Service) ListMyOffers(ctx context.Context, userID int64) ([]Offer, error) {
	return s.repo.ListOffersByUser(ctx, userID)
}

// ListPendingOffers returns the moderator queue, oldest first.
func (s *Service) ListPendingOffers(ctx context.Context) ([]Offer, error) {
	return s.repo.ListPendingOffers(ctx)
}

// CreateRequest validates and records a seeker's referral request, then best-effort pings
// the company's approved referrers. It enforces contact presence, the CV kind/id invariant,
// company eligibility, and the rolling-24h cap before writing; a duplicate active request
// returns ErrAlreadyRequested.
func (s *Service) CreateRequest(ctx context.Context, in RequestInput) (Request, error) {
	if err := validateContact(in); err != nil {
		return Request{}, err
	}
	if err := validateCVChoice(in); err != nil {
		return Request{}, err
	}
	// The attached CV must be one the seeker actually has. A built cv_id comes from the
	// client, so its FK guarantees existence, not ownership — a foreign cv_id is treated as
	// an invalid choice (not a distinct error) so the response never leaks that it exists.
	// An original attachment requires a stored résumé, else it would serve nothing.
	switch in.CVKind {
	case CVBuilt:
		owned, err := s.repo.CVBelongsToUser(ctx, *in.CVID, in.SeekerUserID)
		if err != nil {
			return Request{}, err
		}
		if !owned {
			return Request{}, ErrInvalidCVChoice
		}
	case CVOriginal:
		has, err := s.repo.UserHasResume(ctx, in.SeekerUserID)
		if err != nil {
			return Request{}, err
		}
		if !has {
			return Request{}, ErrNoResume
		}
	}
	eligible, err := s.repo.CompanyHasApprovedReferrer(ctx, in.CompanySlug)
	if err != nil {
		return Request{}, err
	}
	if !eligible {
		return Request{}, ErrCompanyNotEligible
	}
	n, err := s.repo.CountRequestsSince(ctx, in.SeekerUserID, s.now().Add(-24*time.Hour))
	if err != nil {
		return Request{}, err
	}
	if int(n) >= s.dailyCap {
		return Request{}, ErrDailyCapReached
	}
	req, err := s.repo.CreateRequest(ctx, in)
	if err != nil {
		return Request{}, err
	}
	s.notifyReferrers(ctx, in.CompanySlug)
	return req, nil
}

// ResolveRequest marks a sent request contacted or declined on behalf of a referrer, after
// verifying they are an approved referrer of the request's company. A missing request is
// ErrRequestNotFound; a request already resolved by another referrer is ErrRequestNotOpen.
func (s *Service) ResolveRequest(ctx context.Context, requestID, referrerID int64, contacted bool) (Request, error) {
	if _, err := s.authorizedRequest(ctx, requestID, referrerID); err != nil {
		return Request{}, err
	}
	status := RequestContacted
	if !contacted {
		status = RequestDeclined
	}
	return s.repo.ResolveRequest(ctx, requestID, referrerID, status)
}

// AuthorizeCVAccess returns the request when referrerID is an approved referrer of its
// company, so the handler can serve the attached CV — the authorization gate that keeps CV
// access cabinet-only. ErrNotAuthorized otherwise; ErrRequestNotFound for a missing request.
func (s *Service) AuthorizeCVAccess(ctx context.Context, requestID, referrerID int64) (Request, error) {
	return s.authorizedRequest(ctx, requestID, referrerID)
}

// ListMyRequests returns a seeker's requests, newest first.
func (s *Service) ListMyRequests(ctx context.Context, seekerID int64) ([]Request, error) {
	return s.repo.ListRequestsBySeeker(ctx, seekerID)
}

// ListIncoming returns the open requests for every company the referrer is approved for.
func (s *Service) ListIncoming(ctx context.Context, referrerID int64) ([]Request, error) {
	return s.repo.ListIncomingRequests(ctx, referrerID)
}

// authorizedRequest fetches a request and verifies the caller is an approved referrer of its
// company, the shared gate behind acting on and viewing a request.
func (s *Service) authorizedRequest(ctx context.Context, requestID, referrerID int64) (Request, error) {
	req, ok, err := s.repo.GetRequest(ctx, requestID)
	if err != nil {
		return Request{}, err
	}
	if !ok {
		return Request{}, ErrRequestNotFound
	}
	approved, err := s.repo.ReferrerApprovedForCompany(ctx, referrerID, req.CompanySlug)
	if err != nil {
		return Request{}, err
	}
	if !approved {
		return Request{}, ErrNotAuthorized
	}
	return req, nil
}

// notifyReferrers pings every approved referrer of the company. Best-effort: resolve and
// send failures are logged, never returned, so a delivery hiccup never fails the request.
func (s *Service) notifyReferrers(ctx context.Context, companySlug string) {
	recipients, err := s.repo.ApprovedReferrerRecipients(ctx, companySlug)
	if err != nil {
		log.Printf("referral: resolve recipients for %s: %v", companySlug, err)
		return
	}
	for _, r := range recipients {
		if err := s.pinger.PingReferrer(ctx, r, s.cabinetURL); err != nil {
			log.Printf("referral: ping referrer %d: %v", r.UserID, err)
		}
	}
}

// validateContact requires at least one non-blank contact channel.
func validateContact(in RequestInput) error {
	if strings.TrimSpace(in.ContactTelegram) == "" && strings.TrimSpace(in.ContactEmail) == "" {
		return ErrNoContact
	}
	return nil
}

// validateCVChoice enforces the kind/id invariant the DB CHECK deliberately does not (so a
// tailored CV can be deleted later): original carries no cv_id, built carries one.
func validateCVChoice(in RequestInput) error {
	switch in.CVKind {
	case CVOriginal:
		if in.CVID != nil {
			return ErrInvalidCVChoice
		}
	case CVBuilt:
		if in.CVID == nil {
			return ErrInvalidCVChoice
		}
	default:
		return ErrInvalidCVChoice
	}
	return nil
}
