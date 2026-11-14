package reminder

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
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
		defaultTime := notify.DefaultDigestTime
		return Settings{
			Enabled:             true,
			Channels:            []string{notify.ChannelEmail},
			DigestFrequency:     notify.FrequencyDaily,
			DigestTime:          &defaultTime,
			DigestIntervalHours: notify.DefaultIntervalHours,
		}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		Enabled:             row.Enabled,
		InboxDigestEnabled:  row.InboxDigestEnabled,
		Channels:            row.Channels,
		DigestFrequency:     row.DigestFrequency,
		DigestTime:          pgconv.DurationPtr(row.DigestTime),
		DigestIntervalHours: int(row.DigestIntervalHours),
		QuietHoursStart:     pgconv.DurationPtr(row.QuietHoursStart),
		QuietHoursEnd:       pgconv.DurationPtr(row.QuietHoursEnd),
	}, nil
}

// UpsertSettings creates or replaces the caller's rule.
// HasPushDevice reports whether the account has a registered push device.
func (r *QueriesRepository) HasPushDevice(ctx context.Context, userID int64) (bool, error) {
	return r.q.UserHasPushToken(ctx, userID)
}

// The digest defaults are applied here as well as in Service.UpdateSettings, since
// this is the last stop before the column CHECKs: a zero interval must store as the
// default, not violate digest_interval_hours' bounds.
func (r *QueriesRepository) UpsertSettings(ctx context.Context, userID int64, s Settings) (Settings, error) {
	freq := s.DigestFrequency
	digestTime := s.DigestTime
	if freq == "" {
		freq = notify.FrequencyDaily
		if digestTime == nil {
			t := notify.DefaultDigestTime
			digestTime = &t
		}
	}
	if s.InboxDigestEnabled && digestTime == nil {
		t := notify.DefaultDigestTime
		digestTime = &t
	}
	hours := s.DigestIntervalHours
	if hours == 0 {
		hours = notify.DefaultIntervalHours
	}
	row, err := r.q.UpsertNotificationSettings(ctx, db.UpsertNotificationSettingsParams{
		UserID:              userID,
		Enabled:             s.Enabled,
		InboxDigestEnabled:  s.InboxDigestEnabled,
		Channels:            s.Channels,
		DigestFrequency:     freq,
		DigestTime:          pgconv.Duration(digestTime),
		DigestIntervalHours: int16(hours),
		QuietHoursStart:     pgconv.Duration(s.QuietHoursStart),
		QuietHoursEnd:       pgconv.Duration(s.QuietHoursEnd),
	})
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		Enabled:             row.Enabled,
		InboxDigestEnabled:  row.InboxDigestEnabled,
		Channels:            row.Channels,
		DigestFrequency:     row.DigestFrequency,
		DigestTime:          pgconv.DurationPtr(row.DigestTime),
		DigestIntervalHours: int(row.DigestIntervalHours),
		QuietHoursStart:     pgconv.DurationPtr(row.QuietHoursStart),
		QuietHoursEnd:       pgconv.DurationPtr(row.QuietHoursEnd),
	}, nil
}

// GetScheduleContext reads the account's notification hour and timezone. A NULL in
// either column, and a timezone name this build's tzdata cannot resolve, are left
// as nil for the service to read as unconfigured — a stale or misspelled zone name
// must not stop a save from scheduling anything.
func (r *QueriesRepository) GetScheduleContext(ctx context.Context, userID int64) (ScheduleContext, error) {
	row, err := r.q.GetReminderScheduleContext(ctx, userID)
	if err != nil {
		return ScheduleContext{}, err
	}
	sc := ScheduleContext{NotificationHour: pgconv.DurationPtr(row.DigestTime)}
	if row.Timezone.Valid {
		if loc, err := time.LoadLocation(row.Timezone.String); err == nil {
			sc.Location = loc
		}
	}
	return sc, nil
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

func (r *QueriesRepository) HasPaidPlan(ctx context.Context, userID int64) (bool, error) {
	untils, err := r.q.GetPlanUntils(ctx, userID)
	if err != nil {
		return false, err
	}
	return plan.TierOf(untils.ProUntil.Time, untils.UltraUntil.Time, time.Now()).Paid(), nil
}
