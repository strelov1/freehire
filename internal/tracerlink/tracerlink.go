// Package tracerlink holds the domain of opt-in CV link tracing: which links in a CV can be
// traced, the tokens that stand in for them, and what a click tells us about the visitor.
//
// Everything here is a pure function over its arguments. Storage, HTTP and the render payload
// live with their own layers.
package tracerlink

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/url"
	"strings"
)

// productHost is our own domain. A link already pointing at it is left alone: tracing it would
// nest a token inside a token.
const productHost = "freehire.me"

// Target is one traceable link: where it sits in the CV document, and the absolute
// destination a visitor following it must end up at.
type Target struct {
	SourcePath string
	URL        string
}

// Client is what a click's request says about who made it.
type Client struct {
	IsBot      bool
	DeviceType string
	OSFamily   string
	UAFamily   string
}

// Targets returns the traceable links of a CV, in document order, each carrying the position
// it came from. Positions are the indexes of the slices as given, so a skipped link does not
// shift the ones after it — the renderer puts each href back where it found it.
func Targets(headerLinks, projectLinks []string) []Target {
	var out []Target
	for i, raw := range headerLinks {
		if dest, ok := destination(raw); ok {
			out = append(out, Target{SourcePath: fmt.Sprintf("header.links[%d]", i), URL: dest})
		}
	}
	for i, raw := range projectLinks {
		if dest, ok := destination(raw); ok {
			out = append(out, Target{SourcePath: fmt.Sprintf("projects[%d].link", i), URL: dest})
		}
	}
	return out
}

// destination normalises a stored link into the absolute URL a redirect can send a visitor to,
// reporting whether it is traceable at all.
//
// CVs store links the way a candidate writes them on paper — "github.com/ada", no scheme — so
// a missing scheme is the common case and means https, not "not a link". A scheme that is
// present and is not http(s) means the opposite: mailto: and tel: are working links that
// tracing would break, and there is nothing to count on either.
func destination(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	switch {
	case hasScheme(s):
		lower := strings.ToLower(s)
		if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
			return "", false
		}
	default:
		s = "https://" + s
	}

	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", false
	}
	if isOurs(u.Hostname()) {
		return "", false
	}
	return s, true
}

// hasScheme reports whether the link already names a scheme — "https://…", but also the
// slash-less "mailto:…" and "tel:…". A colon after the first slash is part of a path, not a
// scheme ("example.com/a:b"), which is why the position matters rather than the mere presence.
func hasScheme(s string) bool {
	colon := strings.IndexByte(s, ':')
	if colon < 0 {
		return false
	}
	slash := strings.IndexByte(s, '/')
	return slash < 0 || colon < slash
}

func isOurs(host string) bool {
	h := strings.TrimPrefix(strings.ToLower(host), "www.")
	return h == productHost
}

// tokenAlphabet is deliberately lowercase alphanumeric: the token is read off a hover tooltip
// and an address bar, where case is easy to mistake and impossible to check.
const tokenAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// tokenRandomLen is 5 rather than the 2 of the tool this borrows from. Two letters is 676
// tokens per prefix, which is ample for one self-hosted user and wrong here: hundreds of
// candidates apply to the same company and share its prefix, so by the birthday bound
// collisions begin around the thirtieth token. Five characters give ~60 million.
const tokenRandomLen = 5

// noCompanyPrefix stands in when the CV is tied to no job, so every token has the same shape.
const noCompanyPrefix = "cv"

// Token mints a token for a CV's link. The prefix is the company the CV was tailored for: the
// recruiter sees it on hover and in the address bar during the redirect, and their own
// company's name reads less alarmingly there than an opaque string.
func Token(prefix string) string {
	p := sanitizePrefix(prefix)
	var b strings.Builder
	b.Grow(len(p) + 1 + tokenRandomLen)
	b.WriteString(p)
	b.WriteByte('-')
	for range tokenRandomLen {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(tokenAlphabet))))
		if err != nil {
			// crypto/rand does not fail in practice; if it ever does, a predictable token is
			// worse than no token, and the caller's insert is what must fail.
			return ""
		}
		b.WriteByte(tokenAlphabet[n.Int64()])
	}
	return b.String()
}

func sanitizePrefix(prefix string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(prefix) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	p := strings.Trim(b.String(), "-")
	if p == "" {
		return noCompanyPrefix
	}
	return p
}

