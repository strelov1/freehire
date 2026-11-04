package nudge

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/engage/pushnotify"
)

// Compile-time proof PushNotifier satisfies the engine's Notifier seam.
var _ Notifier = (*PushNotifier)(nil)

// PushTokenLister lists a user's registered push devices.
type PushTokenLister = pushnotify.TokenLister

// PushNotifier delivers a nudge as a mobile push, fanning out to every device
// registered for the account via pushnotify.UserSender.
type PushNotifier struct{ push *pushnotify.UserSender }

// NewPushNotifier builds a PushNotifier resolving devices through tokens and
// sending through transport.
func NewPushNotifier(tokens PushTokenLister, transport pushnotify.Notifier) *PushNotifier {
	return &PushNotifier{push: pushnotify.NewUserSender(tokens, transport)}
}

// Send resolves dest as a user id, lists that user's registered devices, and fans
// ONE notification for the whole batch out to all of them. The channel argument is
// ignored — this notifier only serves the push channel.
func (n *PushNotifier) Send(ctx context.Context, _ string, dest, kind string, ms []Message) error {
	if len(ms) == 0 {
		return nil
	}
	title, body := renderNudgeBatch(kind, ms)
	// The deep link needs one destination, so it is carried only when the batch
	// names one job.
	var data map[string]string
	if len(ms) == 1 {
		data = map[string]string{"slug": ms[0].Slug}
	}
	return n.push.Send(ctx, dest, title, body, data)
}

// renderNudgeBatch is the shared title/body copy for one (account, kind) batch,
// used by both the push channel (Send, above) and the notification-center record
// written by Runner.deliverBatch in nudge.go — one rendering, two readers, so the
// in-app record never drifts from what push already shows.
//
// The default arm is not reachable today — application_nudges.kind is CHECKed
// against exactly the three above — but it is the copy a fourth kind would ship
// with until someone gave it its own, so it says what the notification is about
// rather than naming the product.
func renderNudgeBatch(kind string, ms []Message) (title, body string) {
	if len(ms) == 1 {
		return renderNudge(ms[0])
	}
	switch kind {
	case KindFollowUp:
		return "👋 Follow up?", fmt.Sprintf("%d of your applications have gone quiet.", len(ms))
	case KindInterviewPrep:
		return "🎯 Interviews coming up", fmt.Sprintf("You're interviewing for %d roles.", len(ms))
	case KindJobClosed:
		return "📪 Jobs closed", fmt.Sprintf("%d jobs you were tracking were closed.", len(ms))
	case KindAutoApplySubmitted:
		return "🎉 Applications submitted", fmt.Sprintf("Auto-apply submitted %d applications for you.", len(ms))
	case KindAutoApplyBlocked:
		return "⚠️ Needs your attention", fmt.Sprintf("Auto-apply couldn't finish %d applications — a required question needs your own answer.", len(ms))
	case KindAutoApplyFailed:
		return "Auto-apply couldn't submit", fmt.Sprintf("Auto-apply couldn't submit %d applications, and won't try again.", len(ms))
	default:
		return "🔔 Jobs you're tracking", fmt.Sprintf("%d updates on jobs you are tracking.", len(ms))
	}
}

// renderNudge is the single-nudge wording, kept as its own function because it is
// the one a batch of one must be indistinguishable from.
func renderNudge(m Message) (title, body string) {
	switch m.Kind {
	case KindFollowUp:
		return "👋 Follow up?", fmt.Sprintf("It's been %d days since anything moved on %s at %s.", m.DaysSilent, m.JobTitle, m.Company)
	case KindInterviewPrep:
		return "🎯 Interview coming up", fmt.Sprintf("You're interviewing for %s at %s.", m.JobTitle, m.Company)
	case KindJobClosed:
		return "📪 Job closed", fmt.Sprintf("%s at %s was closed.", m.JobTitle, m.Company)
	case KindAutoApplySubmitted:
		return "🎉 Application submitted", fmt.Sprintf("Auto-apply submitted your application to %s at %s.", m.JobTitle, m.Company)
	case KindAutoApplyBlocked:
		return "⚠️ Needs your attention", fmt.Sprintf("Auto-apply couldn't finish %s at %s — a required question needs your own answer.", m.JobTitle, m.Company)
	case KindAutoApplyFailed:
		return "Auto-apply couldn't submit", fmt.Sprintf("Auto-apply couldn't submit %s at %s, and won't try again.", m.JobTitle, m.Company)
	default:
		return "🔔 A job you're tracking", fmt.Sprintf("%s at %s", m.JobTitle, m.Company)
	}
}
