package employer

import (
	"context"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/job/logodomain"
)

// codeIssuer is the slice of *accounts.Service this package needs: mint/mail and confirm a
// code for an arbitrary purpose, sharing the rate-limited, attempt-bounded store every other
// account code already uses. A local interface, not a concrete *accounts.Service dependency,
// so this package's own tests stay independent of accounts' internals.
type codeIssuer interface {
	IssueCode(ctx context.Context, userID int64, purpose, email string, send func(ctx context.Context, email, code string) error) error
	ConfirmCode(ctx context.Context, userID int64, purpose, code string) error
}

// ClaimMailer delivers the work-email verification code. A tiny, dedicated port —
// deliberately not an addition to accounts.CodeMailer, which stays free of any purpose this
// package alone uses. Exported (unlike codeIssuer) so a caller wiring Service can declare a
// properly nil-typed variable when no mail transport is configured: assigning a nil
// *emailnotify.AuthMailer through an UNEXPORTED interface parameter would still typecheck
// but smuggle in a non-nil interface holding a nil pointer, which Service.Claim's own nil
// check cannot see — see its comment.
type ClaimMailer interface {
	SendClaimVerificationCode(ctx context.Context, email, code string) error
}

// Service implements every employer use case: claim/verify/moderate/revoke (account.go) and
// create/edit/close a vacancy (job.go). One type, not two, because the job-authoring half
// depends on the account half's own ActiveAccount guard for every action it takes — keeping
// them in one package avoids a formal interface boundary between two halves nothing outside
// this package ever calls independently.
type Service struct {
	repo   Repository
	codes  codeIssuer
	mailer ClaimMailer

	jobs   JobRepository
	minter Minter
}

// New creates a Service backed by the given account Repository, code issuer, claim mailer,
// job repository, and Minter (moderation.Service satisfies Minter).
func New(repo Repository, codes codeIssuer, mailer ClaimMailer, jobs JobRepository, minter Minter) *Service {
	return &Service{repo: repo, codes: codes, mailer: mailer, jobs: jobs, minter: minter}
}

// Claim resolves companyName to a company slug (through the same alias registry and
// normalize.CompanySlug ingest uses), refuses a public-webmail work email, reserves the slug
// as a pending account, and mails a verification code. The unique-constraint conflicts a
// concurrent or repeat claim can hit are mapped by the Repository to ErrAlreadyHasAccount
// (this user already holds an account) or ErrCompanyAlreadyClaimed (the slug is taken).
func (s *Service) Claim(ctx context.Context, userID int64, companyName, workEmail string) (Account, error) {
	companyName = strings.TrimSpace(companyName)
	workEmail = strings.TrimSpace(workEmail)
	if companyName == "" {
		return Account{}, fmt.Errorf("%w: company name is required", ErrInvalid)
	}
	if workEmail == "" {
		return Account{}, fmt.Errorf("%w: work email is required", ErrInvalid)
	}
	if isPublicWebmailDomain(workEmail) {
		return Account{}, ErrPublicWebmailDomain
	}
	// s.mailer.SendClaimVerificationCode below is a method VALUE, evaluated eagerly as a
	// call argument — taking one from a nil interface panics before accounts.Service ever
	// gets to run its own "is mail configured" check, so this package needs its own guard.
	if s.mailer == nil {
		return Account{}, ErrMailUnavailable
	}

	candidate := normalize.CompanySlug(companyName)
	slug, err := s.repo.ResolveCanonicalSlug(ctx, candidate)
	if err != nil {
		return Account{}, err
	}

	// A company the catalogue already knows keeps its own stored display name (so this
	// employer's postings read consistently with any crawled ones already on the same
	// page); a brand-new company gets exactly what the claimant typed.
	name := companyName
	if existingName, _, found, err := s.repo.ExistingCompany(ctx, slug); err != nil {
		return Account{}, err
	} else if found && existingName != "" {
		name = existingName
	}

	acc, err := s.repo.InsertPending(ctx, userID, slug, name, workEmail)
	if err != nil {
		return Account{}, err
	}

	if err := s.codes.IssueCode(ctx, userID, accounts.PurposeVerifyWorkEmail, workEmail, s.mailer.SendClaimVerificationCode); err != nil {
		return Account{}, err
	}
	return acc, nil
}

