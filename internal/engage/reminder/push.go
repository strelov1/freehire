package reminder

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/engage/pushnotify"
)

// Compile-time proof PushNotifier satisfies the engine's Notifier seam.
var _ Notifier = (*PushNotifier)(nil)

// PushTokenLister lists a user's registered push devices.
type PushTokenLister = pushnotify.TokenLister

// PushNotifier delivers a reminder as a mobile push, fanning out to every device
// the user has registered — unlike Telegram/email, push has zero-to-many
// destinations per recipient.
type PushNotifier struct{ push *pushnotify.UserSender }

// NewPushNotifier builds a PushNotifier resolving devices through tokens and
// sending through transport.
func NewPushNotifier(tokens PushTokenLister, transport pushnotify.Notifier) *PushNotifier {
	return &PushNotifier{push: pushnotify.NewUserSender(tokens, transport)}
}

// Send renders the account's batch as ONE notification and fans it out to every
// device registered for the user encoded in dest (see recipient()'s ChannelPush
// case). The channel argument is ignored — this notifier only serves the push
// channel.
func (n *PushNotifier) Send(ctx context.Context, _ string, dest string, ms []ReminderMessage) error {
	if len(ms) == 0 {
		return nil
	}
	title, body := renderReminderBatch(ms)
	// The deep link needs one destination, so it is carried only when the batch
	// names one job — the same rule a subscription digest follows.
	var data map[string]string
	if len(ms) == 1 {
		data = map[string]string{"slug": ms[0].Slug}
	}
	return n.push.Send(ctx, dest, title, body, data)
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
