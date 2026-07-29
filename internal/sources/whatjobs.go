package sources

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// whatjobs adapts the WhatJobs FeedAPI, a CPC network where freehire is a publisher. Unlike the
// ATS adapters it has no per-tenant board: the account exposes one searchable pool, so the board
// carries a SEARCH KEYWORD and each entry crawls that slice — the same shape as hh, whose board is
// a professional_role. Every posting names its own employer, so the entry's company is a display
// label only.
//
// The feed cannot be crawled exhaustively (it stops serving results past roughly two thousand
// pages while reporting a far larger total), and its postings cannot be verified: url points at
// the network's billing landing page rather than the employer's posting, so cmd/liveness can never
// probe one. Both facts are why the adapter declares a wide sweepGrace instead of relying on the
// default unseen window.
type whatjobs struct {
	http        JSONGetter
	publisherID string
}

const (
	whatjobsFeedURL = "https://api.whatjobs.com/api/v1/jobs.json"
	// whatjobsPageSize is the feed's maximum. A larger value is silently clamped to it, and
	// limit=1 combined with a keyword returns an empty page reporting per_page 0 — a bug in the
	// feed's paginator — so the size is pinned here rather than made configurable.
	whatjobsPageSize = 50
	// whatjobsMaxPages bounds one keyword's crawl. The feed stops serving results past roughly two
	// thousand pages regardless of the total it reports, and a broad keyword ("software engineer",
	// 12k postings) would take hundreds of requests to walk, so a keyword is read as a bounded slice
	// and kept narrow in the board file instead. When the budget is what ended a crawl the adapter
	// says so in the log: a bounded slice must never read as full coverage.
	whatjobsMaxPages = 40
	// whatjobsCrawlIP is the mandatory user_ip. A crawl is not a user viewing a page, so sending
	// a real visitor address would fabricate an impression: the feed ignores tracking for an
	// address it does not accept as a viewer but still serves the results, which is exactly the
	// behaviour wanted here. An empty value is rejected outright (HTTP 400).
	whatjobsCrawlIP = "0.0.0.0"
	// whatjobsSweepGrace is how long the unseen sweep spares a whatjobs posting. A crawl reads
	// only the first whatjobsMaxPages of a keyword, so a posting that drifts deeper reads as
	// unseen; on the 48-hour default it would be closed and reopened as it drifts back. Two weeks
	// outlasts that drift, at the cost of a withdrawn posting lingering that long — unavoidable,
	// since nothing about these postings can be probed.
	whatjobsSweepGrace = 14 * 24 * time.Hour
	// whatjobsCountry is the country this publisher account serves. The vendor issues a separate
	// publisher id per country, so it is a property of the credential rather than of any posting;
	// an account for another market needs its own adapter registration, not a parsed field.
	whatjobsCountry = "United States"
)

// NewWhatJobs builds the WhatJobs adapter over the given HTTP client. The publisher id is the
// account credential; it is read from the environment by All and never from a board file.
func NewWhatJobs(c JSONGetter, publisherID string) Source {
	return whatjobs{http: c, publisherID: publisherID}
}

func (whatjobs) Provider() string { return "whatjobs" }

// whatjobs lists many employers under one account, so it stays in the source facet and takes each
// posting's company from the feed. It is deliberately NOT boardless: the keyword board is what
// selects the slice to crawl, and an entry without one has nothing to fetch.
func (whatjobs) aggregator() {}

// whatjobs crawls a keyword slice, never the whole catalogue, so its unseen jobs need a window
// wider than the sweep default. See whatjobsSweepGrace.
func (whatjobs) sweepGrace() time.Duration { return whatjobsSweepGrace }

// whatjobsPosting is one posting from the feed. snippet is the FULL description HTML despite the
// name and despite the vendor's documentation, which describes it as a highlighted excerpt. The
// documented onmousedown field does not exist. salary, jobType and logo are declared here to
// document that they were read and found worthless — every row carries the same placeholder — so a
// later reader does not go looking for them again.
type whatjobsPosting struct {
	Title    string `json:"title"`
	Company  string `json:"company"`
	Location string `json:"location"`
	Postcode string `json:"postcode"`
	Snippet  string `json:"snippet"`
	URL      string `json:"url"`
	AgeDays  int    `json:"age_days"`
	Salary   string `json:"salary"`
	JobType  string `json:"job_type"`
	Logo     string `json:"logo"`
}

