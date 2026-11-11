//go:build integration

package buddy_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/platform/db"
)

type mailbox struct {
	mu   sync.Mutex
	sent []emailnotify.Message
}

func (b *mailbox) Send(_ context.Context, m emailnotify.Message) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent = append(b.sent, m)
	return nil
}

func TestBookingNoticesNameHandlesOnly(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	q := db.New(pool)
	box := &mailbox{}
	mocks := buddy.NewMockService(svc, buddy.NewQueriesRepository(q, pool),
		buddy.NewMockMailNotifier(q, box, "noreply@freehire.test", "https://freehire.test"))
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, guestHandle := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "Europe/Berlin")

	if _, err := book(mocks, guest, hostHandle, firstSlot(t, mocks, guest, hostHandle), buddy.TopicSystemDesign); err != nil {
		t.Fatal(err)
	}
	if len(box.sent) != 2 {
		t.Fatalf("sent %d mails, want one per participant", len(box.sent))
	}
	for _, m := range box.sent {
		other := "guest@example.test"
		if m.To == "guest@example.test" {
			other = "host@example.test"
		}
		if strings.Contains(m.Subject+m.Text+m.HTML, other) {
			t.Fatalf("mail to %s carries %s", m.To, other)
		}
	}
	var kind, body string
	if err := pool.QueryRow(ctx, `SELECT kind, body FROM user_notifications WHERE user_id = $1`, host).Scan(&kind, &body); err != nil {
		t.Fatal(err)
	}
	if kind != "buddy_mock_booked" || !strings.Contains(body, guestHandle) || strings.Contains(body, "guest@example.test") {
		t.Fatalf("host notification = %s %q", kind, body)
	}
}

type selectivelyFailingMailbox struct{ mailbox }

func (b *selectivelyFailingMailbox) Send(ctx context.Context, m emailnotify.Message) error {
	if m.To == "host@example.test" {
		return errors.New("mail unavailable")
	}
	return b.mailbox.Send(ctx, m)
}

func TestFailedMockMailCreatesNoNotificationForThatRecipient(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	host, handle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")
	m, err := book(mocks, guest, handle, firstSlot(t, mocks, guest, handle), buddy.TopicCoding)
	if err != nil {
		t.Fatal(err)
	}
	m.HostEmail, m.GuestEmail = "host@example.test", "guest@example.test"
	box := &selectivelyFailingMailbox{}
	n := buddy.NewMockMailNotifier(db.New(pool), box, "noreply@freehire.test", "https://freehire.test")
	if err := n.MockBooked(ctx, m); err == nil {
		t.Fatal("failed email should return an error")
	}
	var failedCount, deliveredCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE user_id = $1), count(*) FILTER (WHERE user_id = $2) FROM user_notifications WHERE kind = 'buddy_mock_booked'`, host, guest).Scan(&failedCount, &deliveredCount); err != nil {
		t.Fatal(err)
	}
	if failedCount != 0 || deliveredCount != 1 {
		t.Fatalf("notifications: failed recipient %d, delivered recipient %d; want 0 and 1", failedCount, deliveredCount)
	}
}
