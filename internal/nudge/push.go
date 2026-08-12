package nudge

import (
	"context"
	"fmt"
	"strconv"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pushnotify"
)

// Compile-time proof PushNotifier satisfies the engine's Notifier seam.
var _ Notifier = (*PushNotifier)(nil)

// PushTokenLister lists a user's registered push devices. *db.Queries
// satisfies it via its generated ListPushTokensForUser method.
type PushTokenLister interface {
	ListPushTokensForUser(ctx context.Context, userID int64) ([]db.UserPushToken, error)
}

// PushNotifier delivers a nudge as a mobile push, fanning out to every device
// registered for the account (unlike Telegram/email, which have exactly one
// destination) via pushnotify.SendToDevices.
type PushNotifier struct {
	tokens    PushTokenLister
	transport pushnotify.Notifier
}

// NewPushNotifier builds a PushNotifier listing devices through tokens and
// sending through transport.
func NewPushNotifier(tokens PushTokenLister, transport pushnotify.Notifier) *PushNotifier {
	return &PushNotifier{tokens: tokens, transport: transport}
}

// Send resolves dest as a user id, lists that user's registered devices, and
// fans the rendered message out to all of them. The channel argument is
// ignored — this notifier only serves the push channel.
func (n *PushNotifier) Send(ctx context.Context, _ string, dest string, m Message) error {
	userID, err := strconv.ParseInt(dest, 10, 64)
	if err != nil {
		return fmt.Errorf("nudge: invalid push user id %q: %w", dest, err)
	}
	devices, err := n.tokens.ListPushTokensForUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("nudge: list push tokens for user %d: %w", userID, err)
	}
	tokens := make([]string, len(devices))
	for i, d := range devices {
		tokens[i] = d.Token
	}

	title, body := renderNudge(m)
	data := map[string]string{"slug": m.Slug}
	return pushnotify.SendToDevices(ctx, n.transport, tokens, title, body, data)
}

// renderNudge is the shared title/body copy for one nudge Message, used by both
// the push channel (Send, above) and the notification-center record written by
// Runner.fire in nudge.go — one rendering, two readers, so the in-app record
// never drifts from what push already shows.
func renderNudge(m Message) (title, body string) {
	switch m.Kind {
	case KindFollowUp:
		return "👋 Follow up?", fmt.Sprintf("It's been %d days since anything moved on %s at %s.", m.DaysSilent, m.JobTitle, m.Company)
	case KindInterviewPrep:
		return "🎯 Interview coming up", fmt.Sprintf("You're interviewing for %s at %s.", m.JobTitle, m.Company)
	case KindJobClosed:
		return "📪 Job closed", fmt.Sprintf("%s at %s was closed.", m.JobTitle, m.Company)
	default:
		return "freehire", fmt.Sprintf("%s at %s", m.JobTitle, m.Company)
	}
}
