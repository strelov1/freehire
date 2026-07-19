package reminder

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pgconv"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository adapts *db.Queries to the reminder Repository.
type QueriesRepository struct{ q *db.Queries }

// NewQueriesRepository wraps q as a Repository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository {
	return &QueriesRepository{q: q}
}

// JobIDBySlug resolves a public slug to its internal job id.
func (r *QueriesRepository) JobIDBySlug(ctx context.Context, slug string) (int64, error) {
	id, err := r.q.GetJobIDBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrJobNotFound
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}

// GetSettings returns the caller's rule. A missing row is the unconfigured default
// (disabled, DefaultDelayDays, no channels) rather than an error, so an untouched
// account reads as "reminders off" and the settings UI shows a sensible delay.
func (r *QueriesRepository) GetSettings(ctx context.Context, userID int64) (Settings, error) {
	row, err := r.q.GetReminderSettings(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{Enabled: false, DefaultDelayDays: DefaultDelayDays, Channels: []string{}}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		Enabled:          row.Enabled,
		DefaultDelayDays: int(row.DefaultDelayDays),
		Channels:         row.Channels,
	}, nil
}

// UpsertSettings creates or replaces the caller's rule.
func (r *QueriesRepository) UpsertSettings(ctx context.Context, userID int64, s Settings) (Settings, error) {
	row, err := r.q.UpsertReminderSettings(ctx, db.UpsertReminderSettingsParams{
		UserID:           userID,
		Enabled:          s.Enabled,
		DefaultDelayDays: int32(s.DefaultDelayDays),
		Channels:         s.Channels,
	})
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		Enabled:          row.Enabled,
		DefaultDelayDays: int(row.DefaultDelayDays),
		Channels:         row.Channels,
	}, nil
}

// UpsertReminder schedules or replaces the pending reminder for a (user, job).
func (r *QueriesRepository) UpsertReminder(ctx context.Context, userID, jobID int64, fireAt time.Time, channels []string) error {
	_, err := r.q.UpsertJobReminder(ctx, db.UpsertJobReminderParams{
		UserID:   userID,
		JobID:    jobID,
		FireAt:   pgconv.Timestamptz(&fireAt),
		Channels: channels,
	})
	return err
}

// CancelReminder cancels the pending reminder for a (user, job), idempotently.
func (r *QueriesRepository) CancelReminder(ctx context.Context, userID, jobID int64) error {
	_, err := r.q.CancelJobReminder(ctx, db.CancelJobReminderParams{UserID: userID, JobID: jobID})
	return err
}

// RescheduleReminder moves a pending reminder's deadline. No pending row for the
// pair -> ErrNoReminder.
func (r *QueriesRepository) RescheduleReminder(ctx context.Context, userID, jobID int64, fireAt time.Time) error {
	_, err := r.q.RescheduleJobReminder(ctx, db.RescheduleJobReminderParams{
		FireAt: pgconv.Timestamptz(&fireAt),
		UserID: userID,
		JobID:  jobID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoReminder
	}
	return err
}
