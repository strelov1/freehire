package reminder

import (
	"context"
	"fmt"
	"strconv"

	"github.com/strelov1/freehire/internal/engage/pushnotify"
	"github.com/strelov1/freehire/internal/platform/db"
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

// Send renders the account's batch as ONE notification and fans it out to every
// device registered for the user encoded in dest (see recipient()'s ChannelPush
// case). The channel argument is ignored — this notifier only serves the push
// channel.
func (n *PushNotifier) Send(ctx context.Context, _ string, dest string, ms []ReminderMessage) error {
	if len(ms) == 0 {
		return nil
	}
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

	title, body := renderReminderBatch(ms)
	// The deep link needs one destination, so it is carried only when the batch
	// names one job — the same rule a subscription digest follows.
	var data map[string]string
	if len(ms) == 1 {
		data = map[string]string{"slug": ms[0].Slug}
	}
	return pushnotify.SendToDevices(ctx, n.transport, tokens, title, body, data)
}

// renderReminderBatch produces the short, human-readable title/body for one
// account's batch, shared by the push channel's own copy and the
// notification-center record — both need the identical wording for the same
// delivery event (see the add-notification-center design).
func renderReminderBatch(ms []ReminderMessage) (title, body string) {
	if len(ms) == 1 {
		return renderReminder(ms[0])
	}
	return "⏰ Reminder", fmt.Sprintf("You saved %d jobs and haven't applied yet — still interested?", len(ms))
}

// renderReminder is the single-job wording, kept as its own function because it is
// the one a batch of one must be indistinguishable from.
func renderReminder(m ReminderMessage) (title, body string) {
	return "⏰ Reminder", fmt.Sprintf("You saved %s at %s — still interested?", m.JobTitle, m.Company)
}
