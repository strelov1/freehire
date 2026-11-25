package notify

import (
	"encoding/json"
	"fmt"

	"github.com/strelov1/freehire/internal/engage/pushnotify"
)

// NewPushNotifier builds the digest's push notifier (notify.Push): it renders a
// digest as a short title/body (no room for an itemized listing) and fans it out
// to every device the user has registered. dest is the user id — recipient()
// resolves it live from HasPushDevice rather than a stored per-subscription
// destination.
//
// A message of several digests is still ONE push: its title counts the alerts and
// its body the jobs. A single-job digest carries the job's slug as deep-link data.
func NewPushNotifier(tokens PushTokenLister, transport pushnotify.Notifier) *Push[Message] {
	return NewPush(tokens, transport, renderPush)
}

// renderPush is the digest's push copy, shared by the one-digest and the
// several-digest message.
func renderPush(m Message) (title, body, slug string) {
	if len(m.Digests) == 1 {
		return renderDigest(m.Digests[0])
	}
	return fmt.Sprintf("%d job alerts", len(m.Digests)), JobCount(m.Total()), ""
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
