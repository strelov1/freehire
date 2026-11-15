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

// maxPasswordLen is bcrypt's input ceiling; it silently truncates beyond 72 bytes.
const maxPasswordLen = 72

// dummyPasswordHash is a throwaway bcrypt hash at production cost. Login checks it for
// unknown or passwordless accounts so timing does not reveal which emails exist.
const dummyPasswordHash = "$2a$10$uAK6XQv2KNKj0KXGXHcgAe3fn70C1tZiCUeG4ZCkdGGJ60p2.py7S"

// User is the public representation of a local account. Role ('user'/'moderator'/'admin')
// and BetaTester ride the wire shape only so the client can gate UI; the server
// authorizes on its own (RequireRole).
type User struct {
	ID            int64
	Email         string
	Role          string
	BetaTester    bool
	EmailVerified bool
	// HasPassword is false for an OAuth-only account, which sets one through the reset flow.
	HasPassword bool
	CreatedAt   *time.Time
	// Timezone is the account's IANA name (e.g. "Europe/Moscow"), nil until set (read as UTC).
	Timezone *string
	// Language is one of supportedLanguages; never empty (the column defaults to "en").
	Language string
	// OnboardingCompletedAt is the whole onboarding gate: nil routes the user into the
	// wizard, non-nil never does. Not inferred from whether a CV exists.
	OnboardingCompletedAt *time.Time
	// Banned is set only by the by-email lookup (Login, password reset); a banned account
	// neither signs in nor is mailed a code.
	Banned bool
}

// PasswordHasher hashes and verifies passwords (bcrypt in production). Injected
// so accounts stays free of the auth/fiber dependency graph.
type PasswordHasher interface {
	Hash(plain string) (string, error)
	Check(hash, plain string) error
}

// ErrAccountSuspended means the credentials were right but the account is banned. Only
// shown to someone who proved the credential.
var ErrAccountSuspended = errors.New("accounts: account suspended")

var (
	// ErrIdentityNotFound means no identity row matches (provider, providerUserID).
	ErrIdentityNotFound = errors.New("accounts: identity not found")

	// ErrIdentityConflict means a concurrent callback already inserted the same identity.
	ErrIdentityConflict = errors.New("accounts: identity already exists")

	// ErrEmailRace means link-or-create lost a race on the email index: a concurrent
	// callback created the account under the same email with a different identity. The
	// service retries the link, since our identity was never inserted.
	ErrEmailRace = errors.New("accounts: email create race")

	// ErrNoVerifiedEmail means no verified, non-empty email was supplied; linking on an
	// unverified email is an account-takeover vector.
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
	// ErrIdentityNotFound.
	UserIDByIdentity(ctx context.Context, provider, providerUserID string) (int64, error)

	// LinkOrCreateByEmail links the identity to the account with this email
	// (creating a passwordless account when none exists), atomically. It returns
	// ErrIdentityConflict if the identity was inserted concurrently.
	LinkOrCreateByEmail(ctx context.Context, provider, providerUserID, email string) (int64, error)

	// CreateUser creates an account. emailVerified is false for a password registration,
	// true for a provider-verified OAuth sign-in. timezone is the browser-detected zone
	// (nil when unknown). Returns ErrEmailTaken on a unique violation.
	CreateUser(ctx context.Context, email, passwordHash string, emailVerified bool, timezone *string) (User, error)

	// UpdateTimezone sets (or clears, with nil) the account's IANA timezone name.
	// With onlyIfUnset, preserves an existing zone atomically. The caller validates it.
	UpdateTimezone(ctx context.Context, userID int64, timezone *string, onlyIfUnset bool) (User, error)

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

	// ChangeEmail moves the account to a new address a mailed code proved: it marks the
	// address verified, revokes every session, destroys the codes mailed to the old
	// address, and returns the new generation. ErrEmailTaken when another account holds
	// the address.
	ChangeEmail(ctx context.Context, userID int64, email string) (int32, error)

	// DeleteOAuthGrantsByUser wipes every MCP OAuth grant the account holds. Called on
	// password reset; see ResetPassword for why.
	DeleteOAuthGrantsByUser(ctx context.Context, userID int64) error

	// UserByEmail looks up a user by normalised email, or ErrUserNotFound.
	UserByEmail(ctx context.Context, email string) (user User, passwordHash string, hasPassword bool, err error)

	// UserByID returns the user with the given id, or ErrUserNotFound when absent.
	UserByID(ctx context.Context, id int64) (User, error)

	// UsernameByUser returns the account's username (ok=false if none) and the time of its
	// last explicit SetUsername (nil for a lazily allocated default).
	UsernameByUser(ctx context.Context, userID int64) (name string, updatedAt *time.Time, ok bool, err error)

	// SetUsernameIfAbsent claims username only if the account has none, without touching
	// username_updated_at. ErrUsernameTaken covers both a held name and a concurrent
	// winner; re-read UsernameByUser to tell which.
	SetUsernameIfAbsent(ctx context.Context, userID int64, username string) error

	// SetUsername replaces the username and records the change time. Returns
	// ErrUsernameTaken if another account holds it.
	SetUsername(ctx context.Context, userID int64, username string) error

	// UsernameTaken reports whether any account already holds username.
	UsernameTaken(ctx context.Context, username string) (bool, error)

	// WithTx returns a Repository bound to the given transaction.
	WithTx(tx pgx.Tx) Repository
}

