// Package accounts resolves external sign-in identities into local user accounts
// and provides password-based registration and login.
package accounts

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const minPasswordLen = 8

// maxPasswordLen is bcrypt's input ceiling: it silently truncates beyond 72 bytes,
// so a longer password would have its tail ignored. Reject rather than mislead.
const maxPasswordLen = 72

// dummyPasswordHash is a valid, throwaway bcrypt hash (cost = bcrypt.DefaultCost,
// matching production hashes) used only to keep Login constant-work: an unknown or
// passwordless account runs one bcrypt Check against it so its response time matches
// a real account's, closing the timing side-channel that would otherwise reveal which
// emails have password accounts (account enumeration). It never matches any password.
const dummyPasswordHash = "$2a$10$uAK6XQv2KNKj0KXGXHcgAe3fn70C1tZiCUeG4ZCkdGGJ60p2.py7S"

// User is the public representation of a local account. Role is the DB-stored
// authorization role ('user'/'moderator'/'admin'); it rides the wire shape so a client
// can gate moderator-only UI, while RequireRole still authorizes server-side.
// BetaTester is a separate rollout-group membership (independent of Role); it
// gates the in-app agent assistant and likewise rides the wire shape as a UI
// affordance.
type User struct {
	ID            int64
	Email         string
	Role          string
	BetaTester    bool
	EmailVerified bool
	// HasPassword is whether a password is set at all. False for an OAuth-only account,
	// which must go through the reset flow to set one. The hash itself never leaves the
	// database.
	HasPassword bool
	CreatedAt   *time.Time
	// Timezone is the account's IANA name (e.g. "Europe/Moscow"), nil until set.
	// Read by internal/deliverywindow (via notification_settings' joins) to
	// interpret a digest time or quiet-hours window in the account's own local
	// time; unset reads as UTC there.
	Timezone *string
	// Language is the account's preferred interface language, one of
	// supportedLanguages. Never empty — the column defaults to "en" — so unlike
	// Timezone there is no unset state to represent. First step toward interface
	// i18n: no UI translation ships yet, this just records the preference (see
	// freehire#1836).
	Language string
}

// PasswordHasher hashes and verifies passwords (bcrypt in production). Injected
// so accounts stays free of the auth/fiber dependency graph.
type PasswordHasher interface {
	Hash(plain string) (string, error)
	Check(hash, plain string) error
}

var (
	// ErrIdentityNotFound is returned by the repository when no identity row
	// matches the given (provider, providerUserID) pair.
	ErrIdentityNotFound = errors.New("accounts: identity not found")

	// ErrIdentityConflict is returned by the repository when a concurrent
	// callback already inserted the same identity row (unique violation).
	ErrIdentityConflict = errors.New("accounts: identity already exists")

	// ErrEmailRace is returned by the repository when link-or-create lost a race on
	// the user email index: a concurrent callback created the account under the same
	// verified email (but a different identity) first, so our identity was never
	// inserted. Distinct from ErrIdentityConflict because the recovery differs — the
	// service retries the link to attach our identity to the now-existing account,
	// rather than looking up an identity that does not exist.
	ErrEmailRace = errors.New("accounts: email create race")

	// ErrNoVerifiedEmail is returned by the service when the caller does not
	// supply a verified, non-empty email — a hard requirement before linking or
	// creating an account (unverified email is an account-takeover vector).
	ErrNoVerifiedEmail = errors.New("accounts: no verified email")

	ErrInvalidEmail       = errors.New("accounts: invalid email")
	ErrPasswordTooShort   = errors.New("accounts: password too short")
	ErrPasswordTooLong    = errors.New("accounts: password too long")
	ErrEmailTaken         = errors.New("accounts: email already registered")
	ErrInvalidCredentials = errors.New("accounts: invalid credentials")
	ErrUserNotFound       = errors.New("accounts: user not found")
)

