package handler

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
)

// tailoringKeyTTL bounds how long the minted CLI credential is valid. A tailoring session is
// interactive and short; a couple of hours covers it while limiting the blast radius of a key
// that leaks out of the agent's environment.
const tailoringKeyTTL = 2 * time.Hour

// apiKeyMinter is the slice of the query surface mintTailoringKey needs (*db.Queries satisfies
// it), kept narrow so the mint logic is unit-testable without a database.
type apiKeyMinter interface {
	CreateAPIKey(ctx context.Context, arg db.CreateAPIKeyParams) (db.CreateAPIKeyRow, error)
}

// mintTailoringKey issues a short-lived API key the tailoring agent's CLI uses to act as the
// user against the CV endpoints. It reuses the api_keys machinery; there is no per-endpoint
// scope, so the key is owner-scoped only — the CV endpoints' own owner checks confine it to
// this user's CVs. The plaintext token is returned once (to hand to the agent session) and
// only its hash is stored.
func mintTailoringKey(ctx context.Context, q apiKeyMinter, userID int64, now time.Time) (string, error) {
	token, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		return "", err
	}
	if _, err := q.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		UserID:      userID,
		Name:        "CV tailoring session",
		TokenHash:   hash,
		TokenPrefix: prefix,
		ExpiresAt:   pgtype.Timestamptz{Time: now.Add(tailoringKeyTTL), Valid: true},
	}); err != nil {
		return "", err
	}
	return token, nil
}
