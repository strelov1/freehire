package buddy

import (
	"context"
	"errors"
)

// The reactions a member may give a feed event: a closed vocabulary chosen for a job
// search rather than a generic "like" — congratulating an offer, wishing luck with an
// interview, rooting for someone, being in the same boat, finding a post insightful.
// The stored kind is the constant's value (migration 0220 checks it).
const (
	ReactionCongrats = "congrats"
	ReactionLuck     = "luck"
	ReactionRooting  = "rooting"
	ReactionSame     = "same"
	ReactionInsight  = "insight"
)

var reactionKinds = map[string]bool{
	ReactionCongrats: true, ReactionLuck: true, ReactionRooting: true, ReactionSame: true, ReactionInsight: true,
}

// ErrBadReaction is a reaction kind outside the vocabulary (HTTP 400).
var ErrBadReaction = errors.New("buddy: unknown reaction")

// Reactions is what one event has drawn: how many members chose each kind, and the
// viewer's own choice ("" for none). Counts has no entry for a kind nobody chose.
type Reactions struct {
	Counts map[string]int
	Mine   string
}

// React sets the member's one reaction to a feed event: kind replaces whatever they chose
// before, and "" removes it. Only an event the member's own feed shows can be reacted to —
// read through the feed's own query, positioned at the event, so every exclusion the feed
// keeps (their own events, blocks, hidden applications, private postings, members who
// left) holds here without a second copy of the predicate. Anything else is ErrNotFound,
// which does not say why. It returns the event's reactions after the change.
func (s *Service) React(ctx context.Context, userID, eventID int64, kind string) (Reactions, error) {
	if kind != "" && !reactionKinds[kind] {
		return Reactions{}, ErrBadReaction
	}
	if err := s.requireActive(ctx, userID); err != nil {
		return Reactions{}, err
	}
	rows, err := s.repo.Feed(ctx, userID, nil, false, eventID, 1)
	if err != nil {
		return Reactions{}, err
	}
	if len(rows) == 0 || rows[0].ID != eventID {
		return Reactions{}, ErrNotFound
	}
	if kind == "" {
		err = s.repo.Unreact(ctx, userID, eventID)
	} else {
		err = s.repo.React(ctx, userID, eventID, kind)
	}
	if err != nil {
		return Reactions{}, err
	}
	counts, err := s.repo.ReactionCounts(ctx, userID, []int64{eventID})
	if err != nil {
		return Reactions{}, err
	}
	return counts[eventID], nil
}
