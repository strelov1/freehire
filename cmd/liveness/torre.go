package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// torre's crawl reads the whole sitemap, but its sweep is company-scoped (the sitemap still
// names a few closed postings, so it is not fullCatalog — see the Torre traps in
// internal/ingest/sources/AGENTS.md), so the last posting of a company that leaves the sitemap
// stays open. This is that leak's evidence path.
//
// What it reads is the posting page's own state. Torre keeps serving a closed posting's page
// with HTTP 200 and the full layout around it, so the status code says nothing; the page's
// embedded state assigns the opportunity's status, `<v>.status="closed"` or `"open"`, right
// after its prefilledStatus. Measured 2026-10-29 over 41 stale rows: 36 closed, 4 answering 404
// (an unknown id redirects to /404, which answers 404) and 1 open; six rows the crawl still
// lists all read open.
//
// Torre's own API (/api/suite/opportunities/<id>) carries the same status in a fraction of the
// bytes, but robots.txt disallows /api/, and the adapter reads only what it allows.

// torrePostPage is a posting page by id; Torre redirects it to the slugged canonical URL.
const torrePostPage = "https://torre.ai/post/%s"

const (
	// torreProbeInterval spaces torre probes. Two requests a second (a page and the API) drew
	// 503s on half of them; one page every two seconds drew none in 41 (2026-10-29).
	torreProbeInterval = 2 * time.Second
	// torreMaxProbesPerRun bounds one run to ~20 minutes. ~250 torre postings were past the
	// sweep window when this landed, so one run reaches all of them.
	torreMaxProbesPerRun = 600
	// torreReadLimit is how much of a page is read. The status sits 390-450 KB into pages of
	// 450-700 KB, behind the inlined bundle.
	torreReadLimit = 1 << 20
)

// torreStatus matches the opportunity's own status assignment. prefilledStatus is a bare
// value or a small object, and anchoring on it keeps the match off the members' and
// applicants' statuses ("accepted", ...) that the same state carries.
var torreStatus = regexp.MustCompile(`\.prefilledStatus=(?:\{[^{}]*\}|[^;{}]*);[A-Za-z_$][\w$]*\.status="([a-z]+)"`)

// torrePass is torre's paced pass over pageTemplate (torrePostPage, or a stub in a test).
func torrePass(client *http.Client, pageTemplate string) pacedPass {
	return pacedPass{
		source:    "torre",
		interval:  torreProbeInterval,
		maxPerRun: torreMaxProbesPerRun,
		check: func(ctx context.Context, c candidate) pacedVerdict {
			return checkTorreLiveAt(ctx, client, pageTemplate, strings.TrimPrefix(c.externalID, ":"))
		},
	}
}

// checkTorreLiveAt reads one posting page's status: "closed" is Expired, "open" is Live, and a
// 404 or 410 is a posting Torre no longer knows. Everything else — another status, a page
// without the assignment, a server error — is Uncertain; a 429 also ends the pass.
func checkTorreLiveAt(ctx context.Context, client *http.Client, pageTemplate, id string) pacedVerdict {
	if id == "" {
		return pacedVerdict{verdict: liveness.Uncertain}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(pageTemplate, id), nil)
	if err != nil {
		return pacedVerdict{verdict: liveness.Uncertain}
	}
	req.Header.Set("User-Agent", liveness.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := client.Do(req)
	if err != nil {
		return pacedVerdict{verdict: liveness.Uncertain}
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusGone:
		return pacedVerdict{verdict: liveness.Expired, reason: "torre_post_gone"}
	case http.StatusTooManyRequests:
		return pacedVerdict{verdict: liveness.Uncertain, refused: true}
	default:
		return pacedVerdict{verdict: liveness.Uncertain}
	}
	page, err := io.ReadAll(io.LimitReader(resp.Body, torreReadLimit))
	if err != nil {
		return pacedVerdict{verdict: liveness.Uncertain}
	}
	m := torreStatus.FindSubmatch(page)
	switch {
	case m == nil:
		return pacedVerdict{verdict: liveness.Uncertain}
	case string(m[1]) == "closed":
		return pacedVerdict{verdict: liveness.Expired, reason: "torre_post_closed"}
	case string(m[1]) == "open":
		return pacedVerdict{verdict: liveness.Live}
	default:
		return pacedVerdict{verdict: liveness.Uncertain}
	}
}
