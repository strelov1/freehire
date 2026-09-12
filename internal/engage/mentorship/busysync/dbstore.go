package busysync

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/platform/db"
)

// DBStore adapts *db.Queries to the sync worker's Store.
type DBStore struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

// NewDBStore wraps the generated queries. pool is needed alongside q because
// ReplaceBusyWindow's delete-then-insert is one edit, not two: a mentor observed between
// them would see either an emptied or a duplicated busy set, and either can cost or
// falsely block a booking.
func NewDBStore(q *db.Queries, pool *pgxpool.Pool) *DBStore { return &DBStore{q: q, pool: pool} }

// ListConnections returns the mentors whose grant actually covers busy-sync — every
// predicate (the opt-in flag, the scope, the published profile) lives in the query, so
// this never spends an API call learning something the database already answers.
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

// ReplaceBusyWindow reconciles one mentor's stored busy set in a single transaction:
// delete first, insert second — the same shape ReplaceWeeklyAvailability uses and for the
// same reason (see booking_repository.go), so ListBusy never observes a half-emptied set.
func (s *DBStore) ReplaceBusyWindow(ctx context.Context, mentorID int64, windowEnd time.Time, periods []BusyPeriod) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.q.WithTx(tx)
	if err := q.DeleteMentorBusyIntervalsInWindow(ctx, db.DeleteMentorBusyIntervalsInWindowParams{
		MentorID:  mentorID,
		WindowEnd: pgtype.Timestamptz{Time: windowEnd, Valid: true},
	}); err != nil {
		return err
	}
	for _, p := range periods {
		if err := q.UpsertMentorBusyInterval(ctx, db.UpsertMentorBusyIntervalParams{
			MentorID:   mentorID,
			StartsAt:   pgtype.Timestamptz{Time: p.Start, Valid: true},
			EndsAt:     pgtype.Timestamptz{Time: p.End, Valid: true},
			ExternalID: externalID(p.Start, p.End),
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// externalID derives the storage key a free/busy period needs but Google never provides
// (see the package doc and design.md's "Free/busy, not events" decision): the interval's
// own bounds, concatenated, so a re-sync of an unchanged interval updates the same row
// rather than duplicating it, matching mentor_busy_intervals' unique constraint's original
// intent for an events-based sync. Not hashed: the bounds already sit in cleartext in
// starts_at/ends_at on the very same row, so hashing them here would buy no privacy —
// only two fewer characters to read back while debugging.
func externalID(start, end time.Time) string {
	return start.UTC().Format(time.RFC3339) + "|" + end.UTC().Format(time.RFC3339)
}

// SetNeedsReconsent flags the grant, shared with every other Google feature this account
// may use, AND clears the busy-sync opt-in flag. The second step is the one this feature
// owns alone: `scopes`/`status` naturally clear themselves against a real revocation
// (calsync's own comment on UpsertCalendarGrant explains why unioning scopes would be
// wrong), but this flag exists ONLY because calendar.readonly is shared with an unrelated
// grant, so nothing else would ever reset it — an unrelated reconnect through that other
// flow restores `status` to 'connected' with the same scope without ever touching this
// column, and without clearing it here that would silently resume a consent the mentor
// never re-gave.
func (s *DBStore) SetNeedsReconsent(ctx context.Context, userID int64) error {
	if err := s.q.SetGmailStatus(ctx, db.SetGmailStatusParams{UserID: userID, Status: "needs_reconsent"}); err != nil {
		return err
	}
	return s.q.ClearMentorBusySyncOptedIn(ctx, userID)
}