// ConfirmClaim consumes the mailed code and, when the work email's domain matches the
// company's already-known website, activates the account immediately. An unknown or
// mismatched domain leaves the account pending — see ApproveClaim for how a moderator
// closes that gap, and employer-account's spec for why this method never seeds a website.
func (s *Service) ConfirmClaim(ctx context.Context, userID int64, code string) (Account, error) {
	acc, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return Account{}, err
	}
	if err := s.codes.ConfirmCode(ctx, userID, accounts.PurposeVerifyWorkEmail, code); err != nil {
		return Account{}, err
	}

	_, website, found, err := s.repo.ExistingCompany(ctx, acc.CompanySlug)
	if err != nil {
		return Account{}, err
	}
	knownDomain := logodomain.Domain(website)
	if !found || knownDomain == "" || knownDomain != emailDomain(acc.WorkEmail) {
		return acc, nil
	}
	return s.repo.Activate(ctx, userID)
}

// ListPendingClaims is the moderator review queue.
func (s *Service) ListPendingClaims(ctx context.Context) ([]Account, error) {
	return s.repo.ListPending(ctx)
}

// ApproveClaim activates a pending claim on a moderator's say-so and, when the company's
// website was still blank, seeds it from the claim's confirmed work-email domain — the human
// vouching for exactly the pairing the automatic domain check could not itself verify. It
// never overwrites an existing, non-blank website (SeedCompanyWebsite is a no-op then).
func (s *Service) ApproveClaim(ctx context.Context, userID int64) (Account, error) {
	acc, err := s.repo.Activate(ctx, userID)
	if err != nil {
		return Account{}, err
	}
	if err := s.repo.SeedCompanyWebsite(ctx, acc.CompanySlug, acc.CompanyName, emailDomain(acc.WorkEmail)); err != nil {
		return Account{}, err
	}
	return acc, nil
}

// RejectClaim removes a pending claim, freeing its company slug for a future claim.
func (s *Service) RejectClaim(ctx context.Context, userID int64) error {
	return s.repo.DeletePending(ctx, userID)
}

// RevokeAccount is the admin kill switch: it deactivates the account but keeps the row (and
// the slug reservation) — see migrations/0174 for why.
func (s *Service) RevokeAccount(ctx context.Context, userID int64) (Account, error) {
	return s.repo.Revoke(ctx, userID)
}

// UpdateCompanyProfile applies a verified employer's authoritative edit to their own
// company's curated profile. Gated by ActiveAccount like every other employer-facing
// capability.
func (s *Service) UpdateCompanyProfile(ctx context.Context, userID int64, patch CompanyProfilePatch) (Account, error) {
	acc, err := s.ActiveAccount(ctx, userID)
	if err != nil {
		return Account{}, err
	}
	if err := s.repo.UpdateCompanyProfile(ctx, acc.CompanySlug, patch); err != nil {
		return Account{}, err
	}
	return acc, nil
}

// ActiveAccount is the ownership guard every employer-facing capability calls: it resolves
// the caller's account and refuses (ErrNotActive) unless it is active. ErrNotFound propagates
// unchanged for a caller with no employer account at all.
func (s *Service) ActiveAccount(ctx context.Context, userID int64) (Account, error) {
	acc, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return Account{}, err
	}
	if !acc.Active() {
		return Account{}, ErrNotActive
	}
	return acc, nil
}

// MyAccount returns the caller's own account in whatever status it is in — pending, active,
// or revoked — unlike ActiveAccount, which refuses anything but active. It exists for the
// one read a caller needs BEFORE their account is active: "do I have a claim in flight, and
// what does it say" (the status page a pending claimant sees, and what the dashboard reads
// to decide whether to render the claim form or the dashboard at all). ErrNotFound is a
// caller with no employer account at all.
func (s *Service) MyAccount(ctx context.Context, userID int64) (Account, error) {
	return s.repo.GetByUserID(ctx, userID)
}
