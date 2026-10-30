package buddy

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/db"
)

var _ MockRepository = (*QueriesRepository)(nil)

func (r *QueriesRepository) MockSettings(ctx context.Context, userID int64) (MockSettings, bool, error) {
	row, err := r.q.GetBuddyMockSettings(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return MockSettings{}, false, nil
	}
	if err != nil {
		return MockSettings{}, false, err
	}
	return mockSettingsFromRow(row), true, nil
}

func (r *QueriesRepository) SaveMockSettings(ctx context.Context, userID int64, s MockSettings) (MockSettings, error) {
	row, err := r.q.UpsertBuddyMockSettings(ctx, db.UpsertBuddyMockSettingsParams{
		UserID: userID, Timezone: s.Timezone, MeetingUrl: s.MeetingURL, Topics: s.Topics, Paused: s.Paused,
	})
	if err != nil {
		return MockSettings{}, err
	}
	return mockSettingsFromRow(row), nil
}

func mockSettingsFromRow(row db.BuddyMockSetting) MockSettings {
	return MockSettings{Timezone: row.Timezone, MeetingURL: row.MeetingUrl, Topics: row.Topics, Paused: row.Paused}
}

// MockAvailability skips a row the domain cannot represent rather than failing the read:
// one corrupt row must not make a member unbookable, and the CHECKs stop one existing.
func (r *QueriesRepository) MockAvailability(ctx context.Context, userID int64) ([]slots.Rule, error) {
	rows, err := r.q.ListBuddyMockAvailability(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]slots.Rule, 0, len(rows))
	for _, row := range rows {
		if rule, ok := mockRuleFromRow(row); ok {
			out = append(out, rule.WithID(row.ID))
		}
	}
	return out, nil
}

func (r *QueriesRepository) ReplaceMockWeekly(ctx context.Context, userID int64, rules []slots.Rule) error {
	p := db.ReplaceBuddyMockWeeklyParams{
		UserID: userID, Weekdays: []int16{}, StartTimes: []pgtype.Time{}, EndTimes: []pgtype.Time{},
	}
	for _, rule := range rules {
		p.Weekdays = append(p.Weekdays, int16(rule.Weekday()))
		p.StartTimes = append(p.StartTimes, pgClock(rule.Start()))
		p.EndTimes = append(p.EndTimes, pgClock(rule.End()))
	}
	return r.q.ReplaceBuddyMockWeekly(ctx, p)
}

func (r *QueriesRepository) AddMockRule(ctx context.Context, userID int64, rule slots.Rule) (slots.Rule, error) {
	p := db.CreateBuddyMockAvailabilityRuleParams{
		UserID: userID, StartTime: pgClock(rule.Start()), EndTime: pgClock(rule.End()),
	}
	if rule.IsDated() {
		year, month, day := rule.Date().Parts()
		p.OnDate = pgtype.Date{Time: time.Date(year, month, day, 0, 0, 0, 0, time.UTC), Valid: true}
	} else {
		p.Weekday = pgtype.Int2{Int16: int16(rule.Weekday()), Valid: true}
	}
	row, err := r.q.CreateBuddyMockAvailabilityRule(ctx, p)
	if err != nil {
		return slots.Rule{}, err
	}
	return rule.WithID(row.ID), nil
}

func (r *QueriesRepository) DeleteMockRule(ctx context.Context, userID, ruleID int64) (bool, error) {
	n, err := r.q.DeleteBuddyMockAvailabilityRule(ctx, db.DeleteBuddyMockAvailabilityRuleParams{ID: ruleID, UserID: userID})
	return n == 1, err
}

// mockRuleFromRow rebuilds a domain rule, reporting false for a row that is neither shape.
// mentorship keeps the same conversion for mentor_availability; the two tables are
// different sqlc types, so each adapter converts its own.
func mockRuleFromRow(row db.BuddyMockAvailability) (slots.Rule, bool) {
	start, startOK := clockFromPg(row.StartTime)
	end, endOK := clockFromPg(row.EndTime)
	if !startOK || !endOK {
		return slots.Rule{}, false
	}
	if row.OnDate.Valid {
		date, err := slots.NewDate(row.OnDate.Time.Year(), row.OnDate.Time.Month(), row.OnDate.Time.Day())
		if err != nil {
			return slots.Rule{}, false
		}
		rule, err := slots.NewDatedRule(date, start, end)
		return rule, err == nil
	}
	if !row.Weekday.Valid {
		return slots.Rule{}, false
	}
	rule, err := slots.NewWeeklyRule(time.Weekday(row.Weekday.Int16), start, end)
	return rule, err == nil
}

// pgClock renders a wall-clock time as microseconds since midnight; 24:00 is a valid
// Postgres time and how a rule reaches the end of its day.
func pgClock(t slots.TimeOfDay) pgtype.Time {
	return pgtype.Time{Microseconds: int64(t.Minutes()) * 60 * 1_000_000, Valid: true}
}

// clockFromPg reads one back, refusing a value finer than a minute rather than rounding it.
func clockFromPg(t pgtype.Time) (slots.TimeOfDay, bool) {
	const microsPerMinute = 60 * 1_000_000
	if !t.Valid || t.Microseconds%microsPerMinute != 0 {
		return slots.TimeOfDay{}, false
	}
	minutes := int(t.Microseconds / microsPerMinute)
	got, err := slots.NewTimeOfDay(minutes/60, minutes%60)
	return got, err == nil
}
