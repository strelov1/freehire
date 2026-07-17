package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// loxo adapts Loxo recruiting-agency careers boards. Loxo hosts each agency's public
// board at "<host>/<slug>" where host is an agency subdomain (fitnext.app.loxo.co), the
// bare app.loxo.co, or a regional pod (pod4.app.loxo.co); the board id carries the full
// "<host>/<slug>" so one adapter covers every variant. The server-rendered careers page
// links each posting as /job/<base64>; the base64 decodes to a stable <agency_id>-<slug>
// dedup id. A detail page carries the role in og:title, the location in the og:description
// "Location: … Salary:" run, and the HTML body in an embedded application/json blob (there
// is no schema.org JobPosting). Boards are agencies hosting many clients' vacancies, so a
// hub board resolves the client from the posting and falls back to the agency name.

type loxo struct {
	http HTMLGetter
}

// NewLoxo builds the Loxo careers-board adapter over the given HTML client, wrapping it in a
// throttle for the shared app.loxo.co host (see loxoThrottle).
func NewLoxo(c HTMLGetter) Source {
	return loxo{http: newLoxoThrottle(c, loxoSharedHostGap)}
}

func (loxo) Provider() string { return "loxo" }

func (s loxo) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base, err := url.Parse("https://" + e.Board)
	if err != nil {
		return nil, fmt.Errorf("loxo: board %q: %w", e.Board, err)
	}
	root, err := s.http.GetHTML(ctx, base.String())
	if err != nil {
		return nil, fmt.Errorf("loxo: listing %s: %w", e.Board, err)
	}
	locs := loxoJobLinks(base, root)
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, e, loc)
	}), nil
}

// detail fetches one posting's detail page and maps it to a Job, returning ok=false when
// the fetch fails so the caller skips just that posting.
func (s loxo) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	return mapLoxoDetail(root, e, loc)
}

// mapLoxoDetail maps an already-parsed Loxo detail page to a Job. It is split from detail so
// it can be unit-tested on a saved fixture without an HTTP fake.
func mapLoxoDetail(root *html.Node, e CompanyEntry, loc string) (Job, bool) {
	id := loxoExternalID(loc)
	if id == "" {
		return Job{}, false
	}
	title := metaProperty(root, "og:title")
	if title == "" {
		title = loxoStripAgencySuffix(titleText(root))
	}
	desc := loxoDescription(root)
	if title == "" && desc == "" {
		return Job{}, false // not a rendered job page
	}

	company, title := loxoCompany(title, e)
	location := loxoLocation(metaProperty(root, "og:description"))

	return Job{
		ExternalID:  id,
		URL:         loc,
		Title:       title,
		Company:     company,
		Location:    location,
		Description: sanitizeHTML(desc),
		Remote:      isRemote(location),
	}, true
}

// loxoJobIDPattern captures the base64 job token from a /job/<base64> URL, stopping at a
// ?/# so the ?t=… cache-buster and any sub-action suffix are excluded.
var loxoJobIDPattern = regexp.MustCompile(`/job/([A-Za-z0-9=_-]+)(?:$|[/?#])`)

// loxoExternalID returns the posting's stable dedup id: the base64 token decoded to Loxo's
// <agency_id>-<slug> pair, or the raw token when it does not decode cleanly, or "" when the
// URL is not a job posting.
func loxoExternalID(loc string) string {
	m := loxoJobIDPattern.FindStringSubmatch(loc)
	if m == nil {
		return ""
	}
	tok := m[1]
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if dec, err := enc.DecodeString(tok); err == nil {
			if s := string(dec); strings.ContainsRune(s, '-') && isPrintable(s) {
				return s
			}
		}
	}
	return tok
}

// loxoJobLinks returns the absolute, deduplicated /job/<base64> detail URL of every posting
// linked on a listing page, resolved against base.
func loxoJobLinks(base *url.URL, root *html.Node) []string {
	return jobLinks(base, root, func(href string) bool { return loxoJobIDPattern.MatchString(href) })
}

