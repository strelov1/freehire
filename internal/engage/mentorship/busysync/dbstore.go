package busysync

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// DBStore adapts *db.Queries to the sync worker's Store.
type DBStore struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

// NewDBStore wraps the generated queries. pool is needed alongside q because
// ReplaceBusyWindow's delete-then-insert must be one transaction.
func NewDBStore(q *db.Queries, pool *pgxpool.Pool) *DBStore { return &DBStore{q: q, pool: pool} }

// ListConnections returns the mentors whose grant covers busy-sync. Every predicate (opt-in
// flag, scope, published profile) lives in the query.
func (s *DBStore) ListConnections(ctx context.Context) ([]Connection, error) {
	rows, err := s.q.ListMentorBusySyncConnections(ctx, gmailsync.CalendarScope)
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(rows))
	for _, r := range rows {
		out = append(out, Connection{MentorID: r.MentorID, UserID: r.UserID})
	}
	return out, nil
}

func (s *DBStore) RefreshToken(ctx context.Context, userID int64) (string, error) {
	r, err := s.q.GetGmailRefreshToken(ctx, userID)
	if err != nil {
		return "", err
	}
	return r.RefreshTokenEnc, nil
}

// ReplaceBusyWindow reconciles one mentor's stored busy set in a single transaction
// (delete, then insert), so ListBusy never observes a half-emptied set.
func (s *DBStore) ReplaceBusyWindow(ctx context.Context, mentorID int64, windowEnd time.Time, periods []BusyPeriod) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.q.WithTx(tx)
	if err := q.DeleteMentorBusyIntervalsInWindow(ctx, db.DeleteMentorBusyIntervalsInWindowParams{
		MentorID:  mentorID,
		WindowEnd: pgconv.Instant(windowEnd),
	}); err != nil {
		return err
	}
	for _, p := range periods {
		if err := q.UpsertMentorBusyInterval(ctx, db.UpsertMentorBusyIntervalParams{
			MentorID:   mentorID,
			StartsAt:   pgconv.Instant(p.Start),
			EndsAt:     pgconv.Instant(p.End),
			ExternalID: externalID(p.Start, p.End),
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// externalID derives the storage key Google never provides for a free/busy period: the
// interval's bounds, so a re-sync of an unchanged interval updates the same row. Not
// hashed: the bounds already sit in cleartext on the same row.
func externalID(start, end time.Time) string {
	return start.UTC().Format(time.RFC3339) + "|" + end.UTC().Format(time.RFC3339)
}

// SetNeedsReconsent flags the grant, which every Google feature on this account shares.
// SetGmailStatus itself clears mentor_busy_sync_opted_in alongside.
func (s *DBStore) SetNeedsReconsent(ctx context.Context, userID int64) error {
	return s.q.SetGmailStatus(ctx, db.SetGmailStatusParams{UserID: userID, Status: "needs_reconsent"})
}
