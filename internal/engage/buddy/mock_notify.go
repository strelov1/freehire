package buddy

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Notification-center kinds of a mock notice. user_notifications.kind is free text, so they
// need no migration.
const (
	notificationMockBooked    = "buddy_mock_booked"
	notificationMockCancelled = "buddy_mock_cancelled"
	notificationMockReminder  = "buddy_mock_reminder"
)

var _ MockNotifier = (*MockMailNotifier)(nil)

// MockMailNotifier delivers mock notices by email and records each in the notification
// center. Every notice names the other participant by handle only — never an account name
// or address — and states the time in its recipient's own zone: the host's from their
// settings, the guest's from the booking, falling back to UTC and saying so.
//
// Like mentorship's booking mail it bypasses the account-level notification rule: that
// rule governs what the system originates about a member's activity, not a commitment the
// member made and another member is holding an hour for.
//
// The mail is emailprefs.GroupEssential, as mentorship's booking mail is and for its
// reason: it is a commitment the member made and another member is holding an hour for,
// and an unsubscribe link on it would be an offer that cannot be honoured.
type MockMailNotifier struct {
	q      *db.Queries
	sender emailnotify.Sender
	from   string
	origin string
}

// NewMockMailNotifier builds a MockMailNotifier. A nil sender records notification-center
// rows only (a deployment with no mail transport); a nil q sends mail only.
func NewMockMailNotifier(q *db.Queries, sender emailnotify.Sender, from, origin string) *MockMailNotifier {
	return &MockMailNotifier{q: q, sender: sender, from: from, origin: strings.TrimRight(origin, "/")}
}

// mockRecipient is one participant as their own notice addresses them.
type mockRecipient struct {
	userID int64
	email  string
	zone   string
	host   bool
	// other is the OTHER participant's handle: the only thing a notice says about them.
	other string
}

func (m Mock) hostRecipient() mockRecipient {
	return mockRecipient{userID: m.HostUserID, email: m.HostEmail, zone: m.HostTimezone, host: true, other: m.GuestHandle}
}

func (m Mock) guestRecipient() mockRecipient {
	return mockRecipient{userID: m.GuestUserID, email: m.GuestEmail, zone: m.GuestTimezone, other: m.HostHandle}
}

// mockNotice is one rendered notice before it is addressed.
type mockNotice struct {
	kind    string
	subject string
	line    string
	// link is whether the host's meeting link belongs in it: yes for a booking and a
	// reminder, never for a cancellation.
	link bool
	// event names the occurrence in the dedup key, so a retried send records once.
	event string
}

func (n *MockMailNotifier) MockBooked(ctx context.Context, m Mock) error {
	topic := topicPhrase(m.Topic)
	return n.deliver(ctx, m, []mockRecipient{m.hostRecipient(), m.guestRecipient()}, func(r mockRecipient) mockNotice {
		line := fmt.Sprintf("Your %s mock interview with %s is booked.", topic, r.other)
		if r.host {
			line = fmt.Sprintf("%s booked a %s mock interview with you.", r.other, topic)
		}
		return mockNotice{kind: notificationMockBooked, subject: "Mock interview booked", line: line, link: true, event: "booked"}
	})
}

func (n *MockMailNotifier) MockCancelled(ctx context.Context, m Mock, actorID int64) error {
	to := m.hostRecipient()
	if actorID == m.HostUserID {
		to = m.guestRecipient()
	}
	topic := topicPhrase(m.Topic)
	return n.deliver(ctx, m, []mockRecipient{to}, func(r mockRecipient) mockNotice {
		// A leave or a block reads only as "cancelled": why is the other member's business.
		line := fmt.Sprintf("Your %s mock interview with %s was cancelled.", topic, r.other)
		if m.CancelReason == CancelByParticipant {
			line = fmt.Sprintf("%s cancelled your %s mock interview.", r.other, topic)
		}
		return mockNotice{kind: notificationMockCancelled, subject: "Mock interview cancelled", line: line, event: "cancelled"}
	})
}

func (n *MockMailNotifier) MockReminder(ctx context.Context, m Mock, before time.Duration) error {
	topic, lead := topicPhrase(m.Topic), leadPhrase(before)
	return n.deliver(ctx, m, []mockRecipient{m.hostRecipient(), m.guestRecipient()}, func(r mockRecipient) mockNotice {
		return mockNotice{
			kind:    notificationMockReminder,
			subject: "Your mock interview starts in " + lead,
			line:    fmt.Sprintf("Your %s mock interview with %s starts in %s.", topic, r.other, lead),
			link:    true,
			event:   fmt.Sprintf("reminder-%d", int(before/time.Minute)),
		}
	})
}

