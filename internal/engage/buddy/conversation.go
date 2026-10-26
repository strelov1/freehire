package buddy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/engage/messaging"
)

// Invitation limits.
const (
	// MaxInvitationRunes bounds the first message of a conversation.
	MaxInvitationRunes = 500
	// DailyInvitationCap bounds the invitations one member may send in a rolling window.
	DailyInvitationCap = 10
	// invitationWindow is that rolling window.
	invitationWindow = 24 * time.Hour
)

// ErrRecipientUnavailable is an invitation to someone who is not an active member, is the
// inviter, or is blocked in either direction. The cases are deliberately not told apart
// (403).
var ErrRecipientUnavailable = errors.New("buddy: recipient is unavailable")

// InvitationLimitError is an invitation over the daily cap (429). RetryAt is when the
// oldest invitation in the window ages out, which is the first moment another is possible.
type InvitationLimitError struct{ RetryAt time.Time }

func (e *InvitationLimitError) Error() string {
	return fmt.Sprintf("buddy: invitation limit reached, next possible at %s", e.RetryAt.UTC().Format(time.RFC3339))
}

// pairKey is the messaging pair key of a buddy pair: one conversation per pair, whoever
// invites whom.
func pairKey(a, b int64) string {
	return fmt.Sprintf("%d:%d", min(a, b), max(a, b))
}

// Invite starts a buddy conversation with the member named by toHandle, holding body as
// its first message. What a buddy invitation adds to a plain messaging start is here: both
// sides active members, a first message of at most MaxInvitationRunes, and the daily cap.
//
// Membership is checked by a read before the start, not inside it as a block is: a leave
// committing in between leaves a pending invitation to someone who has left. Their Leave
// closed every conversation that existed when it ran, so this one stays pending until they
// rejoin, which is when they would see it anyway.
func (s *Service) Invite(ctx context.Context, fromUserID int64, toHandle, body string) (messaging.Conversation, error) {
	body = strings.TrimSpace(body)
	if n := utf8.RuneCountInString(body); n < 1 || n > MaxInvitationRunes {
		return messaging.Conversation{}, messaging.ErrInvalidMessage
	}
	if err := s.requireActive(ctx, fromUserID); err != nil {
		return messaging.Conversation{}, err
	}
	to, ok, err := s.repo.MemberByHandle(ctx, toHandle)
	if err != nil {
		return messaging.Conversation{}, err
	}
	if !ok || !to.Active || to.UserID == fromUserID {
		return messaging.Conversation{}, ErrRecipientUnavailable
	}
	blocked, err := s.repo.Blocked(ctx, fromUserID, to.UserID)
	if err != nil {
		return messaging.Conversation{}, err
	}
	if blocked {
		return messaging.Conversation{}, ErrRecipientUnavailable
	}
	key := pairKey(fromUserID, to.UserID)
	// The pair's existing conversation is checked before the cap, so a member who is
	// already talking to this one hears that, not that they are over their limit. A closed
	// one is no barrier: the start reopens it.
	existing, ok, err := s.msg.ConversationByPairKey(ctx, KindName, key)
	if err != nil {
		return messaging.Conversation{}, err
	}
	if ok && existing.Status != messaging.StatusClosed {
		return messaging.Conversation{}, messaging.ErrExists
	}
	// As with referral's daily cap this is a count followed by an insert, so concurrent
	// invitations from one member can slip a few past the cap. The cap is a spam brake,
	// not a quota; the one-conversation-per-pair key is what holds exactly.
	sent, oldest, err := s.msg.StartedSince(ctx, KindName, fromUserID, s.now().Add(-invitationWindow))
	if err != nil {
		return messaging.Conversation{}, err
	}
	if sent >= DailyInvitationCap {
		return messaging.Conversation{}, &InvitationLimitError{RetryAt: oldest.Add(invitationWindow)}
	}
	conv, err := s.msg.Start(ctx, messaging.StartRequest{
		Kind: KindName, StarterID: fromUserID, StarterPersona: PersonaName,
		RecipientID: to.UserID, RecipientPersona: PersonaName, PairKey: key, Body: body,
	})
	if errors.Is(err, messaging.ErrUnavailable) {
		// A block that landed after the read above: still not revealed.
		return messaging.Conversation{}, ErrRecipientUnavailable
	}
	return conv, err
}
