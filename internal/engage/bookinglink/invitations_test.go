package bookinglink

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/application/calsync"
)

type invitationTestStore struct {
	queued   []Invitation
	recorded map[int64]string
}

func (s *invitationTestStore) Claim(context.Context) (Invitation, bool, error) {
	if len(s.queued) == 0 {
		return Invitation{}, false, nil
	}
	in := s.queued[0]
	s.queued = s.queued[1:]
	return in, true, nil
}

func (s *invitationTestStore) Record(_ context.Context, in Invitation, meetURL string) error {
	s.recorded[in.BookingID] = meetURL
	return nil
}

type invitationSenderFunc func(context.Context, Invitation) (string, error)

func (f invitationSenderFunc) Invite(ctx context.Context, in Invitation) (string, error) {
	return f(ctx, in)
}

func TestPendingCalendarInvitationDoesNotPreventOtherInvitations(t *testing.T) {
	store := &invitationTestStore{
		queued:   []Invitation{{BookingID: 1}, {BookingID: 2}, {BookingID: 3}},
		recorded: map[int64]string{},
	}
	const meet = "https://meet.google.com/abc-defg-hij"
	sender := invitationSenderFunc(func(_ context.Context, in Invitation) (string, error) {
		switch in.BookingID {
		case 1:
			return "", calsync.ErrConferencePending
		case 2:
			return "", errors.New("calendar offline")
		default:
			return meet, nil
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := RunInvitations(ctx, store, sender, 3); err == nil {
		t.Fatal("the calendar failure must reach the caller")
	}
	if len(store.recorded) != 1 || store.recorded[3] != meet {
		t.Fatalf("recorded invitations = %v, want only the successful one", store.recorded)
	}
}
