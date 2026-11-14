// Package mailmeta names product-generated mail so inbound readers can avoid notification loops.
package mailmeta

const NotificationHeader = "X-Freehire-Notification"
const InboxDigest = "inbox-digest"
