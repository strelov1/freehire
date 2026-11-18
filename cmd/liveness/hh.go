package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// hh.ru's crawl reads only the newest ~2000 postings of the last 7 days per professional role (see internal/ingest/sources/hhru.go), so a posting that ages out is never listed again and stays open after hh.ru archives or deletes it. This is that leak's evidence path.
//
// It reads the vacancy PAGE, not hh's API: api.hh.ru/vacancies/{id} answers 403 to an anonymous request.
//
// Not the shared plain-GET probe (probeDespiteRegisteredGET): every vacancy page, live ones included, embeds the site's translation table with the string "Вакансия в архиве" inside the first 512 KiB that liveness.Classify reads, so every live posting would read as Expired. hh.ru's rate limit also answers with a redirect to a 200 captcha page, which Classify would judge on content.
//
// Instead it reads the page's analytics flag: window.globalVars.analyticsParams carries "archived": "true" or "false" within the first ~3 KB. A 404 is a deleted vacancy. Everything else (the captcha, a 403, a page without the flag) is Uncertain.

// hhVacancyPage is the canonical vacancy page, built from the vacancy id rather than the stored
// url so a regional host or tracking parameter in jobs.url can never change what is read.
const hhVacancyPage = "https://hh.ru/vacancy/%s"

const (
	// hhProbeInterval spaces hh probes one at a time. One a second drew no captcha, while concurrent requests drew the captcha redirect within ~15 requests. The crawl reads hh's listing from the same IP, so a captcha here would also stall ingest: hence one at a time, and a run that stops at the first captcha.
	hhProbeInterval = time.Second
	// hhMaxProbesPerRun bounds one run to ~25 minutes at hhProbeInterval. The sample is shuffled (see applyPaced), so successive runs reach every row, and the two-strike rule closes a dead posting within a few days.
	hhMaxProbesPerRun = 1500
	// hhReadLimit is how much of a page is read. The archived flag sits at ~2.4 KB while the full page is ~800 KB.
	hhReadLimit = 64 << 10
)

// hhArchivedFlag matches analyticsParams' archived flag. It is a quoted string there, unlike the
// HTML-escaped boolean the client-hydration state carries further down the page.
var hhArchivedFlag = regexp.MustCompile(`"archived": "(true|false)"`)

// hhVacancyID recovers the numeric vacancy id from an hh external id, stored board-scoped as
// "<professional_role>:<vacancy id>". ok=false for anything that is not a plain number.
func hhVacancyID(externalID string) (string, bool) {
	id := externalID[strings.LastIndex(externalID, ":")+1:]
	if id == "" {
		return "", false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return id, true
}

// checkHHLiveAt probes one vacancy page (pageTemplate formatted with the vacancy id, so a test
// can point it at a stub) and classifies it as described at the top of this file.
func checkHHLiveAt(ctx context.Context, client *http.Client, pageTemplate, vacancyID string) pacedVerdict {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(pageTemplate, vacancyID), nil)
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

	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return pacedVerdict{verdict: liveness.Expired, reason: "hh_vacancy_gone"}
	case resp.StatusCode == http.StatusTooManyRequests,
		strings.HasPrefix(resp.Request.URL.Path, "/account/captcha"):
		return pacedVerdict{verdict: liveness.Uncertain, refused: true}
	case resp.StatusCode != http.StatusOK,
		// A redirect anywhere but the vacancy itself (a login wall, the home page) says
		// nothing about this posting.
		resp.Request.URL.Path != "/vacancy/"+vacancyID:
		return pacedVerdict{verdict: liveness.Uncertain}
	}

	head, err := io.ReadAll(io.LimitReader(resp.Body, hhReadLimit))
	if err != nil {
		return pacedVerdict{verdict: liveness.Uncertain}
	}
	m := hhArchivedFlag.FindSubmatch(head)
	switch {
	case m == nil:
		return pacedVerdict{verdict: liveness.Uncertain}
	case string(m[1]) == "true":
		return pacedVerdict{verdict: liveness.Expired, reason: "hh_vacancy_archived"}
	default:
		return pacedVerdict{verdict: liveness.Live}
	}
}

// hhPass is hh's paced pass over pageTemplate (hhVacancyPage, or a stub in a test): one page a
// second, at most hhMaxProbesPerRun a run, stopping at the first captcha or 429.
func hhPass(client *http.Client, pageTemplate string) pacedPass {
	return pacedPass{
		source:    "hh",
		interval:  hhProbeInterval,
		maxPerRun: hhMaxProbesPerRun,
		check: func(ctx context.Context, c candidate) pacedVerdict {
			id, ok := hhVacancyID(c.externalID)
			if !ok {
				log.Printf("liveness: hh %s has no vacancy id in external id %q — skipping", c.publicSlug, c.externalID)
				return pacedVerdict{verdict: liveness.Uncertain}
			}
			return checkHHLiveAt(ctx, client, pageTemplate, id)
		},
	}
}
