//go:build integration

package buddy_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

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
