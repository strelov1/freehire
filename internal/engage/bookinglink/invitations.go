package bookinglink

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/application/calsync"
	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Invitation is one reserved booking that still needs its Google Calendar invitation.
type Invitation struct {
	BookingID int64
	Event     calsync.InterviewEvent

	refreshTokenEnc string
}

type InvitationStore interface {
	Claim(ctx context.Context) (Invitation, bool, error)
	Record(ctx context.Context, invitation Invitation, meetURL string) error
}

type InvitationSender interface {
	Invite(ctx context.Context, invitation Invitation) (string, error)
}

// RunInvitations delivers due calendar invitations. A failed call leaves the claim
// eligible for retry after its lease. Pending Meet provisioning is an ordinary retry.
func RunInvitations(ctx context.Context, store InvitationStore, sender InvitationSender, limit int) error {
	var failures error
	for range limit {
		invitation, ok, err := store.Claim(ctx)
		if err != nil || !ok {
			return errors.Join(failures, err)
		}
		meetURL, err := sender.Invite(ctx, invitation)
		if errors.Is(err, calsync.ErrConferencePending) {
			continue
		}
		if err == nil {
			err = store.Record(ctx, invitation, meetURL)
		}
		if err != nil {
			failures = errors.Join(failures, fmt.Errorf("calendar invitation for booking %d: %w", invitation.BookingID, err))
		}
	}
	return failures
}

type InvitationQueries struct {
	q              *db.Queries
	pilotUserID    int64
	pilotRecruiter string
}

func NewInvitationQueries(q *db.Queries, pilotUserID int64, pilotRecruiter string) *InvitationQueries {
	return &InvitationQueries{q: q, pilotUserID: pilotUserID, pilotRecruiter: pilotRecruiter}
}

func (s *InvitationQueries) Claim(ctx context.Context) (Invitation, bool, error) {
	row, err := s.q.ClaimBookingCalendarInvitation(ctx, db.ClaimBookingCalendarInvitationParams{
		CalendarWriteScope: gmailsync.CalendarEventsScope, PilotUserID: s.pilotUserID, PilotRecruiter: s.pilotRecruiter,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, false, nil
	}
	if err != nil {
		return Invitation{}, false, err
	}
	if !row.CalendarEventKey.Valid {
		return Invitation{}, false, fmt.Errorf("booking %d has no calendar event key", row.ID)
	}
	id := "freehire" + strings.ReplaceAll(uuid.UUID(row.CalendarEventKey.Bytes).String(), "-", "")
	return Invitation{
		BookingID: row.ID,
		Event: calsync.InterviewEvent{
			ID: id, Attendee: row.RecruiterEmail, StartsAt: row.StartsAt.Time, EndsAt: row.EndsAt.Time,
		},
		refreshTokenEnc: row.RefreshTokenEnc,
	}, true, nil
}

func (s *InvitationQueries) Record(ctx context.Context, in Invitation, meetURL string) error {
	updated, err := s.q.RecordBookingCalendarInvitation(ctx, db.RecordBookingCalendarInvitationParams{
		BookingID: in.BookingID, ProviderEventID: in.Event.ID, JoinUrl: meetURL,
	})
	if err == nil && updated != 1 {
		return errors.New("booking invitation no longer pending")
	}
	return err
}

func (c *GoogleCalendar) Invite(ctx context.Context, in Invitation) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	refresh, err := c.cipher.Decrypt(in.refreshTokenEnc)
	if err != nil {
		return "", fmt.Errorf("decrypt calendar grant: %w", err)
	}
	return calsync.NewInterviewWriter(c.client(ctx, refresh)).Invite(ctx, in.Event)
}
