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
	"net/url"
	"strconv"
	"strings"
)

// Section names the part of a CV a traced link sits in. It is half of the persisted
// `source_path`, so its values are a controlled vocabulary rather than free text.
type Section string

const (
	SectionHeaderLinks Section = "header.links"
	SectionProjectLink Section = "projects"
)

// Target is one traceable link: the position it occupies in the CV document, and the absolute
// destination a visitor following it must end up at.
//
// The position is kept as its parts rather than a formatted string because it becomes half of
// the uniqueness key that makes minting idempotent — and a key that only exists pre-formatted
// has to be parsed back apart by whoever needs it next.
type Target struct {
	Section Section
	Index   int
	URL     string
}

// SourcePath is how the position is spelled in storage and in the render payload.
func (t Target) SourcePath() string {
	if t.Section == SectionProjectLink {
		return fmt.Sprintf("%s[%d].link", t.Section, t.Index)
	}
	return fmt.Sprintf("%s[%d]", t.Section, t.Index)
}

// Client is what a click's request says about who made it.
type Client struct {
	IsBot      bool
	DeviceType string
	OSFamily   string
	UAFamily   string
}

// Targets returns the traceable links of a CV, in document order, each carrying the position it
// came from. Positions are the indexes of the slices as given, so a skipped link does not shift
// the ones after it — the renderer puts each href back where it found it.
//
// ownHosts are the domains this deployment serves; a link already pointing at one of them is
// left alone, because tracing it would nest a token inside a token.
func Targets(ownHosts, headerLinks, projectLinks []string) []Target {
	var out []Target
	for i, raw := range headerLinks {
		if dest, ok := destination(ownHosts, raw); ok {
			out = append(out, Target{Section: SectionHeaderLinks, Index: i, URL: dest})
		}
	}
	for i, raw := range projectLinks {
		if dest, ok := destination(ownHosts, raw); ok {
			out = append(out, Target{Section: SectionProjectLink, Index: i, URL: dest})
		}
	}
	return out
}

// destination normalises a stored link into the absolute URL a redirect can send a visitor to,
// reporting whether it is traceable at all.
//
// CVs store links the way a candidate writes them on paper — "github.com/ada", no scheme — so a
// missing scheme is the common case and means https, not "not a link". A scheme that is present
// and is not http(s) means the opposite: mailto: and tel: are working links that tracing would
// break, and there is nothing to count on either.
func destination(ownHosts []string, raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	if hasScheme(s) {
		lower := strings.ToLower(s)
		if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
			return "", false
		}
	} else {
		s = "https://" + s
	}

	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", false
	}
	// Credentials in a URL are a phishing construction, and a stored password nobody asked for.
	if u.User != nil {
		return "", false
	}
	if isOurs(ownHosts, u.Hostname()) {
		return "", false
	}
	return s, true
}

// hasScheme reports whether the link already names a scheme — "https://…", but also the
// slash-less "mailto:…" and "tel:…".
//
// Two things a bare "is there a colon" test gets wrong: a colon after the first slash belongs to
// a path ("example.com/a:b"), and a colon followed by digits is a port on a scheme-less host
// ("myserver.dev:8080/cv"), which is a perfectly traceable link rather than an exotic protocol.
func hasScheme(s string) bool {
	colon := strings.IndexByte(s, ':')
	if colon < 0 {
		return false
	}
	if slash := strings.IndexByte(s, '/'); slash >= 0 && slash < colon {
		return false
	}
	rest := s[colon+1:]
	if end := strings.IndexByte(rest, '/'); end >= 0 {
		rest = rest[:end]
	}
	if rest != "" {
		if _, err := strconv.Atoi(rest); err == nil {
			return false
		}
	}
	return true
}

// isOurs reports whether a host is one this deployment serves. The comparison is over the
// normalised host: case folded, the "www." label dropped, and the trailing dot of a
// fully-qualified name removed — that dot names the same host to DNS and is the cheapest way
// past a string comparison.
func isOurs(ownHosts []string, host string) bool {
	h := normaliseHost(host)
	for _, own := range ownHosts {
		if h == normaliseHost(own) {
			return true
		}
	}
	return false
}

func normaliseHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimSuffix(h, ".")
	return strings.TrimPrefix(h, "www.")
}