// Fetch reads one keyword slice. Pagination stops on an EMPTY page — never on a short one: the feed
// post-filters duplicates after selecting the page, so a request for 50 routinely returns 44 while
// more pages remain, and treating that as the end would truncate every keyword. The same
// post-filtering lets a posting appear on more than one page, so repeats are collapsed here rather
// than left for the pipeline to upsert over and over.
func (w whatjobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	seen := make(map[string]bool)
	page := 1
	for ; page <= whatjobsMaxPages; page++ {
		var resp struct {
			Data []whatjobsPosting `json:"data"`
		}
		if err := w.http.GetJSON(ctx, w.pageURL(e.Board, page), &resp); err != nil {
			return nil, fmt.Errorf("whatjobs: keyword %q page %d: %w", e.Board, page, err)
		}
		if len(resp.Data) == 0 {
			break
		}
		for _, p := range resp.Data {
			job, ok := p.toJob()
			if !ok || seen[job.ExternalID] {
				continue
			}
			seen[job.ExternalID] = true
			jobs = append(jobs, job)
		}
	}
	if page > whatjobsMaxPages {
		log.Printf("whatjobs: keyword %q hit the %d-page budget with %d postings — slice is bounded, not exhausted",
			e.Board, whatjobsMaxPages, len(jobs))
	}
	return jobs, nil
}

// whatjobsTrackedID matches the native posting id in a tracked click-through URL
// (…/pub_api__cpl__<postingID>__<publisherID>). The id is the only stable identity the feed
// exposes — there is no id field on the posting itself.
var whatjobsTrackedID = regexp.MustCompile(`pub_api__cpl__(\d+)__\d+`)

// whatjobsExternalID reads the native posting id out of a tracked URL. It reports ok=false for any
// other URL shape: a posting without this id cannot be deduplicated, so it must be dropped rather
// than stored under something guessed.
func whatjobsExternalID(trackedURL string) (string, bool) {
	m := whatjobsTrackedID.FindStringSubmatch(trackedURL)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// whatjobsResellerMark matches the tracking signature a reseller appends to the body, which 96% of
// the feed's descriptions carry. It identifies the network the posting was resold through, not the
// role, so it is cut before the description reaches the catalogue.
var whatjobsResellerMark = regexp.MustCompile(`\s*#J-\d+-Ljbffr\s*$`)

// toJob maps a posting, returning ok=false when the tracked URL carries no native id. The tracked
// URL is stored verbatim — it is not IP-bound, so the stored copy serves any later visitor and the
// publisher attribution rides along in its path.
//
// Three of the feed's fields are deliberately dropped rather than mapped. salary is the literal
// "0.000000 - 0.000000" on every row, jobType is always empty and logo always null, so storing them
// would assert facts the feed never carried. age/age_days are dropped too: they measure how long
// the record has been in the reseller's index (postings from unrelated companies share one value),
// so PostedAt stays nil and freshness falls back to when freehire first saw the row.
func (p whatjobsPosting) toJob() (Job, bool) {
	id, ok := whatjobsExternalID(p.URL)
	if !ok {
		return Job{}, false
	}
	return Job{
		ExternalID:  id,
		URL:         p.URL,
		Title:       p.Title,
		Company:     p.Company,
		Location:    whatjobsLocation(p.Location),
		Description: sanitizeHTML(whatjobsResellerMark.ReplaceAllString(p.Snippet, "")),
	}, true
}

// whatjobsLocation states the account's country alongside the posting's city. The feed names no
// country, and its cities collide with better-known foreign ones (London is Ohio here, Vienna is
// Virginia), so without this a third of the postings resolve to no country and a few resolve to the
// wrong one. The country is a fact about the configured publisher account — the id is per-country by
// the vendor's design — not a guess about an individual posting.
func whatjobsLocation(city string) string {
	if city = strings.TrimSpace(city); city == "" {
		return whatjobsCountry
	}
	return city + ", " + whatjobsCountry
}

// pageURL builds one feed request. user_agent is deliberately absent: it is optional, it only
// sharpens click attribution on shared IPs, and a slash in its value makes the edge redirect with
// the value corrupted — the reason the vendor's own documented examples fail. unique_id is absent
// too: it promises to suppress previously returned postings and does not.
func (w whatjobs) pageURL(keyword string, page int) string {
	q := url.Values{
		"publisher": {w.publisherID},
		"user_ip":   {whatjobsCrawlIP},
		"keyword":   {keyword},
		"limit":     {strconv.Itoa(whatjobsPageSize)},
		"page":      {strconv.Itoa(page)},
	}
	return whatjobsFeedURL + "?" + q.Encode()
}
