package buddy

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/availability"
	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
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
	settings := mockSettingsFromRow(row)
	if sc, err := r.avail().Load(ctx, userID); err != nil {
		return MockSettings{}, false, err
	} else if sc.Zone != "" {
		settings.Timezone = sc.Zone
	}
	return settings, true, nil
}

func (r *QueriesRepository) SaveMockSettings(ctx context.Context, userID int64, s MockSettings) (MockSettings, error) {
	old, oldErr := r.q.GetBuddyMockSettings(ctx, userID)
	row, err := r.q.UpsertBuddyMockSettings(ctx, db.UpsertBuddyMockSettingsParams{
		UserID: userID, Timezone: s.Timezone, MeetingUrl: s.MeetingURL, Topics: s.Topics, Paused: s.Paused,
	})
	if err != nil {
		return MockSettings{}, err
	}
	// Only a changed product zone is followed, so other edits cannot undo a zone set elsewhere.
	if oldErr == nil && old.Timezone != s.Timezone {
		if err := r.avail().FollowZone(ctx, userID, s.Timezone); err != nil {
			return MockSettings{}, err
		}
	}
	return mockSettingsFromRow(row), nil
}

func mockSettingsFromRow(row db.BuddyMockSetting) MockSettings {
	return MockSettings{Timezone: row.Timezone, MeetingURL: row.MeetingUrl, Topics: row.Topics, Paused: row.Paused}
}

func (r *QueriesRepository) avail() *availability.Store { return availability.New(r.q) }

// MockAvailability is the member's shared availability (see engage/availability).
func (r *QueriesRepository) MockAvailability(ctx context.Context, userID int64) ([]slots.Rule, error) {
	sc, err := r.avail().Load(ctx, userID)
	return sc.Rules, err
}

func (r *QueriesRepository) ReplaceMockWeekly(ctx context.Context, userID int64, rules []slots.Rule) error {
	if err := r.q.EnsureUserAvailabilityZoneFromBuddy(ctx, userID); err != nil {
		return err
	}
	return r.avail().ReplaceWeekly(ctx, userID, rules)
}

func (r *QueriesRepository) AddMockRule(ctx context.Context, userID int64, rule slots.Rule) (slots.Rule, error) {
	if err := r.q.EnsureUserAvailabilityZoneFromBuddy(ctx, userID); err != nil {
		return slots.Rule{}, err
	}
	return r.avail().Add(ctx, userID, rule)
}

func (r *QueriesRepository) DeleteMockRule(ctx context.Context, userID, ruleID int64) (bool, error) {
	return r.avail().Delete(ctx, userID, ruleID)
}

// MockBusy is the member's occupied time across every product, not only mocks.
func (r *QueriesRepository) MockBusy(ctx context.Context, userID int64, from, to time.Time) ([]slots.Interval, error) {
	all, err := r.avail().Busy(ctx, []int64{userID}, from, to)
	return availability.Intervals(all[userID]), err
}

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

// CreateMock books in one transaction: LockBuddyMembersForMock share-locks both
// participants' buddy_members rows in user_id order, THEN CreateBuddyMock checks
// eligibility and writes. That is the booking's half of the lock protocol a leave
// (Leave) and a block (the buddy_mocks_cancel_on_block trigger) take the same rows
// FOR UPDATE under; the check must be a later statement than the lock, since a statement
// keeps the snapshot it started with even after waiting on a row lock.
func (r *QueriesRepository) CreateMock(ctx context.Context, in NewMock) (Mock, error) {
	p := db.CreateBuddyMockParams{
		HostUserID: in.HostUserID, GuestUserID: in.GuestUserID,
		StartsAt: pgconv.Instant(in.StartsAt), EndsAt: pgconv.Instant(in.EndsAt),
		Topic: in.Topic, GuestTimezone: in.GuestTimezone,
	}
	// Two bookings racing for one person's time each insert a participant row and wait on the other's
	// uncommitted one; Postgres aborts one with 40P01 instead of 23P01. It wrote nothing, so run it
	// again: the retry waits for the survivor's commit and then meets the EXCLUDE plainly.
	var id pgtype.UUID
	var err error
	for range createMockAttempts {
		if id, err = r.createMockTx(ctx, p); !pgerr.IsDeadlock(err) {
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

func (r *QueriesRepository) createMockTx(ctx context.Context, p db.CreateBuddyMockParams) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		if err := q.LockBuddyMembersForMock(ctx, db.LockBuddyMembersForMockParams{
			HostUserID: p.HostUserID, GuestUserID: p.GuestUserID,
		}); err != nil {
			return err
		}
		var err error
		id, err = q.CreateBuddyMock(ctx, p)
		return err
	})
	return id, err
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
	sums, err := r.MockSummaries(ctx, []int64{subjectID})
	if err != nil {
		return MockSummary{}, err
	}
	rows, err := r.q.ListBuddyMemberRevealedReviews(ctx, db.ListBuddyMemberRevealedReviewsParams{
		SubjectUserID: subjectID, RowLimit: recent,
	})
	if err != nil {
		return MockSummary{}, err
	}
	sum := sums[subjectID]
	sum.Recent = make([]RevealedReview, len(rows))
	for i, row := range rows {
		sum.Recent[i] = RevealedReview{
			Rating: int(row.Rating), Comment: row.Comment, Topic: row.Topic,
			Day: row.CreatedAt.Time.UTC().Format(time.DateOnly),
		}
	}
	return sum, nil
}

func (r *QueriesRepository) MockSummaries(ctx context.Context, subjectIDs []int64) (map[int64]MockSummary, error) {
	rows, err := r.q.ListBuddyMemberMockSummaries(ctx, subjectIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]MockSummary, len(rows))
	for _, row := range rows {
		out[row.SubjectUserID] = MockSummary{
			Completed: int(row.Completed), RatingCount: int(row.RatingCount), RatingAvg: row.RatingAvg,
		}
	}
	return out, nil
}

func (r *QueriesRepository) MockHostCandidates(ctx context.Context, viewerID int64, topic string, limit int32) ([]MockHostCandidate, error) {
	rows, err := r.q.ListBuddyMockHostCandidates(ctx, db.ListBuddyMockHostCandidatesParams{
		ViewerID: viewerID, Topic: pgconv.Text(topic), RowLimit: limit,
	})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.UserID
	}
	schedules, err := r.avail().LoadMany(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]MockHostCandidate, len(rows))
	for i, row := range rows {
		if zone := schedules[row.UserID].Zone; zone != "" {
			row.Timezone = zone
		}
		out[i] = MockHostCandidate{
			UserID: row.UserID, Handle: row.Handle, Card: summarize(decodeCV(row.ResumeStructured), row.Countries),
			Timezone: row.Timezone, Topics: row.Topics,
		}
	}
	return out, nil
}

func (r *QueriesRepository) MockAvailabilityFor(ctx context.Context, userIDs []int64) (map[int64][]slots.Rule, error) {
	schedules, err := r.avail().LoadMany(ctx, userIDs)
	out := make(map[int64][]slots.Rule, len(schedules))
	for id, sc := range schedules {
		out[id] = sc.Rules
	}
	return out, err
}

func (r *QueriesRepository) MockBusyFor(ctx context.Context, userIDs []int64, from, to time.Time) (map[int64][]slots.Interval, error) {
	all, err := r.avail().Busy(ctx, userIDs, from, to)
	out := make(map[int64][]slots.Interval, len(all))
	for id, b := range all {
		out[id] = availability.Intervals(b)
	}
	return out, err
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
