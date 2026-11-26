// Package availability is the one availability calendar a user has for every product that
// books them: mentorship, buddy mocks and booking links. Weekly rules, dated overrides and
// the zone they are stated in live in user_availability; the occupied time is the union of
// every product's meetings. See AGENTS.md of bookinglink, mentorship and buddy.
package availability

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// Booking-link session parameters when the user never saved any.
const (
	DefaultBookingDuration = 30 * time.Minute
	DefaultBookingNotice   = 12 * time.Hour
	// MaxBookingNotice is the longest notice a user can ask for.
	MaxBookingNotice = 168 * time.Hour
)

// BookingDurations are the meeting lengths a booking link offers.
var BookingDurations = []time.Duration{15 * time.Minute, 30 * time.Minute, 45 * time.Minute, time.Hour}

// ErrInvalidSettings is a duration or notice outside what a booking link offers.
var ErrInvalidSettings = errors.New("availability: invalid booking settings")

// Schedule is one user's hours. Zone is empty and Settings nil until the user saved
// settings; callers then fall back to their product's zone (and, for a booking link, to
// its default week).
type Schedule struct {
	Rules    []slots.Rule
	Zone     string
	Settings *Settings
}

// Settings are the booking-link session parameters beside the zone.
type Settings struct {
	Duration time.Duration
	Notice   time.Duration
}

// Source says which product a busy interval came from.
type Source string

// Busy sources, matching ListUserBusy.
const (
	SourceMentor       Source = "mentor"
	SourceMentorSeeker Source = "mentor_seeker"
	SourceBuddyMock    Source = "buddy_mock"
	SourceBookingLink  Source = "booking_link"
	SourceInterview    Source = "interview"
	SourceGoogle       Source = "google"
)

// Busy is one occupied interval of one user.
type Busy struct {
	UserID int64
	Source Source
	slots.Interval
}

// ValidZone reports whether name is an IANA place. It refuses what time.LoadLocation accepts
// but is not one ("Local" is the SERVER's zone; "EST" and "Factory" are nowhere).
func ValidZone(name string) bool {
	if name != "UTC" && !strings.Contains(name, "/") {
		return false
	}
	_, err := time.LoadLocation(name)
	return err == nil
}

// Store reads and writes availability.
type Store struct{ q *db.Queries }

// New builds a Store.
func New(q *db.Queries) *Store { return &Store{q: q} }

// Load is one user's schedule.
func (s *Store) Load(ctx context.Context, userID int64) (Schedule, error) {
	m, err := s.LoadMany(ctx, []int64{userID})
	return m[userID], err
}

// LoadMany is Load for many users in two statements. Every id has an entry.
func (s *Store) LoadMany(ctx context.Context, userIDs []int64) (map[int64]Schedule, error) {
	rows, err := s.q.ListUserAvailabilityForUsers(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	settings, err := s.q.ListUserAvailabilitySettings(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]Schedule, len(userIDs))
	for _, id := range userIDs {
		out[id] = Schedule{}
	}
	for _, row := range rows {
		// A row the domain cannot represent is skipped: one corrupt row must not make a
		// user unbookable, and the CHECKs stop one existing.
		if rule, ok := ruleFromRow(row); ok {
			sc := out[row.UserID]
			sc.Rules = append(sc.Rules, rule.WithID(row.ID))
			out[row.UserID] = sc
		}
	}
	for _, st := range settings {
		sc := out[st.UserID]
		sc.Zone = st.Timezone
		sc.Settings = &Settings{
			Duration: time.Duration(st.BookingDurationMin) * time.Minute,
			Notice:   time.Duration(st.BookingNoticeMin) * time.Minute,
		}
		out[st.UserID] = sc
	}
	return out, nil
}

// ReplaceWeekly swaps the recurring week, leaving dated overrides alone.
func (s *Store) ReplaceWeekly(ctx context.Context, userID int64, rules []slots.Rule) error {
	p := db.ReplaceUserAvailabilityWeeklyParams{
		UserID: userID, Weekdays: []int16{}, StartTimes: []pgtype.Time{}, EndTimes: []pgtype.Time{},
	}
	for _, rule := range rules {
		if rule.IsDated() {
			continue
		}
		p.Weekdays = append(p.Weekdays, int16(rule.Weekday()))
		p.StartTimes = append(p.StartTimes, pgClock(rule.Start()))
		p.EndTimes = append(p.EndTimes, pgClock(rule.End()))
	}
	return s.q.ReplaceUserAvailabilityWeekly(ctx, p)
}

