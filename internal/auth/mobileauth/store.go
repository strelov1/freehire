// Package mobileauth owns the durable, verifier-bound hand-off used by native
// OAuth clients. Raw credentials are returned once and only their SHA-256 hashes
// are persisted.
package mobileauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	AttemptTTL = 10 * time.Minute
	CodeTTL    = time.Minute
)

var (
	ErrInvalid       = errors.New("mobile auth: invalid or expired credential")
	ErrVerifier      = errors.New("mobile auth: invalid verifier")
	ErrSession       = errors.New("mobile auth: session binding mismatch")
	ErrIdentity      = errors.New("mobile auth: reauthentication identity mismatch")
	verifierPattern  = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)
	challengePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
)

type Binding struct {
	UserID       int64
	TokenVersion int32
	SessionHash  []byte
}

type Attempt struct {
	ID                   uuid.UUID
	Provider             string
	Platform             string
	CallbackTarget       string
	Purpose              string
	CodeChallenge        string
	NonceChallenge       string
	AllowedAudiences     []string
	ExpectedUserID       *int64
	ExpectedTokenVersion *int32
	ExpectedSessionHash  []byte
	ExpiresAt            time.Time
}

type Exchange struct {
	UserID               int64
	Purpose              string
	ExpectedUserID       *int64
	ExpectedTokenVersion *int32
	ExpectedSessionHash  []byte
}

type Store struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool, now: time.Now} }

func ValidateChallenge(challenge, method string) error {
	if method != "S256" || !challengePattern.MatchString(challenge) {
		return ErrVerifier
	}
	return nil
}

func ValidateVerifier(verifier string) error {
	if !verifierPattern.MatchString(verifier) {
		return ErrVerifier
	}
	return nil
}

