package messaging

import "context"

func messageFor(userID int64, r MessageRow) Message {
	return Message{ID: r.ID, Mine: r.SenderID == userID, Body: r.Body, CreatedAt: r.CreatedAt}
}

// Send posts a message into an open conversation. A pending one takes no messages from
// either side: its recipient answers it first.
func (s *Service) Send(ctx context.Context, userID, conversationID int64, body string) (Message, error) {
	body, err := normalizeBody(body, MaxMessageRunes)
	if err != nil {
		return Message{}, err
	}
	row, err := s.repo.InsertMessage(ctx, userID, conversationID, body)
	if err != nil {
		return Message{}, err
	}
	return messageFor(userID, row), nil
}

// Messages reads a conversation's messages after the given id, oldest first. It is open to
// every participant in every status, so a closed conversation's history stays readable,
// and it marks nothing read: that is MarkRead's job.
func (s *Service) Messages(ctx context.Context, userID, conversationID, after int64, limit int) ([]Message, error) {
	_, ok, err := s.repo.ConversationFor(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}
	if !ok {
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
	return s.repo.MarkRead(ctx, userID, conversationID, max(upTo, 0))
}
