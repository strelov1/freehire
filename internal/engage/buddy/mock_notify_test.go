package buddy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
)

type captureSender struct{ sent []emailnotify.Message }

func (c *captureSender) Send(_ context.Context, m emailnotify.Message) error {
	c.sent = append(c.sent, m)
	return nil
}

func (c *captureSender) to(t *testing.T, addr string) emailnotify.Message {
	t.Helper()
	for _, m := range c.sent {
		if m.To == addr {
			return m
		}
	}
	t.Fatalf("nothing sent to %s (sent: %d)", addr, len(c.sent))
	return emailnotify.Message{}
}

// sampleMock is a system-design mock on Tuesday 10 November 2026, 18:00-19:00 UTC: 19:00
// for a host in Berlin and 03:00 on Wednesday for a guest in Tokyo.
func sampleMock() Mock {
	start := time.Date(2026, time.November, 10, 18, 0, 0, 0, time.UTC)
	return Mock{
		ID: uuid.MustParse("3f0e4b8a-1c2d-4e5f-8a9b-0c1d2e3f4a5b"), HostUserID: 1, GuestUserID: 2,
		HostHandle: "calm-otter-11", GuestHandle: "brave-lynx-22",
		StartsAt: start, EndsAt: start.Add(MockDuration), Topic: TopicSystemDesign, Status: MockConfirmed,
		HostTimezone: "Europe/Berlin", GuestTimezone: "Asia/Tokyo", MeetingURL: "https://meet.example.test/room",
		HostEmail: "host@example.test", GuestEmail: "guest@example.test",
	}
}

func notifier(c *captureSender) *MockMailNotifier {
	return NewMockMailNotifier(nil, c, "noreply@freehire.test", "https://freehire.test/")
}

func assertContains(t *testing.T, label, body string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("%s lacks %q:\n%s", label, w, body)
		}
	}
}

func assertLacks(t *testing.T, label, body string, leaks ...string) {
	t.Helper()
	for _, l := range leaks {
		if strings.Contains(body, l) {
			t.Errorf("%s carries %q:\n%s", label, l, body)
		}
	}
}

func TestBookedNoticeNamesTheHandleTopicAndRecipientsZone(t *testing.T) {
	c := &captureSender{}
	if err := notifier(c).MockBooked(context.Background(), sampleMock()); err != nil {
		t.Fatal(err)
	}
	host := c.to(t, "host@example.test")
	all := host.Subject + host.Text + host.HTML
	assertContains(t, "host mail", host.Text, "brave-lynx-22", "system design", "Tuesday 10 November 2026, 19:00", "Europe/Berlin",
		"https://meet.example.test/room", "https://freehire.test/my/buddies/mocks")
	assertLacks(t, "host mail", all, "guest@example.test", "Asia/Tokyo", "calm-otter-11")

	guest := c.to(t, "guest@example.test")
	assertContains(t, "guest mail", guest.Text, "calm-otter-11", "Wednesday 11 November 2026, 03:00", "Asia/Tokyo")
	assertLacks(t, "guest mail", guest.Subject+guest.Text+guest.HTML, "host@example.test", "Europe/Berlin")
}

func TestCancelNoticeGoesOnlyToTheOtherSide(t *testing.T) {
	c := &captureSender{}
	m := sampleMock()
	m.Status, m.CancelReason = MockCancelled, CancelByParticipant
	if err := notifier(c).MockCancelled(context.Background(), m, m.GuestUserID); err != nil {
		t.Fatal(err)
	}
	if len(c.sent) != 1 || c.sent[0].To != "host@example.test" {
		t.Fatalf("sent = %+v, want the host only", c.sent)
	}
	assertContains(t, "participant cancel", c.sent[0].Text, "brave-lynx-22 cancelled your system design mock interview.")
	assertLacks(t, "participant cancel", c.sent[0].Text, "https://meet.example.test/room")

	c = &captureSender{}
	m.CancelReason = CancelByLeave
	if err := notifier(c).MockCancelled(context.Background(), m, m.HostUserID); err != nil {
		t.Fatal(err)
	}
	if len(c.sent) != 1 || c.sent[0].To != "guest@example.test" {
		t.Fatalf("sent = %+v, want the guest only", c.sent)
	}
	// A leave or a block is told only as "cancelled", never why.
	assertContains(t, "leave cancel", c.sent[0].Text, "Your system design mock interview with calm-otter-11 was cancelled.")
	assertLacks(t, "leave cancel", c.sent[0].Text, "left", "block")
}

func TestReminderGoesToBothWithTheLink(t *testing.T) {
	c := &captureSender{}
	if err := notifier(c).MockReminder(context.Background(), sampleMock(), time.Hour); err != nil {
		t.Fatal(err)
	}
	if len(c.sent) != 2 {
		t.Fatalf("sent %d mails, want 2", len(c.sent))
	}
	assertContains(t, "host reminder", c.to(t, "host@example.test").Text, "starts in an hour", "brave-lynx-22", "https://meet.example.test/room")
	assertContains(t, "guest reminder", c.to(t, "guest@example.test").Text, "starts in an hour", "calm-otter-11")
}

func TestAnUnusableZoneFallsBackToUTCAndSaysSo(t *testing.T) {
	c := &captureSender{}
	m := sampleMock()
	m.GuestTimezone = "Local"
	if err := notifier(c).MockBooked(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	assertContains(t, "guest mail", c.to(t, "guest@example.test").Text, "Tuesday 10 November 2026, 18:00", "(UTC)")
}
