// Package qualityaudit measures, by sampling, how accurate what the catalogue shows is: whether
// a posting we serve is still live at its source, and whether a posting we tag remote says so
// itself. It MEASURES and never acts — nothing here closes, reopens or retags a posting; the
// lifecycle workers own those writes. cmd/quality-audit runs it weekly and publishes the counts.
//
// See openspec/changes/add-sampled-quality-audit/design.md for why each rule is what it is.
package qualityaudit

import (
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
)

// Outcome is what one probe of a posting proved.
type Outcome int

const (
	// Unmeasured: the audit holds no trustworthy evidence either way. It is a count of its own,
	// never folded into Live — a source the audit cannot read must not look healthy.
	Unmeasured Outcome = iota
	// Live: the source answered 2xx on the posting's own URL, on a host that 404s a posting it
	// does not have.
	Live
	// Dead: the source answered 404 or 410, on such a host.
	Dead
)

// Reason says why a posting is Unmeasured. It goes to the run log, never to a gauge label: the
// gauges count only sampled/live/dead and the remainder is the unmeasured share.
type Reason string

const (
	ReasonNoAnswer     Reason = "no_answer"    // 5xx, an unresolved 3xx, a transport failure
	ReasonMoved        Reason = "moved"        // 2xx, but only after leaving the posting's URL
	ReasonRefused      Reason = "refused"      // 403/429: the host metering us, not a verdict
	ReasonUncalibrated Reason = "uncalibrated" // the host did not 404 a posting that cannot exist
	ReasonRobots       Reason = "robots"       // robots.txt disallows it, or could not be read
	ReasonBudget       Reason = "budget"       // the run's request budget ran out first
	ReasonStatusBlind  Reason = "status_blind" // the source keeps a closed posting at 200
	ReasonBadURL       Reason = "bad_url"      // the stored URL is not an absolute http(s) URL
	ReasonCanceled     Reason = "canceled"     // the run was stopped first
)

// readStatus reads one answer by its status alone. Bodies are never read: the shared body
// classifier read arbeitnow's live ".ch" pages ("no longer accepting applications from the UK")
// as closed, and a body is where a site's wording, language and A/B test all live.
//
// A 2xx counts as live only when it is still the posting's own page. Greenhouse sends a closed
// posting to its board with a 200, so a 2xx that arrived somewhere else proves nothing.
func readStatus(requested *url.URL, status int, final *url.URL) (Outcome, Reason) {
	switch {
	case status == http.StatusNotFound || status == http.StatusGone:
		return Dead, ""
	case status >= 200 && status < 300:
		if samePage(requested, final) {
			return Live, ""
		}
		return Unmeasured, ReasonMoved
	case status == http.StatusForbidden || status == http.StatusTooManyRequests:
		return Unmeasured, ReasonRefused
	default:
		return Unmeasured, ReasonNoAnswer
	}
}

// samePage reports whether final is still requested: same host (case folded) and same path (a
// trailing slash ignored). The query is ignored — a tracking parameter added on the way is not a
// different posting, while a posting id living in the query is checked by the canary instead.
func samePage(requested, final *url.URL) bool {
	if final == nil {
		return false
	}
	return strings.EqualFold(requested.Host, final.Host) &&
		strings.TrimSuffix(requested.EscapedPath(), "/") == strings.TrimSuffix(final.EscapedPath(), "/")
}

// minCanaryToken is the shortest posting id the canary will rewrite. A short id collides with a
// real posting too easily, and a canary that hits one answers 200 and marks a good host
// uncalibrated — coverage lost, but for nothing.
const minCanaryToken = 5

// canaryURL builds the address of a posting that cannot exist: the stored URL with its posting
// id — the last ":"-segment of external_id — replaced by a random token of the same shape. A
// host that answers it 404/410 has shown that its 404 means "no such posting", which is what
// lets its other answers count. ok is false when the id is too short or does not appear after
// the host, and then this posting cannot calibrate its host.
//
// The token keeps its shape — digits stay digits (never gaining a leading zero), hex stays hex,
// separators stay put — so a router that validates the id's format still routes the canary to
// the posting handler rather than to a generic 404 page that would prove nothing about it.
func canaryURL(raw, externalID string, rng *rand.Rand) (string, bool) {
	token := externalID[strings.LastIndex(externalID, ":")+1:]
	if len(token) < minCanaryToken {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", false
	}
	hostEnd := strings.Index(raw, u.Host) + len(u.Host)
	at := strings.LastIndex(raw, token)
	if at < hostEnd {
		return "", false
	}
	for range 8 {
		if mutated := mutate(token, rng); mutated != token {
			return raw[:at] + mutated + raw[at+len(token):], true
		}
	}
	return "", false
}

const (
	digits = "0123456789"
	hexLow = "abcdef"
	hexUp  = "ABCDEF"
	lower  = "abcdefghijklmnopqrstuvwxyz"
	upper  = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

func mutate(token string, rng *rand.Rand) string {
	out := []byte(token)
	for i, b := range out {
		var set string
		switch {
		case b >= '0' && b <= '9':
			set = digits
			if i == 0 || !isDigit(out[i-1]) {
				set = digits[1:]
			}
		case b >= 'a' && b <= 'f':
			set = hexLow
		case b >= 'A' && b <= 'F':
			set = hexUp
		case b >= 'a' && b <= 'z':
			set = lower
		case b >= 'A' && b <= 'Z':
			set = upper
		default:
			continue
		}
		out[i] = set[rng.IntN(len(set))]
	}
	return string(out)
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