// Service resolves OAuth identities and handles password auth. codes/mailer are
// optional (see WithCodes); without them the code-backed flows report unavailable.
type Service struct {
	repo   Repository
	hasher PasswordHasher
	codes  CodeStore
	mailer CodeMailer

	// now is the clock for the code TTL and resend cooldown; tests freeze it.
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
	// Identity first.
	id, err := s.repo.UserIDByIdentity(ctx, provider, providerUserID)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, ErrIdentityNotFound) {
		return 0, err
	}

	// Never link or create without a verified email (anti-takeover).
	if !emailVerified || strings.TrimSpace(email) == "" {
		return 0, ErrNoVerifiedEmail
	}
	normalized := strings.ToLower(strings.TrimSpace(email))

	// Link by email, or create a passwordless account.
	id, err = s.repo.LinkOrCreateByEmail(ctx, provider, providerUserID, normalized)
	switch {
	case err == nil:
		return id, nil
	case errors.Is(err, ErrIdentityConflict):
		// Lost a race for the same identity; it exists now.
		return s.repo.UserIDByIdentity(ctx, provider, providerUserID)
	case errors.Is(err, ErrEmailRace):
		// Lost a race for the same email under a different identity: retry the link so ours
		// attaches (a bare UserIDByIdentity would miss).
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

// Register creates an account. A missing or invalid timezone is dropped rather than
// failing the signup. Errors: ErrInvalidEmail, ErrPasswordTooShort, ErrEmailTaken.
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
	// Unverified: stops the account being a silent OAuth merge target.
	user, err := s.repo.CreateUser(ctx, addr, hash, false, timezone)
	if err != nil {
		return User{}, err
	}
	s.sendVerificationOnRegister(ctx, user.ID, user.Email)
	return user, nil
}

// ErrInvalidTimezone is a timezone name Go's tzdata does not recognize (mapped to 400).
var ErrInvalidTimezone = errors.New("accounts: invalid timezone")

// validTimezone reports whether tz is a name time.LoadLocation accepts. Empty is
// rejected explicitly because LoadLocation("") returns UTC without error.
func validTimezone(tz string) bool {
	if tz == "" {
		return false
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

// UpdateTimezone sets the caller's IANA timezone. A bad value is an error here, unlike
// Register. onlyIfUnset keeps an existing preference from automatic adoption.
func (s *Service) UpdateTimezone(ctx context.Context, userID int64, timezone string, onlyIfUnset bool) (User, error) {
	if !validTimezone(timezone) {
		return User{}, ErrInvalidTimezone
	}
	return s.repo.UpdateTimezone(ctx, userID, &timezone, onlyIfUnset)
}

// ErrInvalidLanguage is a language code outside supportedLanguages (mapped to 400).
var ErrInvalidLanguage = errors.New("accounts: invalid language")

// supportedLanguages mirrors the DB CHECK constraint on users.language.
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

// Login verifies the email/password pair. Unknown email, passwordless account and wrong
// password all yield ErrInvalidCredentials; never reveal which failed.
func (s *Service) Login(ctx context.Context, email, password string) (User, error) {
	addr, err := normalizeEmail(email)
	if err != nil {
		return User{}, ErrInvalidCredentials
	}
	user, hash, hasPassword, err := s.repo.UserByEmail(ctx, addr)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Dummy check keeps an unknown account's timing equal to a real one's.
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
	// The lookup row lacks the flag; set it so login matches the /auth/me shape.
	user.HasPassword = true
	if user.Banned {
		return User{}, ErrAccountSuspended
	}
	return user, nil
}

// UserByID returns the user with the given id, or ErrUserNotFound.
func (s *Service) UserByID(ctx context.Context, id int64) (User, error) {
	return s.repo.UserByID(ctx, id)
}

// normalizeEmail validates and lowercases an email address.
func normalizeEmail(raw string) (string, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return strings.ToLower(addr.Address), nil
}
