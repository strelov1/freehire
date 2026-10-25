package buddy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
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

// Conversation statuses as stored.
const (
	StatusPending  = "pending"
	StatusOpen     = "open"
	StatusDeclined = "declined"
	StatusClosed   = "closed"
)

var (
	// ErrInvalidMessage is a message that is empty or too long (400).
	ErrInvalidMessage = errors.New("buddy: message is empty or too long")
	// ErrRecipientUnavailable is an invitation to someone who is not an active member, is
	// the inviter, or is blocked in either direction. The cases are deliberately not told
	// apart (403).
	ErrRecipientUnavailable = errors.New("buddy: recipient is unavailable")
	// ErrConversationExists is an invitation to a member the inviter already has a
	// conversation with, in any status (409).
	ErrConversationExists = errors.New("buddy: a conversation with this member already exists")
	// ErrConversationState is an answer to a conversation that is not pending, or by the
	// member who sent the invitation (409).
	ErrConversationState = errors.New("buddy: conversation cannot be answered")
)

// InvitationLimitError is an invitation over the daily cap (429). RetryAt is when the
// oldest invitation in the window ages out, which is the first moment another is possible.
type InvitationLimitError struct{ RetryAt time.Time }

func (e *InvitationLimitError) Error() string {
	return fmt.Sprintf("buddy: invitation limit reached, next possible at %s", e.RetryAt.UTC().Format(time.RFC3339))
}

// ConversationRow is a stored conversation.
type ConversationRow struct {
	ID          int64
	UserLow     int64
	UserHigh    int64
	InitiatorID int64
	Status      string
	CreatedAt   time.Time
	AcceptedAt  *time.Time
}

// Other is the participant that is not userID.
func (c ConversationRow) Other(userID int64) int64 {
	if c.UserLow == userID {
		return c.UserHigh
	}
	return c.UserLow
}

// Conversation is a conversation as one of its members sees it.
type Conversation struct {
	ID          int64
	OtherHandle string
	Status      string
	// Initiator is true when the viewer sent the invitation.
	Initiator  bool
	CreatedAt  time.Time
	AcceptedAt *time.Time
}

func (s *Service) conversationFor(ctx context.Context, userID int64, row ConversationRow) (Conversation, error) {
	// The other side's row is kept when they leave, so their handle always resolves.
	other, ok, err := s.repo.Member(ctx, row.Other(userID))
	if err != nil {
		return Conversation{}, err
	}
	if !ok {
		return Conversation{}, ErrNotFound
	}
	return Conversation{
		ID: row.ID, OtherHandle: other.Handle, Status: row.Status,
		Initiator: row.InitiatorID == userID, CreatedAt: row.CreatedAt, AcceptedAt: row.AcceptedAt,
	}, nil
}

// Invite opens a pending conversation with the member named by toHandle, holding body as
// its first message.
func (s *Service) Invite(ctx context.Context, fromUserID int64, toHandle, body string) (Conversation, error) {
	body = strings.TrimSpace(body)
	if n := utf8.RuneCountInString(body); n < 1 || n > MaxInvitationRunes {
		return Conversation{}, ErrInvalidMessage
	}
	from, ok, err := s.repo.Member(ctx, fromUserID)
	if err != nil {
		return Conversation{}, err
	}
	if !ok || !from.Active {
		return Conversation{}, ErrNotMember
	}
	to, ok, err := s.repo.MemberByHandle(ctx, toHandle)
	if err != nil {
		return Conversation{}, err
	}
	if !ok || !to.Active || to.UserID == fromUserID {
		return Conversation{}, ErrRecipientUnavailable
	}
	blocked, err := s.repo.Blocked(ctx, fromUserID, to.UserID)
	if err != nil {
		return Conversation{}, err
	}
	if blocked {
		return Conversation{}, ErrRecipientUnavailable
	}
	exists, err := s.repo.PairHasConversation(ctx, fromUserID, to.UserID)
	if err != nil {
		return Conversation{}, err
	}
	if exists {
		return Conversation{}, ErrConversationExists
	}
	// As with referral's daily cap this is a count followed by an insert, so concurrent
	// invitations from one member can slip a few past the cap. The cap is a spam brake,
	// not a quota; the one-conversation-per-pair constraint is what holds exactly.
	sent, oldest, err := s.repo.InvitationsSince(ctx, fromUserID, s.now().Add(-invitationWindow))
	if err != nil {
		return Conversation{}, err
	}
	if sent >= DailyInvitationCap {
		return Conversation{}, &InvitationLimitError{RetryAt: oldest.Add(invitationWindow)}
	}
	row, err := s.repo.CreateInvitation(ctx, fromUserID, to.UserID, body)
	if err != nil {
		return Conversation{}, err
	}
	return s.conversationFor(ctx, fromUserID, row)
}

// Accept opens a pending conversation the caller was invited to.
func (s *Service) Accept(ctx context.Context, userID, conversationID int64) (Conversation, error) {
	return s.answer(ctx, userID, conversationID, StatusOpen)
}

// Decline refuses a pending invitation. The inviter is not told: they keep seeing it as
// unanswered, and the pair cannot be invited again.
func (s *Service) Decline(ctx context.Context, userID, conversationID int64) (Conversation, error) {
	return s.answer(ctx, userID, conversationID, StatusDeclined)
}

func (s *Service) answer(ctx context.Context, userID, conversationID int64, status string) (Conversation, error) {
	row, err := s.repo.AnswerInvitation(ctx, userID, conversationID, status)
	if err != nil {
		return Conversation{}, err
	}
	return s.conversationFor(ctx, userID, row)
}