// loxoDescription returns the HTML description from the first embedded
// <script type="application/json"> block that carries one.
func loxoDescription(root *html.Node) string {
	var desc string
	walk(root, func(n *html.Node) bool {
		if desc != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "script" && attr(n, "type") == "application/json" {
			var p struct {
				Description string `json:"description"`
			}
			if json.Unmarshal([]byte(textContent(n)), &p) == nil && p.Description != "" {
				desc = p.Description
			}
		}
		return true
	})
	// The blob carries raw HTML markup (not entity-escaped), so it goes straight to
	// sanitizeHTML in the caller — no UnescapeString, which would corrupt literal
	// entities in text nodes.
	return desc
}

// loxoLocation extracts the free-text location from the og:description, whose format is
// "<role>Location: <loc>Salary: <pay>…". Returns "" when no "Location:" run is present —
// geography is never guessed.
func loxoLocation(ogDesc string) string {
	i := strings.Index(ogDesc, "Location:")
	if i < 0 {
		return ""
	}
	rest := ogDesc[i+len("Location:"):]
	if j := strings.Index(rest, "Salary:"); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}

// loxoCompany resolves the employer for a posting. On a hub board (an agency hosting many
// clients) it splits the client off an explicit em-dash / " @ " delimiter in the title,
// returning (client, roleTitle); otherwise it returns the configured agency name and the
// title unchanged. The ASCII " - " is deliberately not a delimiter: it collides with
// compound roles ("Full-Stack") and location suffixes, so treating it as a client marker
// would guess wrong — never guess.
func loxoCompany(title string, e CompanyEntry) (company, roleTitle string) {
	if e.Hub {
		for _, sep := range []string{" — ", " @ "} {
			if i := strings.LastIndex(title, sep); i >= 0 {
				client := strings.TrimSpace(title[i+len(sep):])
				role := strings.TrimSpace(title[:i])
				if client != "" && role != "" {
					return client, role
				}
			}
		}
	}
	return e.Company, title
}

// loxoStripAgencySuffix drops a trailing " | <agency>" from a document title.
func loxoStripAgencySuffix(title string) string {
	if i := strings.LastIndex(title, " | "); i >= 0 {
		return strings.TrimSpace(title[:i])
	}
	return title
}

func isPrintable(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}

// loxoSharedHost is Loxo's multi-tenant host: many agency boards live at app.loxo.co/<slug>,
// and it 429s when they are crawled back-to-back (listing + detail fan-out). Dedicated agency
// subdomains (fitnext.app.loxo.co) and pods (pod4.app.loxo.co) do not share this limit.
const loxoSharedHost = "app.loxo.co"

// loxoSharedHostGap is the minimum spacing between requests to loxoSharedHost — enough to
// stay under its rate limit while keeping an hourly crawl well within its window.
const loxoSharedHostGap = 350 * time.Millisecond

// loxoThrottle wraps an HTMLGetter to serialize requests to the shared app.loxo.co host,
// spacing them by a minimum gap so a burst of boards on that host does not 429. Requests to
// dedicated subdomains/pods bypass the throttle and keep full concurrency.
type loxoThrottle struct {
	inner HTMLGetter
	gap   time.Duration
	mu    sync.Mutex
	last  time.Time
}

func newLoxoThrottle(inner HTMLGetter, gap time.Duration) *loxoThrottle {
	return &loxoThrottle{inner: inner, gap: gap}
}

func (t *loxoThrottle) GetHTML(ctx context.Context, u string) (*html.Node, error) {
	if loxoIsSharedHost(u) {
		// Hold the lock across the sleep so shared-host requests run one at a time, gap apart.
		t.mu.Lock()
		if d := loxoNextDelay(t.last, time.Now(), t.gap); d > 0 {
			time.Sleep(d)
		}
		t.last = time.Now()
		t.mu.Unlock()
	}
	return t.inner.GetHTML(ctx, u)
}

// loxoIsSharedHost reports whether the URL targets the shared app.loxo.co host (exactly, not a
// subdomain), which is the one that rate-limits.
func loxoIsSharedHost(u string) bool {
	parsed, err := url.Parse(u)
	return err == nil && parsed.Host == loxoSharedHost
}

// loxoNextDelay is how long to wait before the next shared-host request given the last one's
// time: zero on the first request or once the gap has elapsed, else the remaining gap.
func loxoNextDelay(last, now time.Time, gap time.Duration) time.Duration {
	if last.IsZero() {
		return 0
	}
	if d := gap - now.Sub(last); d > 0 {
		return d
	}
	return 0
}
