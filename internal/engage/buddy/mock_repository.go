package buddy

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
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

func (r *QueriesRepository) MockBusy(ctx context.Context, userID int64, from, to time.Time) ([]slots.Interval, error) {
	rows, err := r.q.ListBuddyMockBusy(ctx, db.ListBuddyMockBusyParams{
		UserID: userID, WindowStart: pgInstant(from), WindowEnd: pgInstant(to),
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

func pgInstant(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

// constraintMockNoOverlap is the EXCLUDE that keeps one person out of two overlapping mocks
// in either role. Matched by name, so another exclusion could never be answered with this
// one's meaning.
const constraintMockNoOverlap = "buddy_mock_participants_no_overlap"

func (r *QueriesRepository) FutureGuestMocks(ctx context.Context, guestID int64) (int, error) {
	n, err := r.q.CountFutureBuddyMocksAsGuest(ctx, guestID)
	return int(n), err
}

// createMockAttempts bounds the deadlock retry in CreateMock.
const createMockAttempts = 3

func (r *QueriesRepository) CreateMock(ctx context.Context, in NewMock) (Mock, error) {
	p := db.CreateBuddyMockParams{
		HostUserID: in.HostUserID, GuestUserID: in.GuestUserID,
		StartsAt: pgInstant(in.StartsAt), EndsAt: pgInstant(in.EndsAt),
		Topic: in.Topic, GuestTimezone: in.GuestTimezone,
	}
	// Two bookings racing for one person's time each insert a participant row and then
	// wait on the other's uncommitted one, and Postgres breaks that cycle by aborting one
	// with 40P01 instead of answering 23P01. The statement wrote nothing, so it is run
	// again: the retry waits for the survivor's commit and then meets the EXCLUDE plainly.
	var id pgtype.UUID
	var err error
	for range createMockAttempts {
		if id, err = r.q.CreateBuddyMock(ctx, p); !pgerr.IsDeadlock(err) {
			break
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Mock{}, ErrSlotUnavailable
	}
	// SQLSTATE 23P01, not a unique violation's 23505 — the lost race, answered as a stale page.
	if name, ok := pgerr.ExclusionViolationConstraint(err); ok && name == constraintMockNoOverlap {
		return Mock{}, ErrSlotUnavailable
	}
	if err != nil {
		return Mock{}, err
	}
	mockID := uuid.UUID(id.Bytes)
	// The insert returns the id only, and the booked notice needs both handles, zones and
	// addresses. A failed re-read falls back to what was written: the mock is committed,
	// and losing its notice beats failing a booking that happened.
	if full, found, err := r.MockByID(ctx, mockID); err == nil && found {
		return full, nil
	}
	return Mock{
		ID: mockID, HostUserID: in.HostUserID, GuestUserID: in.GuestUserID,
		StartsAt: in.StartsAt, EndsAt: in.EndsAt, Topic: in.Topic, Status: MockConfirmed,
		GuestTimezone: in.GuestTimezone,
	}, nil
}

func (r *QueriesRepository) MockByID(ctx context.Context, id uuid.UUID) (Mock, bool, error) {
	row, err := r.q.GetBuddyMock(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return Mock{}, false, nil
	}
	if err != nil {
		return Mock{}, false, err
	}
	return Mock{
		ID: uuid.UUID(row.ID.Bytes), HostUserID: row.HostUserID, GuestUserID: row.GuestUserID,
		HostHandle: row.HostHandle, GuestHandle: row.GuestHandle,
		StartsAt: row.StartsAt.Time, EndsAt: row.EndsAt.Time,
		Topic: row.Topic, Status: row.Status, CancelReason: row.CancelReason.String,
		HostTimezone: row.HostTimezone, GuestTimezone: row.GuestTimezone, MeetingURL: row.MeetingUrl,
		HostEmail: row.HostEmail, GuestEmail: row.GuestEmail,
	}, true, nil
}

func (r *QueriesRepository) CancelMock(ctx context.Context, id uuid.UUID, userID int64) (bool, error) {
	_, err := r.q.CancelBuddyMock(ctx, db.CancelBuddyMockParams{ID: pgtype.UUID{Bytes: id, Valid: true}, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// ClaimCancelNotices returns what it read before an error, so the claimed mocks it did
// read are still announced.
func (r *QueriesRepository) ClaimCancelNotices(ctx context.Context, actorID int64) ([]Mock, error) {
	ids, err := r.q.ClaimBuddyMockCancelNotices(ctx, actorID)
	if err != nil {
		return nil, err
	}
	out := make([]Mock, 0, len(ids))
	for _, id := range ids {
		m, found, err := r.MockByID(ctx, uuid.UUID(id.Bytes))
		if err != nil {
			return out, err
		}
		if found {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *QueriesRepository) MockEntries(ctx context.Context, userID int64, limit int32) ([]MockEntry, error) {
	rows, err := r.q.ListBuddyMocksForUser(ctx, db.ListBuddyMocksForUserParams{UserID: userID, RowLimit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]MockEntry, len(rows))
	for i, row := range rows {
		e := MockEntry{
			ID: uuid.UUID(row.ID.Bytes), Role: RoleGuest, Counterpart: row.HostHandle,
			StartsAt: row.StartsAt.Time, EndsAt: row.EndsAt.Time, Topic: row.Topic, Status: row.Status,
			CancelReason: row.CancelReason.String, MeetingURL: row.HostMeetingUrl.String,
		}
		if row.HostUserID == userID {
			e.Role, e.Counterpart = RoleHost, row.GuestHandle
		}
		if !row.CounterpartVisible {
			e.Counterpart = ""
		}
		if row.MyRating.Valid {
			e.MyReview = &MockReview{Rating: int(row.MyRating.Int16), Comment: row.MyComment.String}
		}
		if row.TheirRating.Valid {
			e.TheirReview = &MockReview{Rating: int(row.TheirRating.Int16), Comment: row.TheirComment.String}
		}
		out[i] = e
	}
	return out, nil
}

func (r *QueriesRepository) CreateMockReview(ctx context.Context, id uuid.UUID, authorID int64, rating int, comment string) (bool, error) {
	n, err := r.q.CreateBuddyMockReview(ctx, db.CreateBuddyMockReviewParams{
		MockID: pgtype.UUID{Bytes: id, Valid: true}, AuthorUserID: authorID, Rating: int16(rating), Comment: comment,
	})
	return n == 1, err
}

func (r *QueriesRepository) MockSummary(ctx context.Context, subjectID int64, recent int32) (MockSummary, error) {
	agg, err := r.q.GetBuddyMemberMockSummary(ctx, subjectID)
	if err != nil {
		return MockSummary{}, err
	}
	rows, err := r.q.ListBuddyMemberRevealedReviews(ctx, db.ListBuddyMemberRevealedReviewsParams{
		SubjectUserID: subjectID, RowLimit: recent,
	})
	if err != nil {
		return MockSummary{}, err
	}
	sum := MockSummary{
		Completed: int(agg.Completed), RatingCount: int(agg.RatingCount), RatingAvg: agg.RatingAvg,
		Recent: make([]RevealedReview, len(rows)),
	}
	for i, row := range rows {
		sum.Recent[i] = RevealedReview{
			Rating: int(row.Rating), Comment: row.Comment, Topic: row.Topic,
			Day: row.CreatedAt.Time.UTC().Format(time.DateOnly),
		}
	}
	return sum, nil
}

func (r *QueriesRepository) MocksDueForReminder(ctx context.Context, offset, floor time.Duration, limit int32) ([]Mock, error) {
	ids, err := r.q.ListBuddyMocksDueForReminder(ctx, db.ListBuddyMocksDueForReminderParams{
		OffsetMinutes: int32(offset / time.Minute), FloorMinutes: int32(floor / time.Minute), RowLimit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Mock, 0, len(ids))
	for _, id := range ids {
		m, found, err := r.MockByID(ctx, uuid.UUID(id.Bytes))
		if err != nil {
			return nil, err
		}
		if found {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *QueriesRepository) ClaimMockReminder(ctx context.Context, id uuid.UUID, offset time.Duration) (bool, error) {
	n, err := r.q.RecordBuddyMockReminder(ctx, db.RecordBuddyMockReminderParams{
		MockID: pgtype.UUID{Bytes: id, Valid: true}, OffsetMinutes: int32(offset / time.Minute),
	})
	return n == 1, err
}

func (r *QueriesRepository) ReleaseMockReminder(ctx context.Context, id uuid.UUID, offset time.Duration) error {
	_, err := r.q.DeleteBuddyMockReminder(ctx, db.DeleteBuddyMockReminderParams{
		MockID: pgtype.UUID{Bytes: id, Valid: true}, OffsetMinutes: int32(offset / time.Minute),
	})
	return err
}
