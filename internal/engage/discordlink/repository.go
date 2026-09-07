package discordlink

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// PostgresStore is the Store backed by the generated queries. It holds no rules: who
// warrants the role and what reconciliation owes a binding are decided above it, where they
// can be read and tested without a database.
type PostgresStore struct {
	q *db.Queries
}

// The two implementations the service is actually wired to. Asserted here so a signature
// that drifts fails at build time in this package, rather than at the composition root with
// an error that names neither side clearly.
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
		// The only unique index this statement can violate is the one on
		// discord_links.discord_user_id — the primary key is conflict-handled in the
		// statement itself — so a unique violation means exactly one thing: another
		// freehire account already holds that Discord account.
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
			// A NULL role_granted_at is "the role is not held"; the instant itself is only
			// ever read for the operator's benefit.
			RoleGranted: row.RoleGrantedAt.Valid,
			// A NULL entitlement becomes the zero time, which TierOf reads as "not after
			// now" — free. That is the same answer, reached without a second nullable type
			// travelling up into the decision.
			ProUntil:   row.ProUntil.Time,
			UltraUntil: row.UltraUntil.Time,
		})
	}
	return out, nil
}

// SetRoleGranted records whether the role is held and that the binding was just examined.
//
// It passes the fact, not an instant: the statement decides when, so a role that was already
// granted keeps the instant it was granted at instead of having it pushed forward by every
// hourly pass. See the query's own comment.
func (s *PostgresStore) SetRoleGranted(ctx context.Context, userID int64, granted bool) error {
	return s.q.SetDiscordRoleGranted(ctx, db.SetDiscordRoleGrantedParams{
		UserID: userID, Granted: granted,
	})
}
