package mentorship

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

// Compile-time proof that MailNotifier satisfies Notifier.
var _ Notifier = (*MailNotifier)(nil)

// MailNotifier delivers the three transactional messages a booking produces, each to both
// parties, each rendered in THAT recipient's own timezone.
//
// Each party sees the time in their own zone, never the other's or UTC, so nobody does
// arithmetic to find their meeting.
//
// These messages deliberately bypass the account-level notification rule, which governs
// what the system originates, not a commitment a second person is holding an hour for.
// For the same reason they are emailprefs.GroupEssential and carry no unsubscribe link:
// they are transactional, and unsubscribing would not cancel the appointment.
type MailNotifier struct {
	sender emailnotify.Sender
	from   string
	// cabinetURL is where a recipient goes to see or cancel the session.
	cabinetURL string
}

// NewMailNotifier builds a MailNotifier. With no mail transport the caller passes a nil
// Config.Notifier instead.
func NewMailNotifier(sender emailnotify.Sender, from, cabinetURL string) *MailNotifier {
	return &MailNotifier{sender: sender, from: from, cabinetURL: cabinetURL}
}

// BookingConfirmed tells both parties the session is on, with a calendar invitation each.
func (n *MailNotifier) BookingConfirmed(ctx context.Context, b Booking) error {
	invite := Invite(b, InviteOptions{
		Organizer:     n.from,
		OrganizerName: inviteOrganizerName,
		Summary:       sessionSummary(b),
	})

	return n.deliverToBoth(ctx, b, func(r recipient) message {
		return message{
			subject: "Your mentorship session is confirmed",
			lead:    fmt.Sprintf("%s is confirmed.", sessionSummary(b)),
			when:    r.when,
			invite:  invite,
			method:  "REQUEST",
		}
	})
}

// BookingCancelled tells both parties it is off. The invitation is re-issued as a CANCEL
// with the same UID, which removes the event from their calendar.
func (n *MailNotifier) BookingCancelled(ctx context.Context, b Booking, by CancelledBy, reason string) error {
	cancelled := b
	cancelled.Status = BookingCancelled
	invite := Invite(cancelled, InviteOptions{
		Organizer:     n.from,
		OrganizerName: inviteOrganizerName,
		Summary:       sessionSummary(b),
	})

	lead := fmt.Sprintf("%s has been cancelled by the %s.", sessionSummary(b), by)
	if strings.TrimSpace(reason) != "" {
		lead += " Reason: " + reason
	}

	return n.deliverToBoth(ctx, b, func(r recipient) message {
		return message{
			subject: "Your mentorship session was cancelled",
			lead:    lead,
			when:    r.when,
			invite:  invite,
			method:  "CANCEL",
		}
	})
}

// BookingReminder nudges both parties before the session. No invitation is attached: a
// second REQUEST with the same UID is noise or a duplicate event.
func (n *MailNotifier) BookingReminder(ctx context.Context, b Booking, before time.Duration) error {
	return n.deliverToBoth(ctx, b, func(r recipient) message {
		return message{
			subject: fmt.Sprintf("Your mentorship session starts in %s", humaniseLead(before)),
			lead: fmt.Sprintf("%s starts in %s.",
				sessionSummary(b), humaniseLead(before)),
			when: r.when,
		}
	})
}

// inviteOrganizerName is the display name on the calendar invitation.
const inviteOrganizerName = "freehire mentorship"

// recipient is one side of a session, with the session time already rendered in their own
// zone.
type recipient struct {
	email string
	when  string
}

// message is one rendered notification, before it is addressed.
type message struct {
	subject string
	lead    string
	when    string
	invite  string
	method  string
}

// deliverToBoth sends one message to the mentor and one to the seeker, joining the two
// failures rather than stopping at the first: a mentor who cannot be reached must not
// cost the seeker their confirmation.
func (n *MailNotifier) deliverToBoth(ctx context.Context, b Booking, render func(recipient) message) error {
	if n.sender == nil {
		return nil
	}

	var errs []error
	for _, r := range []recipient{
		{email: b.MentorEmail, when: renderWhen(b, b.MentorTimezone)},
		{email: b.SeekerEmail, when: renderWhen(b, b.SeekerTimezone)},
	} {
		if r.email == "" {
			continue
		}
		if err := n.send(ctx, r, render(r), b); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (n *MailNotifier) send(ctx context.Context, to recipient, m message, b Booking) error {
	var attachments []emailnotify.Attachment
	if m.invite != "" {
		attachments = append(attachments, emailnotify.Attachment{
			Filename: "invite.ics",
			// The method parameter is what makes a client offer "add to calendar" rather
			// than showing a file to download, and it must match the METHOD inside.
			ContentType: "text/calendar; charset=utf-8; method=" + m.method,
			Content:     []byte(m.invite),
		})
	}

	return n.sender.Send(ctx, emailnotify.Message{
		From:        n.from,
		To:          to.email,
		Subject:     m.subject,
		HTML:        n.renderHTML(m, b),
		Text:        n.renderText(m, b),
		Group:       emailprefs.GroupEssential,
		Attachments: attachments,
	})
}

func (n *MailNotifier) renderHTML(m message, b Booking) string {
	var body strings.Builder
	fmt.Fprintf(&body, "<p>%s</p>", html.EscapeString(m.lead))
	fmt.Fprintf(&body, "<p><strong>%s</strong></p>", html.EscapeString(m.when))
	if b.MeetingURL != "" && b.Status == BookingConfirmed {
		link := html.EscapeString(b.MeetingURL)
		fmt.Fprintf(&body, `<p><a href="%s">Join the call</a></p>`, link)
	}
	if n.cabinetURL != "" {
		fmt.Fprintf(&body, `<p><a href="%s">Your sessions</a></p>`, html.EscapeString(n.cabinetURL))
	}
	return body.String()
}

func (n *MailNotifier) renderText(m message, b Booking) string {
	lines := []string{m.lead, "", m.when}
	if b.MeetingURL != "" && b.Status == BookingConfirmed {
		lines = append(lines, "", "Join: "+b.MeetingURL)
	}
	if n.cabinetURL != "" {
		lines = append(lines, "", "Your sessions: "+n.cabinetURL)
	}
	return strings.Join(lines, "\n")
}

// renderWhen states the session in one recipient's zone, naming the zone. A zone that does
// not load falls back to UTC and says so, never to the other party's: a time labelled
// with the wrong zone is undetectably wrong.
func renderWhen(b Booking, zoneName string) string {
	zone, err := time.LoadLocation(zoneName)
	if err != nil || zoneName == "" {
		zone, zoneName = time.UTC, "UTC"
	}
	local := b.StartsAt.In(zone)
	return fmt.Sprintf("%s – %s (%s)",
		local.Format("Monday 2 January 2006, 15:04"),
		b.EndsAt.In(zone).Format("15:04"),
		zoneName)
}

// sessionSummary names the session the way both parties would recognise it.
func sessionSummary(b Booking) string {
	if b.MentorHeadline != "" {
		return "Mentorship session with " + b.MentorHeadline
	}
	return defaultInviteSummary
}

// humaniseLead renders a reminder's offset as somebody would say it.
func humaniseLead(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		days := int(d / (24 * time.Hour))
		if days == 1 {
			return "24 hours"
		}
		return fmt.Sprintf("%d days", days)
	case d >= time.Hour:
		hours := int(d / time.Hour)
		if hours == 1 {
			return "an hour"
		}
		return fmt.Sprintf("%d hours", hours)
	default:
		return fmt.Sprintf("%d minutes", int(d/time.Minute))
	}
}