// Repository is the persistence boundary for the accounts service.
// Implementations must be safe for concurrent use.
type Repository interface {
	// UserIDByIdentity returns the local user id for an external identity, or
	// ErrIdentityNotFound when none is linked yet.
	UserIDByIdentity(ctx context.Context, provider, providerUserID string) (int64, error)

	// LinkOrCreateByEmail links the identity to the account with this email
	// (creating a passwordless account when none exists), atomically. It returns
	// ErrIdentityConflict if the identity was inserted concurrently.
	LinkOrCreateByEmail(ctx context.Context, provider, providerUserID, email string) (int64, error)

	// CreateUser creates a new account with the given email and bcrypt password
	// hash. emailVerified is false for a password registration and true for an account
	// born from a provider-verified OAuth sign-in. timezone is the browser-detected
	// IANA zone captured at signup (nil when unknown/invalid — a password
	// registration is the only caller that ever supplies one). Returns
	// ErrEmailTaken on a unique-constraint violation.
	CreateUser(ctx context.Context, email, passwordHash string, emailVerified bool, timezone *string) (User, error)

	// UpdateTimezone sets (or clears, with nil) the account's IANA timezone name.
	// The caller has already validated it.
	UpdateTimezone(ctx context.Context, userID int64, timezone *string) (User, error)

	// UpdateLanguage sets the account's preferred interface language. The caller
	// has already validated it against supportedLanguages.
	UpdateLanguage(ctx context.Context, userID int64, language string) (User, error)

	// MarkEmailVerified records that control of the account's address was proven.
	MarkEmailVerified(ctx context.Context, userID int64) error

	// PasswordHash returns the account's stored bcrypt hash. hasPassword is false for a
	// passwordless (OAuth-only) account.
	PasswordHash(ctx context.Context, userID int64) (hash string, hasPassword bool, err error)

	// SetPassword replaces a known password and revokes every session, returning the
	// account's new session generation.
	SetPassword(ctx context.Context, userID int64, passwordHash string) (int32, error)

	// ResetPassword sets a password after a mailed code proved the address: it also
	// marks the account verified and revokes every session, returning the new
	// generation.
	ResetPassword(ctx context.Context, userID int64, passwordHash string) (int32, error)

	// UserByEmail looks up the user with the given (already-normalised) email.
	// Returns ErrUserNotFound when absent. hasPassword is true when the account
	// has a non-null password hash stored.
	UserByEmail(ctx context.Context, email string) (user User, passwordHash string, hasPassword bool, err error)

	// UserByID returns the user with the given id, or ErrUserNotFound when absent.
	UserByID(ctx context.Context, id int64) (User, error)

	// WithTx returns a Repository bound to the given transaction.
	WithTx(tx pgx.Tx) Repository
}

// Service resolves external OAuth identities and handles password auth for
// local user accounts. codes/mailer are optional (see WithCodes): without them the
// service still registers and authenticates, and the code-backed flows report the
// feature unavailable.
type Service struct {
	repo   Repository
	hasher PasswordHasher
	codes  CodeStore
	mailer CodeMailer

	// now is the clock the code TTL and resend cooldown read. A field so tests can
	// freeze time instead of sleeping.
	now func() time.Time
}

// New returns a Service backed by the given Repository and PasswordHasher.
func New(repo Repository, hasher PasswordHasher) *Service {
	return &Service{repo: repo, hasher: hasher, now: time.Now}
}

// ResolveOAuthAccount maps a provider identity to a local user id, following
// the identity-first, verified-email-gate, link-or-create, race-retry policy.
func (s *Service) ResolveOAuthAccount(
	ctx context.Context,
	provider, providerUserID, email string,
	emailVerified bool,
) (int64, error) {
	// 1. identity-first: cheapest and safest path.
	id, err := s.repo.UserIDByIdentity(ctx, provider, providerUserID)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, ErrIdentityNotFound) {
		return 0, err
	}

	// 2. verified-email gate (anti-takeover): never link or create without one.
	if !emailVerified || strings.TrimSpace(email) == "" {
		return 0, ErrNoVerifiedEmail
	}
	normalized := strings.ToLower(strings.TrimSpace(email))

	// 3. link existing account by email or create a new passwordless account.
	id, err = s.repo.LinkOrCreateByEmail(ctx, provider, providerUserID, normalized)
	switch {
	case err == nil:
		return id, nil
	case errors.Is(err, ErrIdentityConflict):
		// 4a. lost a race with a concurrent callback for the SAME identity — it now
		// exists; return whichever goroutine won.
		return s.repo.UserIDByIdentity(ctx, provider, providerUserID)
	case errors.Is(err, ErrEmailRace):
		// 4b. lost a race with a concurrent callback that created the account under
		// the same verified email but a DIFFERENT identity, so our identity was never
		// inserted. Retry the link now that the account exists, so our identity
		// attaches to it (a bare UserIDByIdentity retry would miss — it isn't there).
		id, err = s.repo.LinkOrCreateByEmail(ctx, provider, providerUserID, normalized)
		if errors.Is(err, ErrIdentityConflict) {
			// A further concurrent callback inserted our identity in the meantime.
			return s.repo.UserIDByIdentity(ctx, provider, providerUserID)
		}
		return id, err
	default:
		return 0, err
	}
}

