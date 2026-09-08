package mentorship

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
)

// sentMail is one delivery the fake transport captured.
type sentMail struct {
	to          string
	subject     string
	html        string
	text        string
	attachments []emailnotify.Attachment
}

type fakeSender struct {
	sent []sentMail
	err  error
	// failFor makes exactly one recipient's delivery fail, which is how the "one party
	// unreachable must not cost the other their message" rule is tested.
	failFor string
}

func (f *fakeSender) Send(_ context.Context, m emailnotify.Message) error {
	to, subject, htmlBody, textBody, attachments := m.To, m.Subject, m.HTML, m.Text, m.Attachments
	if f.failFor != "" && to == f.failFor {
		return errors.New("mailbox full")
	}
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, sentMail{to: to, subject: subject, html: htmlBody, text: textBody, attachments: attachments})
	return nil
}

func crossZoneBooking(t *testing.T) Booking {
	t.Helper()
	booking := sampleBooking(t)
	booking.MentorEmail = "mentor@example.test"
	booking.SeekerEmail = "seeker@example.test"
	booking.MentorHeadline = "Senior Backend Engineer"
	return booking
}

func (f *fakeSender) to(t *testing.T, address string) sentMail {
	t.Helper()
	for _, m := range f.sent {
		if m.to == address {
			return m
		}
	}
	t.Fatalf("nothing was sent to %s (sent: %d)", address, len(f.sent))
	return sentMail{}
}

// The rule this whole notifier exists for: each party reads the session in THEIR zone.
// One message naming one time makes at least one of them do arithmetic to find out when
// their meeting is, and getting that wrong means missing it.
func TestBothPartiesAreToldTheTimeInTheirOwnZone(t *testing.T) {
	sender := &fakeSender{}
	notifier := NewMailNotifier(sender, "mentors@example.test", "https://freehire.me/my/sessions")

	if err := notifier.BookingConfirmed(context.Background(), crossZoneBooking(t)); err != nil {
		t.Fatalf("BookingConfirmed: %v", err)
	}
	if len(sender.sent) != 2 {
		t.Fatalf("%d messages sent, want 2", len(sender.sent))
	}

	// 16:00 UTC is 18:00 in Berlin and 01:00 the next day in Tokyo.
	mentorMail := sender.to(t, "mentor@example.test")
	if !strings.Contains(mentorMail.text, "18:00") || !strings.Contains(mentorMail.text, "Europe/Berlin") {
		t.Errorf("the mentor was not told 18:00 Europe/Berlin:\n%s", mentorMail.text)
	}
	seekerMail := sender.to(t, "seeker@example.test")
	if !strings.Contains(seekerMail.text, "01:00") || !strings.Contains(seekerMail.text, "Asia/Tokyo") {
		t.Errorf("the seeker was not told 01:00 Asia/Tokyo:\n%s", seekerMail.text)
	}
	// And the same absolute session: the seeker's is the following day.
	if !strings.Contains(seekerMail.text, "9 September") {
		t.Errorf("the seeker's date is not the 9th:\n%s", seekerMail.text)
	}
}

func TestAConfirmationCarriesACalendarInvitation(t *testing.T) {
	sender := &fakeSender{}
	notifier := NewMailNotifier(sender, "mentors@example.test", "")

	if err := notifier.BookingConfirmed(context.Background(), crossZoneBooking(t)); err != nil {
		t.Fatalf("BookingConfirmed: %v", err)
	}

	for _, m := range sender.sent {
		if len(m.attachments) != 1 {
			t.Fatalf("%s got %d attachments, want 1", m.to, len(m.attachments))
		}
		a := m.attachments[0]
		if a.Filename != "invite.ics" {
			t.Errorf("attachment is %q, want invite.ics", a.Filename)
		}
		// The method parameter and the METHOD inside must agree, or a client shows a
		// file to download instead of an "add to calendar" button.
		if !strings.Contains(a.ContentType, "method=REQUEST") {
			t.Errorf("content type = %q, want method=REQUEST", a.ContentType)
		}
		if !strings.Contains(string(a.Content), "METHOD:REQUEST") {
			t.Error("the invitation body is not a REQUEST")
		}
	}
}

// A cancellation must carry a CANCEL with the SAME UID, or the event stays in both
// calendars and two people turn up to a meeting that is off.
func TestACancellationRemovesTheEventRatherThanAddingOne(t *testing.T) {
	sender := &fakeSender{}
	notifier := NewMailNotifier(sender, "mentors@example.test", "")
	booking := crossZoneBooking(t)

	if err := notifier.BookingConfirmed(context.Background(), booking); err != nil {
		t.Fatalf("BookingConfirmed: %v", err)
	}
	confirmUID := uidOf(t, string(sender.to(t, "seeker@example.test").attachments[0].Content))

	sender.sent = nil
	if err := notifier.BookingCancelled(context.Background(), booking, CancelledByMentor, "ill"); err != nil {
		t.Fatalf("BookingCancelled: %v", err)
	}

	cancelMail := sender.to(t, "seeker@example.test")
	invite := string(cancelMail.attachments[0].Content)
	if !strings.Contains(invite, "METHOD:CANCEL") {
		t.Error("the cancellation is not a CANCEL")
	}
	if uidOf(t, invite) != confirmUID {
		t.Error("the cancellation carries a different UID — the event would stay in the calendar")
	}
	if !strings.Contains(cancelMail.text, "cancelled by the mentor") {
		t.Errorf("the message does not say who cancelled:\n%s", cancelMail.text)
	}
	if !strings.Contains(cancelMail.text, "ill") {
		t.Errorf("the reason was dropped:\n%s", cancelMail.text)
	}
}

