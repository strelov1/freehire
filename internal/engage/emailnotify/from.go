package emailnotify

import "net/mail"

// productName is the sender every automated mail shows; the founder sequence
// overrides it with a human name (see internal/engage/onboarding).
const productName = "freehire"

// From builds the From header value, `Name <address@example.com>`, so a message
// list shows who is writing instead of a bare local part. The address may
// already carry a display name; the name given here wins. An unparseable address
// is passed through untouched, since rewriting it would hide the operator's mistake.
func From(name, address string) string {
	parsed, err := mail.ParseAddress(address)
	if err != nil || name == "" {
		return address
	}
	return (&mail.Address{Name: name, Address: parsed.Address}).String()
}