// tokenAlphabet is what Token's random part is drawn from — lowercase, because the token is read
// off a hover tooltip and an address bar, where case is easy to mistake and impossible to check.
// It mirrors the alphabet of crypto/rand.Text, lowercased.
const tokenAlphabet = "abcdefghijklmnopqrstuvwxyz234567"

// tokenRandomLen is 5 rather than the 2 of the tool this borrows from. Two characters is a few
// hundred tokens per prefix, which is ample for one self-hosted user and wrong here: hundreds of
// candidates apply to the same company and share its prefix, so by the birthday bound collisions
// begin around the thirtieth token.
const tokenRandomLen = 5

// noCompanyPrefix stands in when the CV is tied to no job, so every token has the same shape.
const noCompanyPrefix = "cv"

// Token mints a token for a CV's link. The prefix is the company the CV was tailored for: the
// recruiter sees it on hover and in the address bar during the redirect, and their own company's
// name reads less alarmingly there than an opaque string.
//
// Uniqueness is the database's to enforce, through the token's unique index. Nothing retries: a
// collision leaves that one link untraced for this download and mints cleanly on the next one. So
// this only has to make a collision rare, not impossible.
func Token(prefix string) string {
	// crypto/rand.Text draws from the base32 alphabet and cannot fail, so there is no error path
	// here to leave a caller holding an empty token.
	random := strings.ToLower(rand.Text())[:tokenRandomLen]
	return sanitizePrefix(prefix) + "-" + random
}

func sanitizePrefix(prefix string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(prefix) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	if p := strings.Trim(b.String(), "-"); p != "" {
		return p
	}
	return noCompanyPrefix
}

// botMarkers are matched as plain substrings of the lowercased user agent. Whole-word matching —
// the obvious spelling, and the one the tool this borrows from uses — misses "HeadlessChrome",
// because the character after "headless" is a word character.
//
// Every entry here is a name no browser carries. The generic "bot" is deliberately absent: it
// appears inside "CUBOT", a phone brand, and inside "Safari Technology Preview" lives the word
// "preview" that a scanner list would otherwise claim. Generic bots are caught by botSuffixes.
var botMarkers = []string{
	"crawler", "spider", "scanner", "headless", "curl/", "wget",
	"python-requests", "go-http-client", "java/", "okhttp",
	"facebookexternalhit", "whatsapp", "skypeuripreview", "googleimageproxy",
}

// botSuffixes catch the general shape of a bot's name — "…bot" followed by a version separator,
// as in "Googlebot/2.1" or "bingbot;" — without claiming every product whose name happens to end
// in those three letters.
var botSuffixes = []string{"bot/", "bot;", "bot)", "bot ", "bot-"}

// Classify reads what a click's request says about its maker. It is called once, when the click
// is recorded, and its verdict is stored: recomputing it on read would let a later edit to these
// markers silently rewrite history.
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
// user agent; anything else is a fetcher of some kind, and a link checker issuing HEAD is one no
// user-agent list would catch.
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
	for _, suffix := range botSuffixes {
		if strings.Contains(lowerUA, suffix) {
			return true
		}
	}
	return strings.HasSuffix(lowerUA, "bot")
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

// uaFamily reads the browser off the user agent. Order is the whole logic: every entry below
// borrows the name of one further down — Edge, Opera and Samsung Internet all claim Chrome, and
// Chrome claims Safari — so the most specific claim has to be tested first. Reporting a borrower
// as Chrome would be the guessing the unknown fallback exists to refuse.
func uaFamily(lowerUA string) string {
	switch {
	case strings.Contains(lowerUA, "edg/"), strings.Contains(lowerUA, "edga/"), strings.Contains(lowerUA, "edgios/"):
		return "edge"
	case strings.Contains(lowerUA, "opr/"), strings.Contains(lowerUA, "opera"):
		return "opera"
	case strings.Contains(lowerUA, "samsungbrowser/"):
		return "samsung"
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
// would be anonymisation in appearance only. Counting distinct visitors is worth having, but not
// at the price of storing something that only looks anonymous.
//
// An empty return therefore means "not identifiable", and a read surface must report no
// distinct-visitor count at all rather than counting every such click as one visitor.
func VisitorHash(salt, ip, userAgent string) string {
	if salt == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(salt))
	// A separator neither field can contain — an HTTP header cannot carry a raw newline — so
	// ("1.2.3.4", "5") and ("1.2.3.45", "") cannot collide.
	mac.Write([]byte(ip + "\n" + userAgent))
	return hex.EncodeToString(mac.Sum(nil))
}
