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

// Send renders the digest as a short title/body (a full itemized listing, as
// Telegram/email send, has no room in a push notification) and fans it out to
// every device the user has registered. A single-job digest carries the job's
// slug as deep-link data so the mobile app can open it directly; a digest of
// more than one job carries no deep-link data, matching the "Push digest
// content and deep link" requirement.
//
// A message of several digests is still ONE push: its title counts the alerts
// and its body the jobs, since a banner has no room to name each search.
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

// renderDigest renders a digest into its short, human-readable copy: a title,
// a body summarizing the match count, and — only when the digest matched
// exactly one job — that job's slug (empty otherwise). This is the single
// source of the digest's push copy; the notification-center recording in
// deliverMessage reuses it rather than re-deriving the same wording, adding only
// the note an Overflowed digest's page needs.
//
// The title names the saved search that fired: it is what distinguishes one
// digest from another, and the product name this slot used to carry said
// nothing in either reader — a phone prints the app's name above a push banner
// already, and the notification center is inside the app. The name is always
// there to use: saved_searches.name is NOT NULL under a CHECK that it holds at
// least one non-blank character.
func renderDigest(d Digest) (title, body, slug string) {
	title = d.SavedSearchName
	body = JobCount(d.Total)
	if d.Total == 1 && len(d.Jobs) == 1 {
		slug = d.Jobs[0].Slug
	}
	return title, body, slug
}

// digestJobSnapshot is one job as recorded into a multi-job digest's
// notification-center row — the same three fields DigestJob exposes on the
// wire, no internal id, no salary (the notification-history "which jobs were
// these" screen names them; it isn't a second digest render).
// digestJobsSnapshot marshals a digest's matched jobs for the notification
// center's `jobs` column, but only when there's more than one — a single-job
// digest already has its job identified via public_slug. nil (encodes to SQL NULL)
// for a single-job (or, degenerately, zero-job) digest. The shape itself is
// SnapshotJob, shared with the reminder and nudge engines, which write the same
// column for the same page.
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
