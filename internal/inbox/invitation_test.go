package inbox

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// The rehearsal opens on what the employer said about the interview, so the
// invitation reaches it as a message, not as a row.
func TestInterviewInvitationReadsTheMessage(t *testing.T) {
	received := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	q := &fakeQueries{invitation: db.GetInterviewInvitationRow{
		ID: 7, FromAddr: "talent@acme.test", FromName: "Acme Talent",
		Subject:    "Tech round with the lead",
		BodyText:   "45 minutes, focused on distributed systems.",
		ReceivedAt: pgtype.Timestamptz{Time: received, Valid: true},
	}}
	s := New(q, nil)

	msg, err := s.InterviewInvitation(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf("InterviewInvitation: %v", err)
	}
	if msg.Subject != "Tech round with the lead" {
		t.Errorf("subject = %q, want the invitation's", msg.Subject)
	}
	if !strings.Contains(msg.BodyText, "distributed systems") {
		t.Errorf("body = %q, want the employer's own description", msg.BodyText)
	}
	if !msg.ReceivedAt.Equal(received) {
		t.Errorf("received = %v, want %v", msg.ReceivedAt, received)
	}
}

// Most interview invitations come from an ATS, and ATS senders (Ashby, Greenhouse,
// Gem) routinely mail HTML with no text/plain part — so body_text is empty exactly
// where this feature needs a body. The readable-body fallback the classifier already
// uses has to apply here too, or the rehearsal opens on an invitation it cannot read.
func TestInterviewInvitationReadsAnHTMLOnlyBody(t *testing.T) {
	q := &fakeQueries{invitation: db.GetInterviewInvitationRow{
		ID: 8, Subject: "Interview invitation",
		BodyText: "",
		BodyHtml: "<html><body><p>We would like to invite you to a 45 minute call.</p></body></html>",
	}}
	s := New(q, nil)

	msg, err := s.InterviewInvitation(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf("InterviewInvitation: %v", err)
	}
	if !strings.Contains(msg.BodyText, "45 minute call") {
		t.Errorf("body = %q, want the HTML rendered down to text", msg.BodyText)
	}
}

// No invitation is an ordinary answer, not a failure: plenty of interviews are
// arranged outside the mailbox we can see.
func TestInterviewInvitationReportsWhenThereIsNone(t *testing.T) {
	q := &fakeQueries{invitationErr: errNoRows}
	s := New(q, nil)

	_, err := s.InterviewInvitation(context.Background(), 1, 42)
	if !errors.Is(err, errNoRows) {
		t.Fatalf("error = %v, want the store's no-rows error passed through", err)
	}
}
