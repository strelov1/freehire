package buddy

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxMessageRunes bounds one chat message.
	MaxMessageRunes = 4000
	// MaxMessagesLimit is the largest page Messages returns; DefaultMessagesLimit applies
	// when none is asked.
	MaxMessagesLimit     = 100
	DefaultMessagesLimit = 50
)

// Message is one chat message as a member sees it.
type Message struct {
	ID        int64
	Mine      bool
	Body      string
	CreatedAt time.Time
}

// MessageRow is a Repository's raw answer for one message.
type MessageRow struct {
	ID        int64
	SenderID  int64
	Body      string
	CreatedAt time.Time
}

// ConversationListRow is a Repository's raw answer for one entry of a member's list.
type ConversationListRow struct {
	ConversationRow
	// Other is the other member: handle, countries and CV, for the card summary.
	Other         SimilarRow
	Unread        int
	LastMessageAt time.Time
}

func messageFor(userID int64, r MessageRow) Message {
	return Message{ID: r.ID, Mine: r.SenderID == userID, Body: r.Body, CreatedAt: r.CreatedAt}
}

// Send posts a message into an open conversation. A pending invitation takes no messages
// from either side: the recipient answers it first.
func (s *Service) Send(ctx context.Context, userID, conversationID int64, body string) (Message, error) {
	body = strings.TrimSpace(body)
	if n := utf8.RuneCountInString(body); n < 1 || n > MaxMessageRunes {
		return Message{}, ErrInvalidMessage
	}
	if err := s.requireActive(ctx, userID); err != nil {
		return Message{}, err
	}
	row, err := s.repo.InsertMessage(ctx, userID, conversationID, body)
	if err != nil {
		return Message{}, err
	}
	return messageFor(userID, row), nil
}

// Messages reads a conversation's messages after the given id, oldest first. It is open to
// both parties in every status, so a closed chat's history stays readable, and it does not
// mark anything read: that is MarkRead's job.
func (s *Service) Messages(ctx context.Context, userID, conversationID, after int64, limit int) ([]Message, error) {
	if err := s.requireActive(ctx, userID); err != nil {
		return nil, err
	}
	conv, ok, err := s.repo.Conversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if !ok || (conv.UserLow != userID && conv.UserHigh != userID) {
		return nil, ErrNotFound
	}
	switch {
	case limit <= 0:
		limit = DefaultMessagesLimit
	case limit > MaxMessagesLimit:
		limit = MaxMessagesLimit
	}
	rows, err := s.repo.Messages(ctx, conversationID, max(after, 0), limit)
	if err != nil {
		return nil, err
	}
	out := make([]Message, len(rows))
	for i, r := range rows {
		out[i] = messageFor(userID, r)
	}
	return out, nil
}

// MarkRead records that the caller has read the conversation up to the message upTo.
func (s *Service) MarkRead(ctx context.Context, userID, conversationID, upTo int64) error {
	if err := s.requireActive(ctx, userID); err != nil {
		return err
	}
	return s.repo.MarkRead(ctx, userID, conversationID, max(upTo, 0))
}

// ListConversations lists the caller's conversations, newest message first. An invitation
// the caller declined is not listed: they have dealt with it.
func (s *Service) ListConversations(ctx context.Context, userID int64) ([]Conversation, error) {
	if err := s.requireActive(ctx, userID); err != nil {
		return nil, err
	}
	rows, err := s.repo.Conversations(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Conversation, len(rows))
	for i, r := range rows {
		c := viewConversation(userID, r.ConversationRow, r.Other.Handle)
		c.Card, c.Unread, c.LastMessageAt = cardSummary(r.Other), r.Unread, r.LastMessageAt
		out[i] = c
	}
	return out, nil
}
