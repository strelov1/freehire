package linkedinauth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// PostgresStore is the Store backed by the generated queries. Expiry rules live in renew.go.
type PostgresStore struct {
	q *db.Queries
}

// NewPostgresStore wraps the generated query set.
func NewPostgresStore(q *db.Queries) *PostgresStore {
	return &PostgresStore{q: q}
}

func (s *PostgresStore) Load(ctx context.Context) (Token, bool, error) {
	row, err := s.q.GetSocialToken(ctx, Channel)
	if err != nil {
		// No row means never signed in (or disabled), not a failure.
		if errors.Is(err, pgx.ErrNoRows) {
			return Token{}, false, nil
		}
		return Token{}, false, err
	}
	return Token{
		AccessToken:      row.AccessToken,
		ExpiresAt:        row.AccessExpiresAt.Time,
		RefreshToken:     row.RefreshToken.String,
		RefreshExpiresAt: row.RefreshExpiresAt.Time,
		Scope:            row.Scope,
	}, true, nil
}

func (s *PostgresStore) Save(ctx context.Context, t Token) error {
	return s.q.StoreSocialToken(ctx, db.StoreSocialTokenParams{
		Channel:          Channel,
		AccessToken:      t.AccessToken,
		AccessExpiresAt:  pgTime(t.ExpiresAt),
		RefreshToken:     pgText(t.RefreshToken),
		RefreshExpiresAt: pgTime(t.RefreshExpiresAt),
		Scope:            t.Scope,
	})
}

func (s *PostgresStore) Renew(ctx context.Context, previous string, t Token) (bool, error) {
	n, err := s.q.RenewSocialToken(ctx, db.RenewSocialTokenParams{
		Channel:             Channel,
		AccessToken:         t.AccessToken,
		AccessExpiresAt:     pgTime(t.ExpiresAt),
		RefreshToken:        pgText(t.RefreshToken),
		RefreshExpiresAt:    pgTime(t.RefreshExpiresAt),
		PreviousAccessToken: previous,
	})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// AccessToken returns the stored token if usable now, and says why not otherwise.
// socialdigest's LinkedIn publisher calls it on every publish.
//
// Expiry is checked here rather than left to LinkedIn's 401, since an expired credential
// needs a sign-in while a 401 on a live token means LinkedIn revoked us. The token is read
// per publish so a fresh renewal is picked up without a restart.
func (s *PostgresStore) AccessToken(ctx context.Context) (string, error) {
	tok, ok, err := s.Load(ctx)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrNoCredential
	}
	if !tok.ExpiresAt.IsZero() && !time.Now().UTC().Before(tok.ExpiresAt) {
		return "", fmt.Errorf("%w: it expired on %s; run cmd/linkedin-auth to sign in again",
			ErrCredentialExpired, tok.ExpiresAt.Format(time.RFC3339))
	}
	return tok.AccessToken, nil
}

// The two ways a publish can fail before it is attempted. Exported so a caller can tell
// "never turned on" (a clean skip) from "broken" (must exit non-zero).
var (
	ErrNoCredential      = errors.New("linkedinauth: no stored credential")
	ErrCredentialExpired = errors.New("linkedinauth: stored credential has expired")
)

// pgTime maps a zero time to SQL NULL; year 1 would make an absent refresh token read as
// long expired.
func pgTime(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

// pgText maps an empty string to SQL NULL, for the same reason.
func pgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}
