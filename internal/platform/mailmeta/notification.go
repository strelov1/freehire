// Package mailmeta names product-generated mail so inbound readers can avoid notification loops.
package mailmeta

const NotificationHeader = "X-Freehire-Notification"
const InboxDigest = "inbox-digest"

const ApplicationDigest = "application-digest"

// IsDigest requires both headers so unrelated automated mail remains visible.
func IsDigest(kind, submitted string) bool {
	return submitted == "auto-generated" && (kind == InboxDigest || kind == ApplicationDigest)
}
