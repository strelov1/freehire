// Package recentauth implements a separate, short-lived proof of recent
// credential control. It never accepts API-key or bearer authentication.
package recentauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	CookieName = "hire_recent_auth"
	DefaultTTL = 10 * time.Minute
)

var ErrRequired = errors.New("recent authentication required")

type Store struct {
	pool *pgxpool.Pool
	ttl  time.Duration
	now  func() time.Time
}

func NewStore(pool *pgxpool.Pool, ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Store{pool: pool, ttl: ttl, now: time.Now}
}

func hash(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func (s *Store) Issue(ctx context.Context, userID int64, tokenVersion int32, sessionHash []byte) (string, time.Time, error) {
	if len(sessionHash) != sha256.Size {
		return "", time.Time{}, ErrRequired
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	raw := base64.RawURLEncoding.EncodeToString(b)
	expires := s.now().Add(s.ttl)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO recent_auth_proofs (proof_hash,user_id,token_version,session_hash,expires_at)
		VALUES ($1,$2,$3,$4,$5)`, hash(raw), userID, tokenVersion, sessionHash, expires)
	return raw, expires, err
}

func (s *Store) Validate(ctx context.Context, raw string, userID int64, tokenVersion int32, sessionHash []byte) error {
	if raw == "" || len(sessionHash) != sha256.Size {
		return ErrRequired
	}
	var stored []byte
	err := s.pool.QueryRow(ctx, `
		SELECT proof_hash FROM recent_auth_proofs
		WHERE proof_hash=$1 AND user_id=$2 AND token_version=$3 AND session_hash=$4
		  AND consumed_at IS NULL AND expires_at > now()`, hash(raw), userID, tokenVersion, sessionHash).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRequired
	}
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(stored, hash(raw)) != 1 {
		return ErrRequired
	}
	return nil
}

func (s *Store) Consume(ctx context.Context, raw string, userID int64, tokenVersion int32, sessionHash []byte) error {
	if raw == "" || len(sessionHash) != sha256.Size {
		return ErrRequired
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE recent_auth_proofs SET consumed_at=now()
		WHERE proof_hash=$1 AND user_id=$2 AND token_version=$3 AND session_hash=$4
		  AND consumed_at IS NULL AND expires_at > now()`, hash(raw), userID, tokenVersion, sessionHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrRequired
	}
	return nil
}

func (s *Store) Revoke(ctx context.Context, raw string) error {
	if raw == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `UPDATE recent_auth_proofs SET consumed_at=now() WHERE proof_hash=$1 AND consumed_at IS NULL`, hash(raw))
	return err
}

func SetCookie(c *fiber.Ctx, raw string, expires time.Time, secure bool, domain string) {
	c.Cookie(&fiber.Cookie{Name: CookieName, Value: raw, Path: "/", Domain: domain,
		Expires: expires, HTTPOnly: true, Secure: secure, SameSite: fiber.CookieSameSiteLaxMode})
}

func ClearCookie(c *fiber.Ctx, secure bool, domain string) {
	SetCookie(c, "", time.Now().Add(-time.Hour), secure, domain)
}
