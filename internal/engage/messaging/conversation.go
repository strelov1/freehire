package messaging

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

// normalizeBody trims a message and checks it against limit runes.
func normalizeBody(body string, limit int) (string, error) {
	body = strings.TrimSpace(body)
	if n := utf8.RuneCountInString(body); n < 1 || n > limit {
		return "", ErrInvalidMessage
	}
	return body, nil
}

// Start opens a pending conversation holding the starter's first message, as the
// starter sees it. Who may start one is the kind's decision, made before calling this;
// the core refuses only a block between the two and a taken pair key.
func (s *Service) Start(ctx context.Context, r StartRequest) (Conversation, error) {
	body, err := normalizeBody(r.Body, MaxMessageRunes)
	if err != nil {
		return Conversation{}, err
	}
	if r.StarterID == r.RecipientID {
		return Conversation{}, ErrUnavailable
	}
	r.Body = body
	row, err := s.repo.Start(ctx, r)
	if err != nil {
		return Conversation{}, err
	}
	return s.withCounterpart(ctx, r.StarterID, row, Participant{UserID: r.RecipientID, Persona: r.RecipientPersona})
}

// ConversationByPairKey reads the kind's conversation for a pair key in any status, for a
// kind that must tell "already talking" apart from its own limits before starting.
func (s *Service) ConversationByPairKey(ctx context.Context, kind, pairKey string) (ConversationRow, bool, error) {
	return s.repo.ConversationByPairKey(ctx, kind, pairKey)
}

// StartedSince is how many conversations of the kind userID started after since, and
// when the oldest of them was started — what a kind's rolling start cap is built on.
func (s *Service) StartedSince(ctx context.Context, kind string, userID int64, since time.Time) (int, time.Time, error) {
	return s.repo.StartedSince(ctx, kind, userID, since)
}

// Blocked reports a block between the two accounts in either direction. Blocks are
// account-wide, so a kind hides blocked pairs from its own surfaces with this.
func (s *Service) Blocked(ctx context.Context, a, b int64) (bool, error) {
	return s.repo.Blocked(ctx, a, b)
}

// Accept opens a pending conversation the caller did not start.
func (s *Service) Accept(ctx context.Context, userID, conversationID int64) (Conversation, error) {
	return s.answer(ctx, userID, conversationID, StatusOpen)
}

// Decline refuses a pending conversation. The starter is not told: they keep seeing it as
// unanswered.
func (s *Service) Decline(ctx context.Context, userID, conversationID int64) (Conversation, error) {
	return s.answer(ctx, userID, conversationID, StatusDeclined)
}

func (s *Service) answer(ctx context.Context, userID, conversationID int64, status string) (Conversation, error) {
	row, err := s.repo.Answer(ctx, userID, conversationID, status)
	if err != nil {
		return Conversation{}, err
	}
	return s.single(ctx, userID, row)
}

// single is one conversation as userID sees it, with its counterpart.
func (s *Service) single(ctx context.Context, userID int64, row ConversationRow) (Conversation, error) {
	parts, err := s.repo.Participants(ctx, row.ID)
	if err != nil {
		return Conversation{}, err
	}
	var other Participant
	for _, p := range parts {
		if p.UserID != userID {
			other = p
		}
	}
	return s.withCounterpart(ctx, userID, row, other)
}

func (s *Service) withCounterpart(ctx context.Context, userID int64, row ConversationRow, other Participant) (Conversation, error) {
	c := view(userID, row)
	cps, err := s.counterparts(ctx, userID, other.Persona, []int64{other.UserID})
	if err != nil {
		return Conversation{}, err
	}
	c.Counterpart = drawn(other, cps)
	return c, nil
}

// counterparts asks the persona how each of its accounts is drawn for userID. A persona
// this Service was not built with draws nobody.
func (s *Service) counterparts(ctx context.Context, userID int64, persona string, ids []int64) (map[int64]Counterpart, error) {
	p, ok := s.personas[persona]
	if !ok || len(ids) == 0 {
		return nil, nil
	}
	return p.Counterparts(ctx, userID, ids)
}

// drawn is the counterpart of one participant out of its persona's answer. The persona
// name is always the participant's own, and an unreachable side serves nothing, whatever
// the persona filled in.
func drawn(p Participant, cps map[int64]Counterpart) Counterpart {
	cp := cps[p.UserID]
	if !cp.Reachable {
		return Counterpart{Persona: p.Persona}
	}
	cp.Persona = p.Persona
	return cp
}

// List lists the caller's conversations of every kind, newest message first. A
// conversation the caller declined is not listed: they have dealt with it.
func (s *Service) List(ctx context.Context, userID int64) ([]Conversation, error) {
	rows, err := s.repo.Conversations(ctx, userID)
	if err != nil {
		return nil, err
	}
	byPersona := map[string][]int64{}
	for _, r := range rows {
		byPersona[r.Other.Persona] = append(byPersona[r.Other.Persona], r.Other.UserID)
	}
	resolved := make(map[string]map[int64]Counterpart, len(byPersona))
	for persona, ids := range byPersona {
		if resolved[persona], err = s.counterparts(ctx, userID, persona, ids); err != nil {
			return nil, err
		}
	}
	out := make([]Conversation, len(rows))
	for i, r := range rows {
		c := view(userID, r.ConversationRow)
		c.Counterpart = drawn(r.Other, resolved[r.Other.Persona])
		c.Unread, c.LastMessageAt = r.Unread, r.LastMessageAt
		out[i] = c
	}
	return out, nil
}
