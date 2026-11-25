package reminder

import (
	"fmt"

	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/engage/pushnotify"
)

// PushNotifier delivers a reminder as a mobile push, fanning out to every device
// the user has registered. The send is the shared notify.Push; this package owns
// only the copy.
type PushNotifier = notify.Push[[]ReminderMessage]

// NewPushNotifier builds a PushNotifier resolving devices through tokens and
// sending through transport.
func NewPushNotifier(tokens notify.PushTokenLister, transport pushnotify.Notifier) *PushNotifier {
	return notify.NewPush(tokens, transport, func(ms []ReminderMessage) (title, body, slug string) {
		title, body = renderReminderBatch(ms)
		// The deep link needs one destination, so it is carried only when the batch
		// names one job — the same rule a subscription digest follows.
		if len(ms) == 1 {
			slug = ms[0].Slug
		}
		return title, body, slug
	})
}

// renderReminderBatch produces the title/body for one account's batch, shared by the push
// channel and the notification-center record so both use the same wording.
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
