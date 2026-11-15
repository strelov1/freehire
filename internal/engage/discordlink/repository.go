package discordlink

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// PostgresStore is the Store backed by the generated queries. It holds no rules;
// those are decided above it, testable without a database.
type PostgresStore struct {
	q *db.Queries
}

// Asserted so a drifting signature fails to build here, not at the composition
// root.
var (
	_ Store   = (*PostgresStore)(nil)
	_ Discord = (*Client)(nil)
)

// NewPostgresStore wraps the generated query set.
func NewPostgresStore(q *db.Queries) *PostgresStore {
	return &PostgresStore{q: q}
}

func (s *PostgresStore) Link(ctx context.Context, userID int64, discordUserID string) (Link, error) {
	row, err := s.q.LinkDiscordAccount(ctx, db.LinkDiscordAccountParams{
		UserID: userID, DiscordUserID: discordUserID,
	})
	if err != nil {
		// The only unique index this statement can violate is on
		// discord_links.discord_user_id (the primary key is conflict-handled in the
		// statement), so a violation means another account holds that Discord account.
		if pgerr.IsUniqueViolation(err) {
			return Link{}, ErrAlreadyLinkedElsewhere
		}
		return Link{}, err
	}
	return Link{
		UserID:        row.UserID,
		DiscordUserID: row.DiscordUserID,
		RoleGranted:   row.RoleGrantedAt.Valid,
	}, nil
}

func (s *PostgresStore) Get(ctx context.Context, userID int64) (Link, error) {
	row, err := s.q.GetDiscordLink(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Link{}, ErrNotLinked
	}
	if err != nil {
		return Link{}, err
	}
	return Link{
		UserID:        row.UserID,
		DiscordUserID: row.DiscordUserID,
		RoleGranted:   row.RoleGrantedAt.Valid,
	}, nil
}

func (s *PostgresStore) Unlink(ctx context.Context, userID int64) (bool, error) {
	n, err := s.q.UnlinkDiscordAccount(ctx, userID)
	return n > 0, err
}

func (s *PostgresStore) Plan(ctx context.Context, userID int64) (time.Time, time.Time, error) {
	row, err := s.q.GetPlanUntils(ctx, userID)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return row.ProUntil.Time, row.UltraUntil.Time, nil
}

func (s *PostgresStore) ListToSync(ctx context.Context, limit int32) ([]Candidate, error) {
	rows, err := s.q.ListDiscordLinksToSync(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		out = append(out, Candidate{
			UserID:        row.UserID,
			DiscordUserID: row.DiscordUserID,
			// A NULL role_granted_at means the role is not held.
			RoleGranted: row.RoleGrantedAt.Valid,
			// A NULL entitlement becomes the zero time, which TierOf reads as free.
			ProUntil:   row.ProUntil.Time,
			UltraUntil: row.UltraUntil.Time,
		})
	}
	return out, nil
}

// SetRoleGranted records whether the role is held and that the binding was just
// examined. It passes the fact, not an instant: the statement decides when, so
// an already-granted role keeps its original instant.
func (s *PostgresStore) SetRoleGranted(ctx context.Context, userID int64, granted bool) error {
	return s.q.SetDiscordRoleGranted(ctx, db.SetDiscordRoleGrantedParams{
		UserID: userID, Granted: granted,
	})
}
