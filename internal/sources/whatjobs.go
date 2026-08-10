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
	provider    string
	country     string
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
	// whatjobsMinCorroboratedPct is the share of a page's postings that must name the keyword's terms
	// for the crawl to continue. The feed's keyword RANKS rather than filters and pads a thin result
	// set with unrelated inventory: measured on "rust developer", page 1 corroborated 47/47 and page 2
	// only 4/26. Any threshold inside that gap behaves the same, so this sits plainly in the middle
	// rather than being tuned to one sample.
	whatjobsMinCorroboratedPct = 50
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
)

// whatjobsMarket describes one WhatJobs publisher account. The vendor issues a separate
// publisher id per country, so country (and the provider name it registers under) is a property
// of the credential, not of any posting — a market that isn't in this table needs a new row here
// plus its own board file, nothing else. code is the short key its publisher id is read under
// from WHATJOBS_PUBLISHER_IDS (see registry.go); provider is a distinct dedup namespace per
// market, since each account has its own id space for the tracked-URL posting id (see
// whatjobsExternalID) — sharing one provider across markets would let two different countries'
// postings collide on the dedup key (source, external_id). "us" alone keeps the bare "whatjobs"
// provider name (no suffix), for backward compatibility with rows already ingested under it.
type whatjobsMarket struct {
	code     string
	provider string
	country  string
}

var whatjobsMarkets = []whatjobsMarket{
	{"us", "whatjobs", "United States"},
	{"br", "whatjobs-br", "Brazil"},
	{"de", "whatjobs-de", "Germany"},
	{"fr", "whatjobs-fr", "France"},
	{"ar", "whatjobs-ar", "Argentina"},
	{"at", "whatjobs-at", "Austria"},
	{"uk", "whatjobs-uk", "United Kingdom"},
	{"be", "whatjobs-be", "Belgium"},
	{"vn", "whatjobs-vn", "Vietnam"},
	{"bh", "whatjobs-bh", "Bahrain"},
	{"tr", "whatjobs-tr", "Turkey"},
	{"cl", "whatjobs-cl", "Chile"},
	{"co", "whatjobs-co", "Colombia"},
	{"ch", "whatjobs-ch", "Switzerland"},
	{"hu", "whatjobs-hu", "Hungary"},
	{"dk", "whatjobs-dk", "Denmark"},
	{"ae", "whatjobs-ae", "United Arab Emirates"},
	{"ca", "whatjobs-ca", "Canada"},
	{"id", "whatjobs-id", "Indonesia"},
	{"es", "whatjobs-es", "Spain"},
	{"mx", "whatjobs-mx", "Mexico"},
	{"ve", "whatjobs-ve", "Venezuela"},
	{"in", "whatjobs-in", "India"},
	{"my", "whatjobs-my", "Malaysia"},
	{"om", "whatjobs-om", "Oman"},
	{"ph", "whatjobs-ph", "Philippines"},
	{"pk", "whatjobs-pk", "Pakistan"},
	{"qa", "whatjobs-qa", "Qatar"},
	{"sa", "whatjobs-sa", "Saudi Arabia"},
	{"sg", "whatjobs-sg", "Singapore"},
	{"za", "whatjobs-za", "South Africa"},
	{"fi", "whatjobs-fi", "Finland"},
	{"hk", "whatjobs-hk", "Hong Kong"},
	{"ie", "whatjobs-ie", "Ireland"},
	{"it", "whatjobs-it", "Italy"},
	{"kw", "whatjobs-kw", "Kuwait"},
	{"lu", "whatjobs-lu", "Luxembourg"},
	{"nl", "whatjobs-nl", "Netherlands"},
	{"no", "whatjobs-no", "Norway"},
	{"nz", "whatjobs-nz", "New Zealand"},
	{"pe", "whatjobs-pe", "Peru"},
	{"pl", "whatjobs-pl", "Poland"},
	{"pt", "whatjobs-pt", "Portugal"},
	{"se", "whatjobs-se", "Sweden"},
	{"th", "whatjobs-th", "Thailand"},
	{"au", "whatjobs-au", "Australia"},
	{"sv", "whatjobs-sv", "El Salvador"},
	{"eg", "whatjobs-eg", "Egypt"},
	{"gr", "whatjobs-gr", "Greece"},
	{"ke", "whatjobs-ke", "Kenya"},
	{"py", "whatjobs-py", "Paraguay"},
}

