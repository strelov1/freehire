package emailnotify

import "net/mail"

// From builds the From header value a mail client shows in its message list:
// `Name <address@example.com>`.
//
// It exists because the list is the only thing most people read before deciding
// whether to open. A bare address renders as its local part — "notifications" —
// which says nothing about who is writing or why, and looks the same as every other
// service that never set one.
//
// The address argument may itself already carry a display name (a deployment is
// free to set NOTIFY_EMAIL_FROM to `freehire <notifications@freehire.me>`); the
// address is extracted from it so the name given here always wins. An unparseable
// value is passed through untouched: a malformed From is the operator's to fix, and
// silently rewriting it would hide the mistake.
// productName is the sender every automated mail shows: a digest, a reminder and a
// verification code all come from the product, not from a person. The founder
// sequence overrides it with a human name — see internal/onboarding.
const productName = "freehire"

func From(name, address string) string {
	parsed, err := mail.ParseAddress(address)
	if err != nil || name == "" {
		return address
	}
	return (&mail.Address{Name: name, Address: parsed.Address}).String()
}