// Register creates a new account with the given email and password. timezone is
// the browser-detected IANA zone (if any) — a missing or invalid value is
// silently dropped (nil) rather than failing the signup; the account simply
// starts without one, same as before this parameter existed.
// Returns ErrInvalidEmail for unparseable emails, ErrPasswordTooShort when the
// password is under minPasswordLen characters, and ErrEmailTaken when the
// normalised email is already registered.
func (s *Service) Register(ctx context.Context, email, password string, timezone *string) (User, error) {
	addr, err := normalizeEmail(email)
	if err != nil {
		return User{}, ErrInvalidEmail
	}
	if err := validatePassword(password); err != nil {
		return User{}, err
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return User{}, err
	}
	if timezone != nil && !validTimezone(*timezone) {
		timezone = nil
	}
	// A password registration proves nothing about the address, so the account starts
	// unverified — which is what stops it from being a silent OAuth merge target.
	user, err := s.repo.CreateUser(ctx, addr, hash, false, timezone)
	if err != nil {
		return User{}, err
	}
	s.sendVerificationOnRegister(ctx, user.ID, user.Email)
	return user, nil
}

// ErrInvalidTimezone is a timezone name Go's tzdata does not recognize (mapped to 400).
var ErrInvalidTimezone = errors.New("accounts: invalid timezone")

// validTimezone reports whether tz is a name time.LoadLocation accepts. Empty
// is rejected explicitly: time.LoadLocation("") returns UTC, nil (a Go stdlib
// quirk for "unset"), which would otherwise let an empty request body through
// as if it named a real zone.
func validTimezone(tz string) bool {
	if tz == "" {
		return false
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

// UpdateTimezone sets the caller's IANA timezone name. Unlike Register's silent
// drop, an explicit profile edit with a bad value is a real error the caller
// should see and correct.
func (s *Service) UpdateTimezone(ctx context.Context, userID int64, timezone string) (User, error) {
	if !validTimezone(timezone) {
		return User{}, ErrInvalidTimezone
	}
	return s.repo.UpdateTimezone(ctx, userID, &timezone)
}

// ErrInvalidLanguage is a language code outside supportedLanguages (mapped to 400).
var ErrInvalidLanguage = errors.New("accounts: invalid language")

// supportedLanguages is the curated set a profile can select — the same list the
// DB CHECK constraint on users.language enforces. No UI translation exists for
// any of these yet (freehire#1836); this only records the preference for later.
var supportedLanguages = map[string]bool{
	"en": true,
	"ru": true,
	"es": true,
	"pt": true,
	"de": true,
	"fr": true,
}

func validLanguage(lang string) bool {
	return supportedLanguages[lang]
}

// UpdateLanguage sets the caller's preferred interface language.
func (s *Service) UpdateLanguage(ctx context.Context, userID int64, language string) (User, error) {
	if !validLanguage(language) {
		return User{}, ErrInvalidLanguage
	}
	return s.repo.UpdateLanguage(ctx, userID, language)
}

// Login verifies the email/password pair and returns the matching user.
// Unknown email, passwordless accounts, and wrong passwords all yield
// ErrInvalidCredentials — never reveal which factor failed.
func (s *Service) Login(ctx context.Context, email, password string) (User, error) {
	addr, err := normalizeEmail(email)
	if err != nil {
		return User{}, ErrInvalidCredentials
	}
	user, hash, hasPassword, err := s.repo.UserByEmail(ctx, addr)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Spend one bcrypt Check on a dummy hash so an unknown account costs
			// the same as a real one — closing the timing side-channel.
			_ = s.hasher.Check(dummyPasswordHash, password)
			return User{}, ErrInvalidCredentials
		}
		return User{}, err
	}
	if !hasPassword {
		_ = s.hasher.Check(dummyPasswordHash, password)
		return User{}, ErrInvalidCredentials
	}
	if s.hasher.Check(hash, password) != nil {
		return User{}, ErrInvalidCredentials
	}
	// The caller just proved a password exists, but the lookup row does not carry the
	// flag — set it so login answers the same shape as /auth/me and a client cannot be
	// told the account is passwordless right after it signed in with a password.
	user.HasPassword = true
	return user, nil
}

// UserByID returns the user with the given id, delegating directly to the
// repository. Returns ErrUserNotFound when absent.
func (s *Service) UserByID(ctx context.Context, id int64) (User, error) {
	return s.repo.UserByID(ctx, id)
}

// normalizeEmail validates and lowercases an email address. It uses the same
// logic as the HTTP layer so the Go layer is the single normalizer.
func normalizeEmail(raw string) (string, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return strings.ToLower(addr.Address), nil
}
