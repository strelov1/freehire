package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/strelov1/freehire/internal/engage/pushnotify"
	"github.com/strelov1/freehire/internal/platform/db"
)

// PushTokenLister lists a user's registered push devices. *db.Queries
// satisfies it.
type PushTokenLister interface {
	ListPushTokensForUser(ctx context.Context, userID int64) ([]db.UserPushToken, error)
}

// PushNotifier delivers a digest as a mobile push, fanned out to every device
// the user has registered. dest is the user id — recipient() resolves it
// live from HasPushDevice rather than a stored per-subscription destination
// (see design decision #1); this notifier is the one that expands that single
// id into N device sends via pushnotify.SendToDevices.
type PushNotifier struct {
	tokens    PushTokenLister
	transport pushnotify.Notifier
}

// NewPushNotifier builds a PushNotifier.
func NewPushNotifier(tokens PushTokenLister, transport pushnotify.Notifier) *PushNotifier {
	return &PushNotifier{tokens: tokens, transport: transport}
}

// Send renders the digest as a short title/body (a full itemized listing, as
// Telegram/email send, has no room in a push notification) and fans it out to
// every device the user has registered. A single-job digest carries the job's
// slug as deep-link data so the mobile app can open it directly; a digest of
// more than one job carries no deep-link data, matching the "Push digest
// content and deep link" requirement.
func (n *PushNotifier) Send(ctx context.Context, _ string, dest string, d Digest) error {
	userID, err := strconv.ParseInt(dest, 10, 64)
	if err != nil {
		return fmt.Errorf("notify: push dest %q is not a user id: %w", dest, err)
	}

	rows, err := n.tokens.ListPushTokensForUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("notify: list push tokens for user %d: %w", userID, err)
	}
	tokens := make([]string, len(rows))
	for i, r := range rows {
		tokens[i] = r.Token
	}

	title, body, _ := renderDigest(d)

	var data map[string]string
	if d.Total == 1 {
		data = map[string]string{"slug": d.Jobs[0].Slug}
	}

	return pushnotify.SendToDevices(ctx, n.transport, tokens, title, body, data)
}

// renderDigest renders a digest into its short, human-readable copy: a title,
// a body summarizing the match count, and — only when the digest matched
// exactly one job — that job's slug (empty otherwise). This is the single
// source of the digest's push copy; the notification-center recording in
// deliverOne reuses it verbatim rather than re-deriving the same wording.
func renderDigest(d Digest) (title, body, slug string) {
	title = "freehire"
	body = fmt.Sprintf("%d new jobs for %q", d.Total, d.SavedSearchName)
	if d.Total == 1 {
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
