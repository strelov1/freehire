// Package employer implements the verified-employer account: claiming a company, proving a
// work-email tie to it, and — once active — publishing, editing, and closing that company's
// own vacancies. It also owns the moderator review queue for a claim that cannot auto-verify.
//
// It sits beside internal/ingest/moderation and internal/ingest/submission, the other two
// manual-intake write paths, and reuses moderation.Service.Create for the actual job insert
// (see job.go) — but never moderation.Service.Update, which is scoped to "any
// manually-authored job" rather than to a specific actor. See
// openspec/changes/add-employer-company-accounts for the full design.
package employer

import (
	"context"
	"errors"
	"time"
)

// Account status values. company_accounts.status is CHECK-constrained to exactly these
// three (migration 0174).
const (
	StatusPending = "pending"
	StatusActive  = "active"
	StatusRevoked = "revoked"
)

// Sentinel errors, mapped to HTTP statuses by the handler.
var (
	// ErrInvalid wraps every validation failure. Its text is user-facing: the handler
	// surfaces the wrapped message in the 400 body, so it carries no package prefix —
	// mirrors moderation.ErrInvalid.
	ErrInvalid = errors.New("invalid request")
	// ErrNotFound is a user with no employer account at all.
	ErrNotFound = errors.New("employer: no account for this user")
	// ErrAlreadyHasAccount is a claim attempt by a user who already holds one, in any status.
	ErrAlreadyHasAccount = errors.New("employer: this user already has an employer account")
	// ErrCompanyAlreadyClaimed is a claim attempt on a company slug another account already
	// holds — the UNIQUE(company_slug) conflict, mapped to a clean refusal.
	ErrCompanyAlreadyClaimed = errors.New("employer: this company is already claimed")
	// ErrPublicWebmailDomain is a claim whose work email is at a known public/free provider.
	ErrPublicWebmailDomain = errors.New("employer: work email must be at a company domain, not a public email provider")
	// ErrNotActive is any employer-facing action attempted by a pending or revoked account.
	ErrNotActive = errors.New("employer: account is not active")
	// ErrClaimNotPending is an approve/reject of a claim that is no longer pending (already
	// active, revoked, or never existed).
	ErrClaimNotPending = errors.New("employer: no pending claim for this user")
	// ErrMailUnavailable reports that no claim-mail transport is configured (see New's
	// mailer parameter), so no code can be delivered. Checked before Claim ever evaluates
	// a method value on it — mirrors accounts.ErrMailUnavailable's role, but is this
	// package's own sentinel since a caller mapping errors should never need to import
	// identity/accounts just to recognize this one.
	ErrMailUnavailable = errors.New("employer: claim-mail delivery is not configured")
)

// Account is one user's claim on one company, decoupled from the generated db row.
type Account struct {
	UserID      int64
	CompanySlug string
	CompanyName string
	WorkEmail   string
	Status      string
	VerifiedAt  *time.Time
	CreatedAt   time.Time
}

// Active reports whether this account may act on its company — publish/edit/close a
// vacancy, or edit the company's curated profile.
func (a Account) Active() bool { return a.Status == StatusActive }

// Repository is the persistence contract for company_accounts and the slug/company-identity
// lookups a claim needs, expressed in package domain types rather than generated db rows.
type Repository interface {
	// InsertPending reserves companySlug for userID (status='pending'). ErrCompanyAlreadyClaimed
	// or ErrAlreadyHasAccount on the corresponding unique-constraint conflict.
	InsertPending(ctx context.Context, userID int64, companySlug, companyName, workEmail string) (Account, error)
	// GetByUserID is the one account a user may hold, whatever status it is in. ErrNotFound
	// when the user holds none.
	GetByUserID(ctx context.Context, userID int64) (Account, error)
	// ListPending is the moderator review queue, oldest first.
	ListPending(ctx context.Context) ([]Account, error)
	// Activate flips a pending account to active. ErrClaimNotPending when the account is
	// missing or not pending.
	Activate(ctx context.Context, userID int64) (Account, error)
	// Revoke flips a pending or active account to revoked, keeping the row (and the slug
	// reservation) — see migrations/0174. ErrNotFound when the user holds no account at all.
	Revoke(ctx context.Context, userID int64) (Account, error)
	// DeletePending removes a still-pending claim, freeing its company slug. ErrClaimNotPending
	// when the account is missing or not pending.
	DeletePending(ctx context.Context, userID int64) error

	// ResolveCanonicalSlug returns the canonical slug for a candidate slug: candidate itself
	// when it names no retired alias, or the alias's canonical_slug when it does (see
	// company_slug_aliases / docs/agents/company-identity.md). A claim must resolve through
	// this so it lands on the same company identity ingest would.
	ResolveCanonicalSlug(ctx context.Context, candidate string) (string, error)
	// ExistingCompany reads a company's current display name and website (company_info's
	// "website" key, "" if unset), found=false when no companies row exists for slug yet.
	ExistingCompany(ctx context.Context, slug string) (name, website string, found bool, err error)
	// SeedCompanyWebsite fills a company's website when it is not already set: inserting a
	// new is_reference row (mirroring cmd/import-yc's pattern for a company with no jobs
	// yet) or filling company_info's blank "website" key on an existing row. A no-op when
	// the company already has a non-blank website — this never overwrites.
	SeedCompanyWebsite(ctx context.Context, slug, name, website string) error
	// UpdateCompanyProfile applies a verified employer's authoritative edit to their own
	// company's curated profile (see CompanyProfilePatch).
	UpdateCompanyProfile(ctx context.Context, slug string, patch CompanyProfilePatch) error
}

// CompanyProfilePatch is a partial, authoritative edit to the account's own company's
// curated profile fields — never the job-derived ones (company_types/company_sizes, see
// design.md's Context). A nil field is left unchanged. Industries replaces the whole
// curated set when non-nil (SetCompanyIndustries' own replace semantics — an empty,
// non-nil slice clears it).
type CompanyProfilePatch struct {
	Tagline       *string
	Description   *string
	Website       *string
	Industries    []string
	YearFounded   *int
	EmployeeCount *int
	HqCountry     *string
	Subindustry   *string
}