// A reminder attaches nothing: both parties already hold the invitation, and a second
// REQUEST with the same UID is noise at best and a duplicate event at worst.
func TestAReminderCarriesNoInvitation(t *testing.T) {
	sender := &fakeSender{}
	notifier := NewMailNotifier(sender, "mentors@example.test", "")

	if err := notifier.BookingReminder(context.Background(), crossZoneBooking(t), 24*time.Hour); err != nil {
		t.Fatalf("BookingReminder: %v", err)
	}

	for _, m := range sender.sent {
		if len(m.attachments) != 0 {
			t.Errorf("%s got %d attachments on a reminder, want none", m.to, len(m.attachments))
		}
		if !strings.Contains(m.subject, "24 hours") {
			t.Errorf("subject = %q, want it to name the lead time", m.subject)
		}
	}
}

// One unreachable party must not cost the other their message. errors.Join reports both
// outcomes; the caller logs and carries on either way.
func TestOnePartyFailingStillReachesTheOther(t *testing.T) {
	sender := &fakeSender{failFor: "mentor@example.test"}
	notifier := NewMailNotifier(sender, "mentors@example.test", "")

	err := notifier.BookingConfirmed(context.Background(), crossZoneBooking(t))
	if err == nil {
		t.Error("a failed delivery was not reported")
	}
	if len(sender.sent) != 1 || sender.sent[0].to != "seeker@example.test" {
		t.Errorf("the seeker did not get their confirmation: %d sent", len(sender.sent))
	}
}

// A booking whose party has no address on it — which is what a partially-populated row
// looks like — must not send to an empty address or panic.
func TestAMissingAddressIsSkippedNotSentTo(t *testing.T) {
	sender := &fakeSender{}
	notifier := NewMailNotifier(sender, "mentors@example.test", "")
	booking := crossZoneBooking(t)
	booking.MentorEmail = ""

	if err := notifier.BookingConfirmed(context.Background(), booking); err != nil {
		t.Fatalf("BookingConfirmed: %v", err)
	}
	if len(sender.sent) != 1 || sender.sent[0].to != "seeker@example.test" {
		t.Errorf("sent %d messages, want only the seeker's", len(sender.sent))
	}
}

// A zone that does not load falls back to UTC and SAYS so, never to the other party's:
// a time labelled with somebody else's zone is undetectably wrong.
func TestAnUnresolvableZoneFallsBackToUTCAndSaysSo(t *testing.T) {
	sender := &fakeSender{}
	notifier := NewMailNotifier(sender, "mentors@example.test", "")
	booking := crossZoneBooking(t)
	booking.SeekerTimezone = "Mars/Olympus_Mons"

	if err := notifier.BookingConfirmed(context.Background(), booking); err != nil {
		t.Fatalf("BookingConfirmed: %v", err)
	}

	seekerMail := sender.to(t, "seeker@example.test")
	if !strings.Contains(seekerMail.text, "(UTC)") {
		t.Errorf("the fallback does not name UTC:\n%s", seekerMail.text)
	}
	if strings.Contains(seekerMail.text, "Berlin") {
		t.Errorf("the seeker was shown the MENTOR's zone:\n%s", seekerMail.text)
	}
	if !strings.Contains(seekerMail.text, "16:00") {
		t.Errorf("the UTC fallback does not show the UTC time:\n%s", seekerMail.text)
	}
}

// The HTML body is assembled by hand, so anything a mentor or seeker typed has to be
// escaped — a headline containing a tag would otherwise be markup in somebody's inbox.
func TestUserSuppliedTextIsEscapedInTheHTMLBody(t *testing.T) {
	sender := &fakeSender{}
	notifier := NewMailNotifier(sender, "mentors@example.test", "")
	booking := crossZoneBooking(t)
	booking.MentorHeadline = `Engineer <script>alert("x")</script>`

	if err := notifier.BookingConfirmed(context.Background(), booking); err != nil {
		t.Fatalf("BookingConfirmed: %v", err)
	}

	body := sender.to(t, "seeker@example.test").html
	if strings.Contains(body, "<script>") {
		t.Errorf("a headline reached the HTML body unescaped:\n%s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("the headline was not escaped:\n%s", body)
	}
}

func TestANotifierWithNoTransportIsAQuietNoOp(t *testing.T) {
	notifier := NewMailNotifier(nil, "mentors@example.test", "")
	booking := crossZoneBooking(t)

	if err := notifier.BookingConfirmed(context.Background(), booking); err != nil {
		t.Errorf("BookingConfirmed: %v", err)
	}
	if err := notifier.BookingCancelled(context.Background(), booking, CancelledBySeeker, ""); err != nil {
		t.Errorf("BookingCancelled: %v", err)
	}
	if err := notifier.BookingReminder(context.Background(), booking, time.Hour); err != nil {
		t.Errorf("BookingReminder: %v", err)
	}
}

func TestReminderLeadTimesReadLikeSentences(t *testing.T) {
	for _, tc := range []struct {
		in   time.Duration
		want string
	}{
		{24 * time.Hour, "24 hours"},
		{48 * time.Hour, "2 days"},
		{time.Hour, "an hour"},
		{3 * time.Hour, "3 hours"},
		{15 * time.Minute, "15 minutes"},
	} {
		if got := humaniseLead(tc.in); got != tc.want {
			t.Errorf("humaniseLead(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