func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomCredential() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func digest(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func bindingValues(binding *Binding) (any, any, any) {
	if binding == nil {
		return nil, nil, nil
	}
	return binding.UserID, binding.TokenVersion, binding.SessionHash
}

func validatePurpose(purpose string, binding *Binding) error {
	if purpose != "sign_in" && purpose != "reauth" {
		return ErrInvalid
	}
	if (purpose == "reauth") != (binding != nil) {
		return ErrSession
	}
	if binding != nil && (binding.UserID <= 0 || len(binding.SessionHash) != sha256.Size) {
		return ErrSession
	}
	return nil
}

// CreateBrowserAttempt stores a v2 attempt and returns its raw browser state.
func (s *Store) CreateBrowserAttempt(ctx context.Context, provider, platform, callbackTarget, purpose, challenge string, binding *Binding) (Attempt, string, error) {
	validPlatform := platform == "ios" || platform == "android" || (platform == "web" && purpose == "reauth")
	if provider == "" || !validPlatform || callbackTarget == "" || (provider == "apple" && platform != "web") {
		return Attempt{}, "", ErrInvalid
	}
	if err := validatePurpose(purpose, binding); err != nil {
		return Attempt{}, "", err
	}
	if err := ValidateChallenge(challenge, "S256"); err != nil {
		return Attempt{}, "", err
	}
	state, err := randomCredential()
	if err != nil {
		return Attempt{}, "", err
	}
	expires := s.now().Add(AttemptTTL)
	uid, version, sessionHash := bindingValues(binding)
	var a Attempt
	err = s.pool.QueryRow(ctx, `
		INSERT INTO oauth_auth_attempts
			(state_hash, provider, platform, callback_target, purpose,
			 code_challenge, code_challenge_method, expected_user_id,
			 expected_token_version, expected_session_hash, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,'S256',$7,$8,$9,$10)
		RETURNING id, provider, platform, callback_target, purpose,
		          code_challenge, expires_at`,
		digest(state), provider, platform, callbackTarget, purpose, challenge, uid, version, sessionHash, expires,
	).Scan(&a.ID, &a.Provider, &a.Platform, &a.CallbackTarget, &a.Purpose, &a.CodeChallenge, &a.ExpiresAt)
	return a, state, err
}

// ConsumeBrowserAttempt atomically validates and burns state. A callback can
// therefore resolve at most one account across any number of replicas.
func (s *Store) ConsumeBrowserAttempt(ctx context.Context, state, provider string) (Attempt, error) {
	if state == "" || provider == "" {
		return Attempt{}, ErrInvalid
	}
	var a Attempt
	var uid *int64
	var version *int32
	err := s.pool.QueryRow(ctx, `
		UPDATE oauth_auth_attempts
		SET consumed_at = now()
		WHERE state_hash = $1 AND provider = $2 AND protocol_version = 2
		  AND consumed_at IS NULL AND expires_at > now() AND callback_target <> 'native_apple'
		RETURNING id, provider, platform, callback_target, purpose,
		          code_challenge, expected_user_id, expected_token_version,
		          expected_session_hash, expires_at`,
		digest(state), provider,
	).Scan(&a.ID, &a.Provider, &a.Platform, &a.CallbackTarget, &a.Purpose,
		&a.CodeChallenge, &uid, &version, &a.ExpectedSessionHash, &a.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attempt{}, ErrInvalid
	}
	a.ExpectedUserID, a.ExpectedTokenVersion = uid, version
	return a, err
}

// CreateAppleAttempt stores the nonce challenge to which the signed Apple token
// must later bind.
func (s *Store) CreateAppleAttempt(ctx context.Context, nonceChallenge, purpose string, audiences []string, binding *Binding) (Attempt, error) {
	if !challengePattern.MatchString(nonceChallenge) || len(audiences) == 0 {
		return Attempt{}, ErrVerifier
	}
	if err := validatePurpose(purpose, binding); err != nil {
		return Attempt{}, err
	}
	expires := s.now().Add(AttemptTTL)
	uid, version, sessionHash := bindingValues(binding)
	var a Attempt
	err := s.pool.QueryRow(ctx, `
		INSERT INTO oauth_auth_attempts
			(provider, platform, callback_target, purpose, nonce_challenge, allowed_audiences,
			 expected_user_id, expected_token_version, expected_session_hash, expires_at)
		VALUES ('apple','ios','native_apple',$1,$2,$3,$4,$5,$6,$7)
		RETURNING id, provider, platform, callback_target, purpose,
		          nonce_challenge, allowed_audiences, expected_user_id,
		          expected_token_version, expected_session_hash, expires_at`,
		purpose, nonceChallenge, audiences, uid, version, sessionHash, expires,
	).Scan(&a.ID, &a.Provider, &a.Platform, &a.CallbackTarget, &a.Purpose,
		&a.NonceChallenge, &a.AllowedAudiences, &a.ExpectedUserID,
		&a.ExpectedTokenVersion, &a.ExpectedSessionHash, &a.ExpiresAt)
	return a, err
}

// ConsumeAppleAttempt burns the attempt before any remote exchange, preventing
// replay even when Apple returns a retryable error.
func (s *Store) ConsumeAppleAttempt(ctx context.Context, id uuid.UUID) (Attempt, error) {
	var a Attempt
	err := s.pool.QueryRow(ctx, `
		UPDATE oauth_auth_attempts SET consumed_at = now()
		WHERE id = $1 AND provider = 'apple' AND consumed_at IS NULL AND expires_at > now()
		RETURNING id, provider, platform, callback_target, purpose,
		          nonce_challenge, allowed_audiences, expected_user_id,
		          expected_token_version, expected_session_hash, expires_at`, id,
	).Scan(&a.ID, &a.Provider, &a.Platform, &a.CallbackTarget, &a.Purpose,
		&a.NonceChallenge, &a.AllowedAudiences, &a.ExpectedUserID,
		&a.ExpectedTokenVersion, &a.ExpectedSessionHash, &a.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attempt{}, ErrInvalid
	}
	return a, err
}

func (s *Store) MintCode(ctx context.Context, a Attempt, userID int64) (string, error) {
	code, err := randomCredential()
	if err != nil {
		return "", err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO oauth_exchange_codes
			(code_hash, user_id, source_attempt_id, code_challenge,
			 code_challenge_method, purpose, expected_user_id,
			 expected_token_version, expected_session_hash, expires_at)
		VALUES ($1,$2,$3,$4,'S256',$5,$6,$7,$8,$9)`,
		digest(code), userID, a.ID, a.CodeChallenge, a.Purpose,
		a.ExpectedUserID, a.ExpectedTokenVersion, a.ExpectedSessionHash, s.now().Add(CodeTTL))
	return code, err
}

// ConsumeCode always burns a live code, including on verifier mismatch.
func (s *Store) ConsumeCode(ctx context.Context, code, verifier string) (Exchange, error) {
	if code == "" {
		return Exchange{}, ErrInvalid
	}
	var e Exchange
	var storedChallenge string
	err := s.pool.QueryRow(ctx, `
		UPDATE oauth_exchange_codes SET consumed_at = now()
		WHERE code_hash = $1 AND consumed_at IS NULL AND expires_at > now()
		RETURNING user_id, purpose, code_challenge,
		          expected_user_id, expected_token_version,
		          expected_session_hash`, digest(code),
	).Scan(&e.UserID, &e.Purpose, &storedChallenge,
		&e.ExpectedUserID, &e.ExpectedTokenVersion, &e.ExpectedSessionHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Exchange{}, ErrInvalid
	}
	if err != nil {
		return Exchange{}, err
	}
	if err := ValidateVerifier(verifier); err != nil {
		return Exchange{}, ErrVerifier
	}
	actual := Challenge(verifier)
	if subtle.ConstantTimeCompare([]byte(actual), []byte(storedChallenge)) != 1 {
		return Exchange{}, ErrVerifier
	}
	if e.Purpose == "reauth" && (e.ExpectedUserID == nil || *e.ExpectedUserID != e.UserID || len(e.ExpectedSessionHash) != sha256.Size) {
		return Exchange{}, ErrIdentity
	}
	return e, nil
}

func (s *Store) VerifyBinding(ctx context.Context, userID int64, expectedVersion int32) error {
	var version int32
	if err := s.pool.QueryRow(ctx, `SELECT token_version FROM users WHERE id=$1`, userID).Scan(&version); err != nil {
		return err
	}
	if version != expectedVersion {
		return ErrSession
	}
	return nil
}

func (s *Store) IdentityOwner(ctx context.Context, provider, providerUserID string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		SELECT user_id FROM user_identities
		WHERE provider=$1 AND provider_user_id=$2 AND status='active'`, provider, providerUserID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrIdentity
	}
	return id, err
}

func (a Attempt) CheckExpectedUser(userID int64) error {
	if a.Purpose == "reauth" && (a.ExpectedUserID == nil || *a.ExpectedUserID != userID) {
		return fmt.Errorf("%w", ErrIdentity)
	}
	return nil
}

// Cleanup deletes bounded batches of expired ephemeral credentials.
func (s *Store) Cleanup(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 || limit > 10_000 {
		limit = 1000
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var total int64
	queries := []string{
		`DELETE FROM oauth_exchange_codes WHERE ctid IN
			(SELECT ctid FROM oauth_exchange_codes WHERE expires_at < now() ORDER BY expires_at LIMIT $1)`,
		`DELETE FROM oauth_auth_attempts a WHERE a.id IN
			(SELECT candidate.id FROM oauth_auth_attempts candidate
			 WHERE candidate.expires_at < now()
			   AND NOT EXISTS (
			       SELECT 1 FROM oauth_exchange_codes code
			       WHERE code.source_attempt_id=candidate.id
			         AND code.consumed_at IS NULL AND code.expires_at > now())
			 ORDER BY candidate.expires_at LIMIT $1)`,
		`DELETE FROM recent_auth_proofs WHERE ctid IN
			(SELECT ctid FROM recent_auth_proofs WHERE expires_at < now() ORDER BY expires_at LIMIT $1)`,
	}
	for _, q := range queries {
		tag, err := tx.Exec(ctx, q, limit)
		if err != nil {
			return 0, err
		}
		total += tag.RowsAffected()
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return total, nil
}
