package employer

import "strings"

// publicWebmailDomains is the closed set of consumer email providers a work email must not
// be at — see employer-account's "public webmail domain refused" requirement. It is
// deliberately short and hand-curated rather than exhaustive: the goal is to catch the
// overwhelmingly common case (a claimant typing their personal inbox instead of a work one),
// not to enumerate every free provider that has ever existed. A miss here still falls to
// moderator review at confirm time, the same as any other domain the system cannot vouch for
// — it is never the only line of defense.
var publicWebmailDomains = map[string]bool{
	"gmail.com":      true,
	"googlemail.com": true,
	"outlook.com":    true,
	"hotmail.com":    true,
	"live.com":       true,
	"msn.com":        true,
	"yahoo.com":      true,
	"ymail.com":      true,
	"icloud.com":     true,
	"me.com":         true,
	"aol.com":        true,
	"protonmail.com": true,
	"proton.me":      true,
	"gmx.com":        true,
	"mail.com":       true,
	"zoho.com":       true,
	"yandex.com":     true,
	"yandex.ru":      true,
	"mail.ru":        true,
	"inbox.ru":       true,
	"bk.ru":          true,
	"list.ru":        true,
	"qq.com":         true,
	"163.com":        true,
	"126.com":        true,
}

// emailDomain lowercases and returns the part of an email address after its last "@", or ""
// for a string with none (an already-invalid address, refused earlier by the caller's own
// validation — this never has to guess).
func emailDomain(email string) string {
	i := strings.LastIndex(email, "@")
	if i < 0 || i == len(email)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(email[i+1:]))
}

// isPublicWebmailDomain reports whether email is at a known consumer provider.
func isPublicWebmailDomain(email string) bool {
	return publicWebmailDomains[emailDomain(email)]
}