// Add stores one rule, weekly or dated, and returns it with its row id.
func (s *Store) Add(ctx context.Context, userID int64, rule slots.Rule) (slots.Rule, error) {
	p := db.CreateUserAvailabilityRuleParams{
		UserID: userID, StartTime: pgClock(rule.Start()), EndTime: pgClock(rule.End()),
	}
	if rule.IsDated() {
		year, month, day := rule.Date().Parts()
		p.OnDate = pgtype.Date{Time: time.Date(year, month, day, 0, 0, 0, 0, time.UTC), Valid: true}
	} else {
		p.Weekday = pgtype.Int2{Int16: int16(rule.Weekday()), Valid: true}
	}
	row, err := s.q.CreateUserAvailabilityRule(ctx, p)
	if err != nil {
		return slots.Rule{}, err
	}
	return rule.WithID(row.ID), nil
}

// Delete removes one rule of the user's own schedule and reports whether it existed.
func (s *Store) Delete(ctx context.Context, userID, ruleID int64) (bool, error) {
	n, err := s.q.DeleteUserAvailabilityRule(ctx, db.DeleteUserAvailabilityRuleParams{ID: ruleID, UserID: userID})
	return n == 1, err
}

// SaveSettings validates and stores the zone and booking parameters. zone is checked by
// the caller (each layer has its own IANA rule); duration and notice are checked here.
func (s *Store) SaveSettings(ctx context.Context, userID int64, zone string, st Settings) error {
	if !slices.Contains(BookingDurations, st.Duration) || st.Notice < 0 || st.Notice > MaxBookingNotice {
		return ErrInvalidSettings
	}
	_, err := s.q.UpsertUserAvailabilitySettings(ctx, db.UpsertUserAvailabilitySettingsParams{
		UserID: userID, Timezone: zone,
		BookingDurationMin: int32(st.Duration / time.Minute), BookingNoticeMin: int32(st.Notice / time.Minute),
	})
	return err
}

// FollowZone carries a product's zone edit into the shared settings, when some were saved,
// so the edit is not silently ignored.
func (s *Store) FollowZone(ctx context.Context, userID int64, zone string) error {
	return s.q.SetUserAvailabilityZone(ctx, db.SetUserAvailabilityZoneParams{UserID: userID, Timezone: zone})
}

// SetZone sets the zone the hours are stated in, creating the settings with the booking
// defaults when the user has none.
func (s *Store) SetZone(ctx context.Context, userID int64, zone string) error {
	return s.q.UpsertUserAvailabilityZone(ctx, db.UpsertUserAvailabilityZoneParams{UserID: userID, Timezone: zone})
}

// Busy is the occupied time of users in a window, from every product, keyed by user.
func (s *Store) Busy(ctx context.Context, userIDs []int64, from, to time.Time) (map[int64][]Busy, error) {
	rows, err := s.q.ListUserBusy(ctx, db.ListUserBusyParams{
		UserIds: userIDs, WindowStart: pgconv.Instant(from), WindowEnd: pgconv.Instant(to),
	})
	if err != nil {
		return nil, err
	}
	out := make(map[int64][]Busy, len(userIDs))
	for _, row := range rows {
		out[row.UserID] = append(out[row.UserID], Busy{
			UserID: row.UserID, Source: Source(row.Source),
			Interval: slots.Interval{Start: row.StartsAt.Time, End: row.EndsAt.Time},
		})
	}
	return out, nil
}

// Intervals drops the sources.
func Intervals(b []Busy) []slots.Interval {
	out := make([]slots.Interval, len(b))
	for i, x := range b {
		out[i] = x.Interval
	}
	return out
}

// ruleFromRow rebuilds a domain rule, reporting false for a row that is neither shape.
func ruleFromRow(row db.UserAvailability) (slots.Rule, bool) {
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
