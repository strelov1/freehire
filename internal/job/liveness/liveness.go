// Package liveness classifies whether a job posting URL is still live, from a plain
// HTTP probe (no headless browser, no LLM). Classify is a pure function over the fetch
// outcome; the cmd/liveness worker owns fetching and acting on the verdict.
//
// A posting is judged expired only on a positive death signal (HTTP gone, an
// error/listing redirect, a curated closed-posting phrase, or a near-empty body).
// Anything else, including a JS-only shell, is not-expired: without a browser we
// under-detect closures rather than risk closing a live posting.
package liveness

import (
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// Verdict is the outcome of a probe. Three states so the worker can tell a live
// posting (reset strikes) from one it could not judge (leave strikes untouched). Only
// Expired advances a job toward closing.
type Verdict int

const (
	// Uncertain: the probe could not establish liveness (5xx, 403, a fetch error, or
	// any non-2xx that is not a definitive gone). Takes no action.
	Uncertain Verdict = iota
	// Live: a healthy 2xx posting with no death signal. Resets the strike count.
	Live
	// Expired: a definitive death signal. Advances the strike count toward a close.
	Expired
)

// minContentChars is the visible-text length below which a 2xx response is treated as
// an empty shell rather than a real posting. It is measured against extracted text
// (htmltext.ToText), not raw HTML: a JS-only shell's <head> alone can exceed it.
const minContentChars = 300

// hardExpired matches phrases an employer page shows when a posting is closed
// (EN/DE/FR/RU); a match on a 2xx body is a definitive death signal. The
// sub-patterns are top-level alternatives, so a plain "|" join is the whole pattern.
// (?i) folds Cyrillic case too.
var hardExpired = regexp.MustCompile(`(?i)` + strings.Join([]string{
	`job (is )?no longer available`,
	`job.*no longer open`,
	`position has been filled`,
	`this job has expired`,
	`job posting has expired`,
	`no longer accepting applications`,
	`this (position|role|job) (is )?no longer`,
	`this job (listing )?is closed`,
	// remoteok's closed-posting banner ("This job post is closed and the position is
	// probably filled."); the extra "post" is why the pattern above does not cover it.
	`this job post is closed`,
	// djinni's alert on a removed posting, which still answers 200 with the full page.
	`the job ad is no longer active`,
	`job (listing )?not found`,
	`the page you are looking for doesn.t exist`,
	`applications?\s+(?:(?:have|are|is)\s+)?closed`,
	`diese stelle (ist )?(nicht mehr|bereits) besetzt`,
	`offre (expirée|n'est plus disponible)`,
	// A RU-language orphan source (not habr_career/geekjob, which are registered
	// providers) can serve a closed posting as a healthy 200 whose only death signal is
	// a Russian archived/closed banner.
	`ваканси\S* (в архиве|закрыта|неактивна)`,
}, "|"))

// listingPage matches a careers/search index rather than a single posting — the URL
// resolved to a generic list, so the specific job is gone.
var listingPage = regexp.MustCompile(`(?i)` + strings.Join([]string{
	`\d+\s+jobs?\s+found`,
	`search for jobs page is loaded`,
}, "|"))

// expiredURL matches a final URL an ATS redirects to when a posting is gone.
var expiredURL = regexp.MustCompile(`(?i)[?&]error=true`)

// Classify maps a probe outcome to a Verdict and, for Expired, a reason code for the
// close log (never persisted). Checks run from the most to the least definitive; only
// a 2xx response is inspected for content signals.
func Classify(status int, finalURL, body string) (Verdict, string) {
	if status == 404 || status == 410 {
		return Expired, "http_gone"
	}
	// Non-success (5xx, 403, a 3xx that did not resolve, or a fetch error reported as
	// status 0) is uncertain — neither a death signal nor proof of life.
	if status < 200 || status >= 300 {
		return Uncertain, ""
	}
	if expiredURL.MatchString(finalURL) {
		return Expired, "expired_url"
	}
	if hardExpired.MatchString(body) {
		return Expired, "expired_body"
	}
	if listingPage.MatchString(body) {
		return Expired, "listing_page"
	}
	if len(strings.TrimSpace(htmltext.ToText(body))) < minContentChars {
		return Expired, "insufficient_content"
	}
	return Live, ""
}
