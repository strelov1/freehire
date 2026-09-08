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

// PostgresStore is the Store backed by the generated queries. It holds no rules about expiry —
// those all live in renew.go, where they can be read in one screen and tested without a
// database.
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
		// No row means the channel has never been signed in. A state, not a failure: it is how
		// this feature ships and what disabling it looks like.
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

// AccessToken returns the stored token if it is usable right now, and says exactly why not
// otherwise. This is what socialdigest's LinkedIn publisher calls on every publish.
//
// The expiry is checked HERE rather than left to LinkedIn's 401, because the two failures need
// different words: an expired credential is somebody's job to fix and says so, while a 401 on
// a live token is LinkedIn revoking us and means something else entirely. Reading the token
// per publish rather than caching it also means a renewal that landed an hour ago is picked up
// without a restart — this is one row read once a day, so there is nothing to save.
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

// The two ways a publish can fail before it is attempted. Exported so a caller can tell "this
// channel was never turned on" from "this channel is broken" — the first is a clean skip, the
// second is a run that must exit non-zero.
var (
	ErrNoCredential      = errors.New("linkedinauth: no stored credential")
	ErrCredentialExpired = errors.New("linkedinauth: stored credential has expired")
)

// pgTime maps a zero time to SQL NULL. The zero value is how this package spells "LinkedIn did
// not give us one", and writing it as year 1 would make an absent refresh token read as one
// that expired two millennia ago — the same warning, for the wrong reason.
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