// NewWhatJobs builds the WhatJobs adapter for the default (US) publisher account over the given
// HTTP client. The publisher id is the account credential; it is read from the environment by
// All and never from a board file.
func NewWhatJobs(c JSONGetter, publisherID string) Source {
	return newWhatJobs(c, publisherID, "us")
}

// NewWhatJobsMarket builds the WhatJobs adapter for one of the non-default publisher accounts.
// market must be a code from whatjobsMarkets; All is the only caller that resolves a code read
// from configuration, so every other caller passes a literal and an unrecognized code is a
// programmer error.
func NewWhatJobsMarket(c JSONGetter, publisherID, market string) Source {
	return newWhatJobs(c, publisherID, market)
}

// parseWhatJobsPublisherIDs reads WHATJOBS_PUBLISHER_IDS's "code:id,code:id,..." shape into a
// map keyed by whatjobsMarket.code. A malformed pair (no colon) is skipped rather than failing
// the whole variable — one typo'd market must not take every other configured account down with
// it.
func parseWhatJobsPublisherIDs(raw string) map[string]string {
	out := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		code, id, ok := strings.Cut(strings.TrimSpace(pair), ":")
		if !ok || code == "" || id == "" {
			continue
		}
		out[code] = id
	}
	return out
}

func newWhatJobs(c JSONGetter, publisherID, market string) Source {
	for _, m := range whatjobsMarkets {
		if m.code == market {
			return whatjobs{http: c, publisherID: publisherID, provider: m.provider, country: m.country}
		}
	}
	panic("sources: unknown whatjobs market " + market)
}

func (w whatjobs) Provider() string { return w.provider }

// whatjobs lists many employers under one account, so it stays in the source facet and takes each
// posting's company from the feed. It is deliberately NOT boardless: the keyword board is what
// selects the slice to crawl, and an entry without one has nothing to fetch.
func (whatjobs) aggregator() {}

// whatjobs crawls a keyword slice, never the whole catalogue, so its unseen jobs need a window
// wider than the sweep default. See whatjobsSweepGrace.
func (whatjobs) sweepGrace() time.Duration { return whatjobsSweepGrace }

// whatjobsPosting is the part of a feed posting worth reading. snippet is the FULL description HTML
// despite its name and despite the vendor's documentation, which calls it a highlighted excerpt.
//
// The feed sends five more fields, all deliberately unread, so that a later reader does not go
// looking for them: salary is the literal "0.000000 - 0.000000" on every row, job_type is always
// empty, logo always null, and age/age_days measure the record's age in the reseller's index rather
// than the posting date. postcode carries a real US ZIP, but the geography dictionary cannot use it
// — it reads a state token ("Austin, TX"), not a ZIP, and a ZIP would not settle the homonym cities
// anyway: "London, OH" resolves to gb+us exactly as "London" alone does. The documented onmousedown
// field does not exist at all.
type whatjobsPosting struct {
	Title    string `json:"title"`
	Company  string `json:"company"`
	Location string `json:"location"`
	Snippet  string `json:"snippet"`
	URL      string `json:"url"`
}

