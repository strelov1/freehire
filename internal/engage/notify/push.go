package notify

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/strelov1/freehire/internal/engage/pushnotify"
)

// PushTokenLister lists a user's registered push devices.
type PushTokenLister = pushnotify.TokenLister

// PushNotifier delivers a digest as a mobile push, fanned out to every device
// the user has registered. dest is the user id — recipient() resolves it live
// from HasPushDevice rather than a stored per-subscription destination.
type PushNotifier struct{ push *pushnotify.UserSender }

// NewPushNotifier builds a PushNotifier resolving devices through tokens and
// sending through transport.
func NewPushNotifier(tokens PushTokenLister, transport pushnotify.Notifier) *PushNotifier {
	return &PushNotifier{push: pushnotify.NewUserSender(tokens, transport)}
}

// Send renders the digest as a short title/body (no room for an itemized listing)
// and fans it out to every device the user has registered. A single-job digest
// carries the job's slug as deep-link data; a multi-job one carries none.
//
// A message of several digests is still ONE push: its title counts the alerts and
// its body the jobs.
func (n *PushNotifier) Send(ctx context.Context, _ string, dest string, m Message) error {
	if len(m.Digests) == 1 {
		title, body, slug := renderDigest(m.Digests[0])
		var data map[string]string
		if slug != "" {
			data = map[string]string{"slug": slug}
		}
		return n.push.Send(ctx, dest, title, body, data)
	}
	return n.push.Send(ctx, dest, fmt.Sprintf("%d job alerts", len(m.Digests)), JobCount(m.Total()), nil)
}

// renderDigest renders a digest into its short copy: a title, a body with the match
// count, and the job's slug only when the digest matched exactly one job. It is the
// single source of the digest's push copy; deliverMessage reuses it for the
// notification-center recording.
//
// The title is the saved search's name, which is always set (saved_searches.name is
// NOT NULL with a non-blank CHECK).
func renderDigest(d Digest) (title, body, slug string) {
	title = d.SavedSearchName
	body = JobCount(d.Total)
	if d.Total == 1 && len(d.Jobs) == 1 {
		slug = d.Jobs[0].Slug
	}
	return title, body, slug
}

// digestJobsSnapshot marshals a digest's matched jobs for the notification center's
// `jobs` column, only when there is more than one: a single-job digest is already
// identified via public_slug, so it returns nil (SQL NULL). The shape is SnapshotJob.
func digestJobsSnapshot(d Digest) json.RawMessage {
	if d.Total <= 1 {
		return nil
	}
	jobs := make([]SnapshotJob, len(d.Jobs))
	for i, j := range d.Jobs {
		jobs[i] = SnapshotJob{Title: j.Title, Company: j.Company, Slug: j.Slug}
	}
	return JobsSnapshot(jobs)
}
