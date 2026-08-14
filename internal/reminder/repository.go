package reminder

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/notify"
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

// GetSettings returns the caller's rule. A missing row reads as the
// opt-out-by-default default (enabled, channel email) rather than an error or a
// disabled rule — see the centralize-lifecycle-notifications change for why: this
// changes only what "never configured" means, an explicit stored choice (enabled
// or disabled) is always returned as stored.
func (r *QueriesRepository) GetSettings(ctx context.Context, userID int64) (Settings, error) {
	row, err := r.q.GetNotificationSettings(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{Enabled: true, Channels: []string{notify.ChannelEmail}, DigestFrequency: "instant"}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		Enabled:         row.Enabled,
		Channels:        row.Channels,
		DigestFrequency: row.DigestFrequency,
		DigestTime:      pgconv.DurationPtr(row.DigestTime),
		QuietHoursStart: pgconv.DurationPtr(row.QuietHoursStart),
		QuietHoursEnd:   pgconv.DurationPtr(row.QuietHoursEnd),
	}, nil
}

// UpsertSettings creates or replaces the caller's rule.
func (r *QueriesRepository) UpsertSettings(ctx context.Context, userID int64, s Settings) (Settings, error) {
	freq := s.DigestFrequency
	if freq == "" {
		freq = "instant"
	}
	row, err := r.q.UpsertNotificationSettings(ctx, db.UpsertNotificationSettingsParams{
		UserID:          userID,
		Enabled:         s.Enabled,
		Channels:        s.Channels,
		DigestFrequency: freq,
		DigestTime:      pgconv.Duration(s.DigestTime),
		QuietHoursStart: pgconv.Duration(s.QuietHoursStart),
		QuietHoursEnd:   pgconv.Duration(s.QuietHoursEnd),
	})
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		Enabled:         row.Enabled,
		Channels:        row.Channels,
		DigestFrequency: row.DigestFrequency,
		DigestTime:      pgconv.DurationPtr(row.DigestTime),
		QuietHoursStart: pgconv.DurationPtr(row.QuietHoursStart),
		QuietHoursEnd:   pgconv.DurationPtr(row.QuietHoursEnd),
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
