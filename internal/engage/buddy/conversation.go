package buddy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/engage/messaging"
)

// MaxInvitationRunes bounds the first message of a conversation.
const MaxInvitationRunes = 500

// ErrRecipientUnavailable is an invitation to someone who is not an active member, is the
// inviter, or is blocked in either direction. The cases are deliberately not told apart
// (403).
var ErrRecipientUnavailable = errors.New("buddy: recipient is unavailable")

// pairKey is the messaging pair key of a buddy pair: one conversation per pair, whoever
// invites whom.
func pairKey(a, b int64) string {
	return fmt.Sprintf("%d:%d", min(a, b), max(a, b))
}

// Invite starts a buddy conversation with the member named by toHandle, holding body as
// its first message. What a buddy invitation adds to a plain messaging start is here: both
// sides active members and a first message of at most MaxInvitationRunes.
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
	// An existing live conversation is a conflict. A closed one is reopened by Start.
	existing, ok, err := s.msg.ConversationByPairKey(ctx, KindName, key)
	if err != nil {
		return messaging.Conversation{}, err
	}
	if ok && existing.Status != messaging.StatusClosed {
		return messaging.Conversation{}, messaging.ErrExists
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