// deliver sends one notice per recipient and records each delivered one in the
// notification center, joining failures rather than stopping at the first: one
// unreachable mailbox must not cost the other participant their notice.
func (n *MockMailNotifier) deliver(ctx context.Context, m Mock, to []mockRecipient, render func(mockRecipient) mockNotice) error {
	var errs []error
	for _, r := range to {
		nt := render(r)
		when := mockWhen(m.StartsAt, m.EndsAt, r.zone)
		if n.sender != nil && r.email != "" {
			if err := n.sender.Send(ctx, emailnotify.Message{
				From: n.from, To: r.email, Subject: nt.subject,
				HTML: n.renderHTML(nt, when, m), Text: n.renderText(nt, when, m),
				Group: emailprefs.GroupEssential,
			}); err != nil {
				errs = append(errs, fmt.Errorf("buddy: %s notice to user %d: %w", nt.event, r.userID, err))
				continue
			}
		}
		n.record(ctx, r.userID, nt, when, m)
	}
	return errors.Join(errs...)
}

// record writes the notification-center row. Best-effort: a failure here never fails the
// delivery it records.
func (n *MockMailNotifier) record(ctx context.Context, userID int64, nt mockNotice, when string, m Mock) {
	if n.q == nil {
		return
	}
	if _, err := n.q.RecordNotification(ctx, db.RecordNotificationParams{
		UserID: userID, Kind: nt.kind, Title: nt.subject, Body: nt.line + " " + when,
		DedupKey: pgtype.Text{String: fmt.Sprintf("%s:%s:%s:%d", nt.kind, m.ID, nt.event, userID), Valid: true},
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("buddy: recording %s notice for user %d: %v", nt.kind, userID, err)
	}
}

func (n *MockMailNotifier) cabinetURL() string { return n.origin + "/my/buddies/mocks" }

func (n *MockMailNotifier) renderText(nt mockNotice, when string, m Mock) string {
	lines := []string{nt.line, "", when}
	if nt.link && m.MeetingURL != "" {
		lines = append(lines, "", "Join: "+m.MeetingURL)
	}
	lines = append(lines, "", "Your mock interviews: "+n.cabinetURL())
	return strings.Join(lines, "\n")
}

func (n *MockMailNotifier) renderHTML(nt mockNotice, when string, m Mock) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<p>%s</p><p><strong>%s</strong></p>", html.EscapeString(nt.line), html.EscapeString(when))
	if nt.link && m.MeetingURL != "" {
		fmt.Fprintf(&b, `<p><a href="%s">Join the call</a></p>`, html.EscapeString(m.MeetingURL))
	}
	fmt.Fprintf(&b, `<p><a href="%s">Your mock interviews</a></p>`, html.EscapeString(n.cabinetURL()))
	return b.String()
}

// mockWhen states a mock in one recipient's zone, naming the zone. A zone that is not an
// IANA place falls back to UTC AND SAYS SO — never to the other participant's zone, since a
// time labelled with a zone that is not the reader's is undetectably wrong.
func mockWhen(start, end time.Time, zoneName string) string {
	zone, ok := ianaZone(zoneName)
	if !ok {
		zone, zoneName = time.UTC, "UTC"
	}
	return fmt.Sprintf("%s – %s (%s)",
		start.In(zone).Format("Monday 2 January 2006, 15:04"), end.In(zone).Format("15:04"), zoneName)
}

// topicPhrase is a topic as it reads inside a sentence.
func topicPhrase(topic string) string {
	switch topic {
	case TopicCoding:
		return "coding"
	case TopicSystemDesign:
		return "system design"
	case TopicBehavioral:
		return "behavioral"
	}
	return topic
}

// leadPhrase renders a reminder offset as somebody would say it.
func leadPhrase(d time.Duration) string {
	switch {
	case d == time.Hour:
		return "an hour"
	case d >= time.Hour:
		return fmt.Sprintf("%d hours", int(d/time.Hour))
	default:
		return fmt.Sprintf("%d minutes", int(d/time.Minute))
	}
}
