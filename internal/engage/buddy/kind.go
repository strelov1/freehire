package buddy

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/engage/messaging"
)

// KindName is the messaging kind of a buddy conversation, and PersonaName the persona both
// of its participants speak as: a member is their handle there, never their account.
const (
	KindName    = "buddy"
	PersonaName = "buddy"
)

// Notification-center kinds of a buddy notice. user_notifications.kind is free text, so
// they need no migration.
const (
	notificationMessage    = "buddy_message"
	notificationInvitation = "buddy_invitation"
)

// Counterpart is a member speaking as the buddy persona, as another sees them: the handle
// and the card summary, never anything that names the account.
type Counterpart struct {
	Handle string
	Card   CardSummary
}

// CounterpartRow is a Repository's raw answer for one other member of a conversation.
type CounterpartRow struct {
	UserID int64
	SimilarRow
	// Visible is false once either side has left or a block stands between them.
	Visible bool
}

// Persona is the messaging.Persona of a buddy member: drawn by handle and card, named by
// handle, and nothing else — not the account id, not the email, not the Talent Network or
// community handle of the same person.
type Persona struct {
	repo Repository
}

var _ messaging.Persona = (*Persona)(nil)

// NewPersona builds the buddy Persona.
func NewPersona(repo Repository) *Persona { return &Persona{repo: repo} }

// Name is the stored persona.
func (*Persona) Name() string { return PersonaName }

// Counterparts draws each other member by handle and card while both sides are active and
// unblocked; otherwise the conversation still lists, but as a former member.
func (p *Persona) Counterparts(ctx context.Context, viewerID int64, otherIDs []int64) (map[int64]messaging.Counterpart, error) {
	rows, err := p.repo.Counterparts(ctx, viewerID, otherIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]messaging.Counterpart, len(rows))
	for _, r := range rows {
		if r.Visible {
			out[r.UserID] = messaging.Counterpart{
				Reachable: true, Detail: Counterpart{Handle: r.Handle, Card: cardSummary(r.SimilarRow)},
			}
		}
	}
	return out, nil
}

// Names reads each member by handle: a moderator sees what the members saw.
func (p *Persona) Names(ctx context.Context, userIDs []int64) (map[int64]string, error) {
	return p.repo.Handles(ctx, userIDs)
}

// Kind is the messaging.Kind of buddy conversations: the wording of their notices.
type Kind struct{}

var _ messaging.Kind = Kind{}

// Name is the stored kind.
func (Kind) Name() string { return KindName }

// Notice is deliberately contentless: who wrote, and what, lives behind authorization in
// the chat, never in an inbox or a Telegram thread.
func (Kind) Notice(conversationID int64, pending bool) messaging.Notice {
	n := messaging.Notice{
		Title:            "You have a new message from a buddy",
		Line:             "You have a new message from a buddy.",
		Path:             fmt.Sprintf("/my/buddies?c=%d", conversationID),
		NotificationKind: notificationMessage,
		Footer:           "You’re getting this because you joined interview buddies.",
	}
	if pending {
		n.Title, n.Line, n.NotificationKind = "You have a new buddy invitation", "You have a new buddy invitation.", notificationInvitation
	}
	return n
}
