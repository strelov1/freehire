package nudge

import (
	"fmt"

	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/engage/pushnotify"
)

// PushNotifier delivers a nudge as a mobile push to every device registered for the
// account. The send is the shared notify.Push; this package owns only the copy.
type PushNotifier = notify.Push[[]Message]

// NewPushNotifier builds a PushNotifier resolving devices through tokens and
// sending through transport.
func NewPushNotifier(tokens notify.PushTokenLister, transport pushnotify.Notifier) *PushNotifier {
	return notify.NewPush(tokens, transport, func(ms []Message) (title, body, slug string) {
		title, body = renderNudgeBatch(ms[0].Kind, ms)
		// The deep link needs one destination: only a single-job batch carries it.
		if len(ms) == 1 {
			slug = ms[0].Slug
		}
		return title, body, slug
	})
}

// renderNudgeBatch is the shared title/body copy for one (account, kind) batch, used
// by both the push channel and the notification-center record, so the in-app record
// never drifts from push.
//
// The default arm is unreachable while application_nudges.kind is CHECKed to the
// known kinds; it is the copy a new kind ships with until it gets its own.
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
	case KindAutoApplyReviewReady:
		return "📝 Ready to send", fmt.Sprintf("%d tailored applications are waiting for your OK before auto-apply sends them.", len(ms))
	default:
		return "🔔 Jobs you're tracking", fmt.Sprintf("%d updates on jobs you are tracking.", len(ms))
	}
}

// renderNudge is the single-nudge wording; a batch of one must read identically.
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
	case KindAutoApplyReviewReady:
		return "📝 Ready to send", fmt.Sprintf("Your application to %s at %s is tailored and waiting for your OK.", m.JobTitle, m.Company)
	default:
		return "🔔 A job you're tracking", fmt.Sprintf("%s at %s", m.JobTitle, m.Company)
	}
}