// botMarkers are matched as plain substrings of the lowercased user agent, not as whole words.
// A word-boundary match — the obvious spelling, and the one the tool this borrows from uses —
// misses "HeadlessChrome", because the character after "headless" is a word character.
var botMarkers = []string{
	"bot", "crawler", "spider", "preview", "scanner", "security", "headless",
	"curl", "wget", "python-requests", "go-http-client", "facebookexternalhit",
	"whatsapp", "skypeuripreview", "googleimageproxy",
}

// Classify reads what a click's request says about its maker. It is called once, when the click
// is recorded, and its verdict is stored: recomputing it on read would let a later edit to
// these markers silently rewrite history.
func Classify(method, userAgent string) Client {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	os := osFamily(ua)
	return Client{
		IsBot:      isAutomated(method, ua),
		DeviceType: deviceType(ua, os),
		OSFamily:   os,
		UAFamily:   uaFamily(ua),
	}
}

// isAutomated errs towards flagging. A human reading a CV in a browser issues GET and sends a
// user agent; anything else is a fetcher of some kind, and a link checker issuing HEAD is one
// no user-agent list would catch.
//
// It cannot catch the case that matters most: corporate mail-security scanners fetch links with
// ordinary browser user agents. Counts are therefore evidence a link was opened, never proof a
// person read the CV.
func isAutomated(method, lowerUA string) bool {
	if !strings.EqualFold(method, "GET") {
		return true
	}
	if lowerUA == "" {
		return true
	}
	for _, marker := range botMarkers {
		if strings.Contains(lowerUA, marker) {
			return true
		}
	}
	return false
}

// The three families below are dictionaries in the sense this codebase uses the word: they
// recognise or they report unknown, and never guess a plausible-looking answer.

func osFamily(lowerUA string) string {
	switch {
	// Before macos: an iPhone announces itself as "like Mac OS X".
	case strings.Contains(lowerUA, "iphone"), strings.Contains(lowerUA, "ipad"), strings.Contains(lowerUA, "ipod"):
		return "ios"
	// Before linux: Android is built on it and says so.
	case strings.Contains(lowerUA, "android"):
		return "android"
	case strings.Contains(lowerUA, "windows"):
		return "windows"
	case strings.Contains(lowerUA, "mac os x"), strings.Contains(lowerUA, "macintosh"):
		return "macos"
	case strings.Contains(lowerUA, "linux"), strings.Contains(lowerUA, "x11"):
		return "linux"
	default:
		return "unknown"
	}
}

func uaFamily(lowerUA string) string {
	switch {
	// Order is the whole logic here: Edge claims Chrome, and Chrome claims Safari.
	case strings.Contains(lowerUA, "edg/"), strings.Contains(lowerUA, "edga/"), strings.Contains(lowerUA, "edgios/"):
		return "edge"
	case strings.Contains(lowerUA, "chrome/"), strings.Contains(lowerUA, "crios/"):
		return "chrome"
	case strings.Contains(lowerUA, "firefox/"), strings.Contains(lowerUA, "fxios/"):
		return "firefox"
	case strings.Contains(lowerUA, "safari/"):
		return "safari"
	default:
		return "unknown"
	}
}

func deviceType(lowerUA, os string) string {
	switch os {
	case "ios":
		if strings.Contains(lowerUA, "ipad") {
			return "tablet"
		}
		return "mobile"
	case "android":
		// Android reserves "Mobile" for phones; a tablet omits it.
		if strings.Contains(lowerUA, "mobile") {
			return "mobile"
		}
		return "tablet"
	case "windows", "macos", "linux":
		return "desktop"
	default:
		return "unknown"
	}
}

// VisitorHash is how one visitor is told from another without keeping who they are. It returns
// empty when no salt is configured, and that is not a fallback to an unsalted digest: IPv4 has
// 4.3 billion addresses, so a bare hash of an address is reversible by exhaustive search and
// would be anonymisation in appearance only. Counting distinct visitors is worth having, but
// not at the price of storing something that only looks anonymous.
func VisitorHash(salt, ip, userAgent string) string {
	if salt == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(salt))
	// A separator no address contains, so ("1.2.3.4", "5") and ("1.2.3.45", "") cannot collide.
	mac.Write([]byte(ip + "\n" + userAgent))
	return hex.EncodeToString(mac.Sum(nil))
}
