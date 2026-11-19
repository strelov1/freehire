package bookinglink

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// constraintNoOverlap is the EXCLUDE that refuses two confirmed bookings for one candidate
// at once; its violation is the lost race.
const constraintNoOverlap = "booking_link_bookings_no_overlap"

// InterviewSource is application_interviews.source for a meeting booked through a link.
const InterviewSource = "booking_link"

// bookAttempts bounds the deadlock retry: two inserts into the GiST exclusion index can
// each wait on the other and Postgres aborts one with 40P01; the retry meets the EXCLUDE.
const bookAttempts = 3

// QueriesRepository is the Postgres Repository.
type QueriesRepository struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

var _ Repository = (*QueriesRepository)(nil)

// NewQueriesRepository builds a QueriesRepository.
func NewQueriesRepository(q *db.Queries, pool *pgxpool.Pool) *QueriesRepository {
	return &QueriesRepository{q: q, pool: pool}
}

func (r *QueriesRepository) Upsert(ctx context.Context, userID, applicationID int64, seed, hash []byte, zone string) (Link, bool, error) {
	row, err := r.q.UpsertBookingLink(ctx, db.UpsertBookingLinkParams{
		UserID: userID, ApplicationID: applicationID, TokenSeed: seed, TokenHash: hash, Zone: zone,
	})
	return found(Link(row), err)
}

func (r *QueriesRepository) ForApplication(ctx context.Context, userID, applicationID int64) (Link, bool, error) {
	row, err := r.q.GetBookingLinkForApplication(ctx, db.GetBookingLinkForApplicationParams{
		UserID: userID, ApplicationID: applicationID,
	})
	return found(Link(row), err)
}

func (r *QueriesRepository) ByHash(ctx context.Context, hash []byte) (Link, bool, error) {
	row, err := r.q.GetBookingLinkByHash(ctx, hash)
	return found(Link(row), err)
}

func found(l Link, err error) (Link, bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return Link{}, false, nil
	}
	if err != nil {
		return Link{}, false, err
	}
	return l, true, nil
}

func (r *QueriesRepository) SetHash(ctx context.Context, linkID int64, hash []byte) error {
	return r.q.SetBookingLinkTokenHash(ctx, db.SetBookingLinkTokenHashParams{ID: linkID, TokenHash: hash})
}

func (r *QueriesRepository) Busy(ctx context.Context, userID int64, from, to time.Time) ([]slots.Interval, error) {
	rows, err := r.q.ListBookingLinkBusy(ctx, db.ListBookingLinkBusyParams{
		UserID: userID, FromAt: pgconv.Instant(from), ToAt: pgconv.Instant(to),
	})
	if err != nil {
		return nil, err
	}
	out := make([]slots.Interval, 0, len(rows))
	for _, row := range rows {
		out = append(out, slots.Interval{Start: row.StartsAt.Time, End: row.EndsAt.Time})
	}
	return out, nil
}

func (r *QueriesRepository) Book(ctx context.Context, b NewBooking) error {
	var err error
	for range bookAttempts {
		if err = r.bookTx(ctx, b); !pgerr.IsDeadlock(err) {
			break
		}
	}
	// SQLSTATE 23P01, not 23505; matched by name so another constraint is never mistaken for it.
	if name, ok := pgerr.ExclusionViolationConstraint(err); ok && name == constraintNoOverlap {
		return ErrSlotUnavailable
	}
	return err
}

// bookTx writes the booking and, in the same transaction, the application's interview and
// its interview_scheduled ledger event, so the meeting cannot exist without showing on the
// application.
func (r *QueriesRepository) bookTx(ctx context.Context, b NewBooking) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		id, err := q.InsertBookingLinkBooking(ctx, db.InsertBookingLinkBookingParams{
			LinkID: b.Link.ID, UserID: b.Link.UserID,
			StartsAt: pgconv.Instant(b.StartsAt), EndsAt: pgconv.Instant(b.EndsAt),
			RecruiterName: b.RecruiterName, RecruiterEmail: b.RecruiterEmail,
		})
		if err != nil {
			return err
		}
		_, err = q.UpsertApplicationInterview(ctx, db.UpsertApplicationInterviewParams{
			UserID:        b.Link.UserID,
			ApplicationID: b.Link.ApplicationID,
			IcalUid:       "booking:" + strconv.FormatInt(id, 10),
			StartsAt:      pgconv.Instant(b.StartsAt),
			EndsAt:        pgconv.Instant(b.EndsAt),
			Title:         b.RecruiterName + " <" + b.RecruiterEmail + ">",
			Status:        "confirmed",
			Source:        InterviewSource,
			// The platform recording what an anonymous booker did: not the employer's own
			// pipeline, so not trusted for day math.
			EventSource: appevent.SourceSystem,
		})
		return err
	})
}
