// Package qualityaudit measures, by sampling, how accurate the catalogue is: whether a
// served posting is still live at its source, and whether a posting tagged remote says
// so itself. It only measures; the lifecycle workers own closing, reopening and
// retagging. cmd/quality-audit runs it weekly and publishes the counts.
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
	// Unmeasured: the audit holds no trustworthy evidence either way. Counted on its own,
	// never folded into Live.
	Unmeasured Outcome = iota
	// Live: the source answered 2xx on the posting's own URL, on a host that 404s a posting it
	// does not have.
	Live
	// Dead: the source answered 404 or 410, on such a host.
	Dead
)

// Reason says why a posting is Unmeasured. The fixed vocabulary bounds metric cardinality.
type Reason string

const (
	ReasonNoAnswer     Reason = "no_answer"    // 5xx, a transport failure
	ReasonMoved        Reason = "moved"        // left the posting's URL: a 2xx elsewhere, or a redirect not followed
	ReasonRefused      Reason = "refused"      // 403/429: the host metering us, not a verdict
	ReasonUncalibrated Reason = "uncalibrated" // the host did not 404 a posting that cannot exist
	ReasonRobots       Reason = "robots"       // robots.txt disallows it, or could not be read
	ReasonBudget       Reason = "budget"       // the run's request budget ran out first
	ReasonStatusBlind  Reason = "status_blind" // the source keeps a closed posting at 200
	ReasonBadURL       Reason = "bad_url"      // the stored URL is not an absolute http(s) URL
	ReasonCanceled     Reason = "canceled"     // the run was stopped first
)

// UnmeasuredReasons is the bounded set of coverage failures, in exposition order.
var UnmeasuredReasons = [...]Reason{
	ReasonNoAnswer, ReasonMoved, ReasonRefused, ReasonUncalibrated, ReasonRobots,
	ReasonBudget, ReasonStatusBlind, ReasonBadURL, ReasonCanceled,
}

// readStatus reads one answer by its status alone. Bodies are never read: the shared
// body classifier misread arbeitnow's live ".ch" pages ("no longer accepting
// applications from the UK") as closed.
//
// A 2xx counts as live only when it is still the posting's own page: Greenhouse sends
// a closed posting to its board with a 200.
func readStatus(requested *url.URL, status int, final *url.URL) (Outcome, Reason) {
	switch {
	case status == http.StatusNotFound || status == http.StatusGone:
		return Dead, ""
	case status >= 200 && status < 300:
		if samePage(requested, final) {
			return Live, ""
		}
		return Unmeasured, ReasonMoved
	case status >= 300 && status < 400:
		// A redirect handed back unfollowed: it left the host, or for a path robots.txt forbids.
		return Unmeasured, ReasonMoved
	case status == http.StatusForbidden || status == http.StatusTooManyRequests:
		return Unmeasured, ReasonRefused
	default:
		return Unmeasured, ReasonNoAnswer
	}
}

// samePage reports whether final is still the requested page: same host (case folded)
// and path (trailing slash ignored). The query is ignored; a posting id living in the
// query is checked by the canary instead.
func samePage(requested, final *url.URL) bool {
	if final == nil {
		return false
	}
	return strings.EqualFold(requested.Host, final.Host) &&
		strings.TrimSuffix(requested.EscapedPath(), "/") == strings.TrimSuffix(final.EscapedPath(), "/")
}

// minCanaryToken is the shortest posting id the canary will rewrite: a short id
// collides with a real posting and would wrongly mark a good host uncalibrated.
const minCanaryToken = 5

// canaryURL builds the address of a posting that cannot exist: the stored URL with its
// posting id (the last ":"-segment of external_id) replaced by a random token of the
// same shape. A host that answers it 404/410 has shown its 404 means "no such posting".
// ok is false when the id is too short or does not appear after the host.
//
// The token keeps its shape (digits stay digits, never gaining a leading zero; hex
// stays hex; separators stay put) so a router that validates the id's format still
// routes the canary to the posting handler rather than a generic 404.
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
