package reminder

import (
	"context"
	"fmt"
	"strconv"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pushnotify"
)

// Compile-time proof PushNotifier satisfies the engine's Notifier seam.
var _ Notifier = (*PushNotifier)(nil)

// PushTokenLister lists a user's registered push devices. *db.Queries satisfies it.
type PushTokenLister interface {
	ListPushTokensForUser(ctx context.Context, userID int64) ([]db.UserPushToken, error)
}

// PushNotifier delivers a reminder as a mobile push, fanning out to every device
// the user has registered — unlike Telegram/email, push has zero-to-many
// destinations per recipient (see design decision #1 and #3 of
// add-push-notification-channel).
type PushNotifier struct {
	tokens    PushTokenLister
	transport pushnotify.Notifier
}

// NewPushNotifier builds a PushNotifier resolving devices through tokens and
// sending through transport.
func NewPushNotifier(tokens PushTokenLister, transport pushnotify.Notifier) *PushNotifier {
	return &PushNotifier{tokens: tokens, transport: transport}
}

// Send renders the reminder and fans it out to every device registered for the
// user encoded in dest (see recipient()'s ChannelPush case). The channel argument
// is ignored — this notifier only serves the push channel.
func (n *PushNotifier) Send(ctx context.Context, _ string, dest string, m ReminderMessage) error {
	userID, err := strconv.ParseInt(dest, 10, 64)
	if err != nil {
		return fmt.Errorf("reminder: invalid push user id %q: %w", dest, err)
	}
	rows, err := n.tokens.ListPushTokensForUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("reminder: list push tokens for user %d: %w", userID, err)
	}
	tokens := make([]string, len(rows))
	for i, row := range rows {
		tokens[i] = row.Token
	}

	title, body := renderReminder(m)
	// A reminder always concerns exactly one job, so the deep-link data is
	// unconditional here — unlike a subscription digest, which only carries a
	// slug when it matched a single job.
	data := map[string]string{"slug": m.Slug}
	return pushnotify.SendToDevices(ctx, n.transport, tokens, title, body, data)
}

// renderReminder produces the short, human-readable title/body for a reminder,
// shared by the push channel's own copy and the notification-center record — both
// need the identical wording for the same delivery event (see the
// add-notification-center design).
func renderReminder(m ReminderMessage) (title, body string) {
	return "⏰ Reminder", fmt.Sprintf("You saved %s at %s — still interested?", m.JobTitle, m.Company)
}
