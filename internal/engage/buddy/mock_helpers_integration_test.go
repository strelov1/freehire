//go:build integration

package buddy_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/db"
)

// recordingNotifier is a MockNotifier that keeps what it was asked to send. failReminders
// makes that many reminder deliveries fail first.
type recordingNotifier struct {
	mu            sync.Mutex
	booked        []buddy.Mock
	cancelled     []buddy.Mock
	reminded      []string
	failReminders int
}

func (n *recordingNotifier) MockBooked(_ context.Context, m buddy.Mock) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.booked = append(n.booked, m)
	return nil
}

func (n *recordingNotifier) MockCancelled(_ context.Context, m buddy.Mock, _ int64) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.cancelled = append(n.cancelled, m)
	return nil
}

func (n *recordingNotifier) MockReminder(_ context.Context, m buddy.Mock, before time.Duration) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.failReminders > 0 {
		n.failReminders--
		return errors.New("mail server down")
	}
	n.reminded = append(n.reminded, fmt.Sprintf("%s:%v", m.ID, before))
	return nil
}

// mockSetup is setup plus a MockService over the same database with a recording notifier.
func mockSetup(t *testing.T) (*pgxpool.Pool, *buddy.Service, *buddy.MockService, *recordingNotifier) {
	t.Helper()
	pool, svc := setup(t)
	n := &recordingNotifier{}
	return pool, svc, buddy.NewMockService(svc, buddy.NewQueriesRepository(db.New(pool)), n), n
}

// joined seeds an account, joins it, and returns its id and handle.
func joined(t *testing.T, pool *pgxpool.Pool, svc *buddy.Service, email string) (int64, string) {
	t.Helper()
	id := seedUser(t, pool, email)
	m, err := svc.Join(context.Background(), id)
	if err != nil {
		t.Fatalf("join %s: %v", email, err)
	}
	return id, m.Handle
}

func weekly(t *testing.T, day time.Weekday, fromHour, toHour int) slots.Rule {
	t.Helper()
	start, err := slots.NewTimeOfDay(fromHour, 0)
	if err != nil {
		t.Fatal(err)
	}
	end, err := slots.NewTimeOfDay(toHour, 0)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := slots.NewWeeklyRule(day, start, end)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

// hostLink is the meeting link openAllWeek saves for a member.
func hostLink(userID int64) string {
	return "https://meet.example.test/room-" + strconv.FormatInt(userID, 10)
}

// openAllWeek saves settings in zone and offers every day 00:00-24:00, so a slot exists at
// every whole hour past the notice period. topics defaults to the whole vocabulary.
func openAllWeek(t *testing.T, mocks *buddy.MockService, userID int64, zone string, topics ...string) {
	t.Helper()
	ctx := context.Background()
	if len(topics) == 0 {
		topics = buddy.MockTopics
	}
	if _, err := mocks.SaveSettings(ctx, userID, buddy.MockSettings{
		Timezone: zone, MeetingURL: hostLink(userID), Topics: topics,
	}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	week := make([]slots.Rule, 0, 7)
	for d := time.Sunday; d <= time.Saturday; d++ {
		week = append(week, weekly(t, d, 0, 24))
	}
	if err := mocks.ReplaceWeekly(ctx, userID, week); err != nil {
		t.Fatalf("replace week: %v", err)
	}
}

// firstSlot is the earliest slot hostHandle offers viewerID in the next three days, in UTC.
func firstSlot(t *testing.T, mocks *buddy.MockService, viewerID int64, hostHandle string) time.Time {
	t.Helper()
	now := time.Now()
	res, err := mocks.OpenSlots(context.Background(), viewerID, hostHandle, now, now.AddDate(0, 0, 3), "UTC")
	if err != nil {
		t.Fatalf("open slots: %v", err)
	}
	if len(res.Slots) == 0 {
		t.Fatal("the host offers no slot")
	}
	return res.Slots[0].Start.UTC()
}

// seedMock writes a confirmed one-hour mock and both participant rows directly, for states
// a booking cannot reach: a mock already under way or long over, or one in the past.
func seedMock(t *testing.T, pool *pgxpool.Pool, host, guest int64, start time.Time) uuid.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := pool.QueryRow(context.Background(), `
		WITH m AS (
			INSERT INTO buddy_mocks (host_user_id, guest_user_id, starts_at, ends_at, topic, guest_timezone)
			VALUES ($1, $2, $3, $3::timestamptz + interval '1 hour', 'coding', 'UTC')
			RETURNING id, starts_at, ends_at
		), p AS (
			INSERT INTO buddy_mock_participants (mock_id, user_id, starts_at, ends_at)
			SELECT id, $1, starts_at, ends_at FROM m
			UNION ALL
			SELECT id, $2, starts_at, ends_at FROM m
		)
		SELECT id FROM m`, host, guest, start).Scan(&id); err != nil {
		t.Fatalf("seed mock: %v", err)
	}
	return uuid.UUID(id.Bytes)
}