// Fetch reads one keyword slice. Pagination stops on an EMPTY page — never on a short one: the feed
// post-filters duplicates after selecting the page, so a request for 50 routinely returns 44 while
// more pages remain, and treating that as the end would truncate every keyword. The same
// post-filtering lets a posting appear on more than one page, so repeats are collapsed here rather
// than left for the pipeline to upsert over and over.
func (w whatjobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// A blank keyword is refused rather than crawled. Config validation only rejects an EMPTY board,
	// so an entry padded with whitespace reaches here — and the feed answers a blank keyword with its
	// whole unfiltered inventory (tens of thousands of postings, most of them not even technical),
	// which one stray board entry would pour into the catalogue.
	keyword := strings.TrimSpace(e.Board)
	if keyword == "" {
		return nil, fmt.Errorf("whatjobs: company %q has a blank keyword board", e.Company)
	}
	terms := whatjobsCorroborationTerms(keyword)
	var jobs []Job
	seen := make(map[string]bool)
	stopped := ""
	page := 1
	for ; page <= whatjobsMaxPages; page++ {
		var resp struct {
			Data []whatjobsPosting `json:"data"`
		}
		if err := w.http.GetJSON(ctx, w.pageURL(keyword, page), &resp); err != nil {
			return nil, fmt.Errorf("whatjobs: keyword %q page %d: %w", keyword, page, err)
		}
		if len(resp.Data) == 0 {
			stopped = "feed ran dry"
			break
		}
		var usable, corroborated int
		for _, p := range resp.Data {
			job, ok := p.toJob(w.country)
			if !ok {
				continue
			}
			usable++
			if !p.corroborates(terms) {
				continue
			}
			corroborated++
			if seen[job.ExternalID] {
				continue
			}
			seen[job.ExternalID] = true
			jobs = append(jobs, job)
		}
		// A page full of postings none of which carried a native id means the tracked-URL shape
		// changed. Returning an empty result would hide that: a provider credited with zero ingested
		// jobs is skipped by the unseen sweep, so every existing row would stay open indefinitely
		// while nothing refreshed it. Failing the board surfaces it in board_health within a cycle.
		if usable == 0 {
			return nil, fmt.Errorf("whatjobs: keyword %q page %d: %d postings, none carrying a %s id — tracked URL shape changed?",
				keyword, page, len(resp.Data), whatjobsTrackedID)
		}
		// Relevance does not taper, it falls off a cliff — measured 100% on page 1 and 15% on page 2.
		// A corroborated share under the minimum means the feed has started padding the result set, so
		// every further page is mostly waste. Stopping here is what keeps the request volume (and the
		// rate limiting it caused) down; the page's corroborated postings are real and are kept.
		if len(terms) > 0 && corroborated*100 < usable*whatjobsMinCorroboratedPct {
			stopped = fmt.Sprintf("relevance collapsed to %d/%d on page %d", corroborated, usable, page)
			break
		}
	}
	if page > whatjobsMaxPages {
		stopped = fmt.Sprintf("hit the %d-page budget", whatjobsMaxPages)
	}
	log.Printf("whatjobs: keyword %q returned %d corroborated postings (%s)", keyword, len(jobs), stopped)
	return jobs, nil
}

// whatjobsGenericRoleWords are the keyword words that corroborate nothing: they appear in nearly
// every technical posting, so requiring them would wave through the unrelated inventory the feed pads
// a thin keyword with, exactly as readily as a real match.
var whatjobsGenericRoleWords = map[string]bool{
	"developer": true, "engineer": true, "programmer": true,
	"dev": true, "specialist": true, "architect": true,
}

// whatjobsCorroborationTerms reduces a keyword to the terms a posting must actually name to prove it
// belongs to that keyword. Returns nil for a keyword that is entirely generic — there is nothing to
// corroborate on, so such a board filters nothing rather than dropping everything.
func whatjobsCorroborationTerms(keyword string) []string {
	var out []string
	for _, w := range strings.Fields(strings.ToLower(keyword)) {
		if !whatjobsGenericRoleWords[w] {
			out = append(out, w)
		}
	}
	return out
}

// corroborates reports whether a posting names every one of the keyword's terms. The match is a
// case-insensitive substring rather than a word-boundary match: the feed writes "iOS" and "Node.JS",
// which tokenization would fail to line up with "ios" and "node.js". A substring can false-positive
// ("rust" inside "trust"), which is the cheap direction — the alternative drops real matches.
func (p whatjobsPosting) corroborates(terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	hay := strings.ToLower(p.Title + " " + p.Snippet)
	for _, t := range terms {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
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
// publisher attribution rides along in its path. PostedAt is left nil on purpose (see
// whatjobsPosting): freshness then falls back to when freehire first saw the row, which is true,
// where the feed's own age is not.
func (p whatjobsPosting) toJob(country string) (Job, bool) {
	id, ok := whatjobsExternalID(p.URL)
	if !ok {
		return Job{}, false
	}
	return Job{
		ExternalID:  id,
		URL:         p.URL,
		Title:       p.Title,
		Company:     p.Company,
		Location:    whatjobsLocation(p.Location, country),
		Description: sanitizeHTML(whatjobsResellerMark.ReplaceAllString(p.Snippet, "")),
	}, true
}

// whatjobsLocation states the account's country alongside the posting's city. The feed names no
// country, and its cities collide with better-known foreign ones (London is Ohio here, Vienna is
// Virginia), so without this a third of the postings resolve to no country and a few resolve to the
// wrong one. The country is a fact about the configured publisher account — the id is per-country by
// the vendor's design — not a guess about an individual posting.
func whatjobsLocation(city, country string) string {
	if city = strings.TrimSpace(city); city == "" {
		return country
	}
	return city + ", " + country
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
