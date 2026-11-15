package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// commonCrawlCollInfoURL lists every Common Crawl monthly snapshot, newest first, each with
// its own CDX API root.
const commonCrawlCollInfoURL = "https://index.commoncrawl.org/collinfo.json"

// commonCrawlSnapshotCount is how many of the most recent snapshots discovery sweeps per
// run. Snapshots a few months apart overlap heavily, so the marginal yield per extra snapshot
// drops fast while CDX request cost keeps climbing.
const commonCrawlSnapshotCount = 3

// commonCrawlMaxPages bounds the CDX pages read per snapshot: a backstop against a runaway
// query, not an expected limit. A single host fits in a handful of pages, but a whole-domain
// wildcard such as Workday's "*.myworkdayjobs.com" spans 20+ pages, so a low cap would
// silently drop the tail of the sweep while the run reported success. It is set with headroom
// because the index grows.
const commonCrawlMaxPages = 40

// commonCrawlSlug extracts a candidate board id from a Common Crawl-matched URL: the first
// non-empty path segment, exactly as crawled. Greenhouse and Ashby both key a board by that
// segment, so no per-provider variant is needed. Case is preserved: both APIs are
// case-insensitive, but that is handled once at the dedup layer (dedupKey in prober.go). A URL
// with no path segments yields no candidate.
func commonCrawlSlug(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part != "" {
			return part, true
		}
	}
	return "", false
}

// truncatedButUsable reports whether err is only GetText's size cap and something was read
// before it tripped. A CDX page is JSON-lines, so every complete record before the cut is
// valid and commonCrawlParsePage drops the partial one; half a page of real slugs beats none
// for a best-effort discovery. Spelled out here because everywhere else a truncated body is a
// wrong answer that looks right, which is why GetText reports the cut.
func truncatedButUsable(body string, err error) bool {
	var tooLarge *sources.BodyTooLargeError
	return errors.As(err, &tooLarge) && body != ""
}

// commonCrawlParsePage extracts candidate slugs from one CDX page response body: JSON-lines,
// one record per line, each URL sliced to a board id by slugOf. A line that fails to parse (e.g.
// truncated by GetText's size cap) is skipped rather than sinking the page.
func commonCrawlParsePage(body string, slugOf func(string) (string, bool)) []string {
	var slugs []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if slug, ok := slugOf(rec.URL); ok {
			slugs = append(slugs, slug)
		}
	}
	return slugs
}

// commonCrawlSnapshots returns the CDX API root of the commonCrawlSnapshotCount most recent
// Common Crawl snapshots (collinfo.json is already ordered newest first).
func commonCrawlSnapshots(ctx context.Context, c httpClient) ([]string, error) {
	var collections []struct {
		CDXAPI string `json:"cdx-api"`
	}
	if err := c.GetJSON(ctx, commonCrawlCollInfoURL, &collections); err != nil {
		return nil, err
	}
	n := commonCrawlSnapshotCount
	if len(collections) < n {
		n = len(collections)
	}
	apis := make([]string, 0, n)
	for _, coll := range collections[:n] {
		if coll.CDXAPI != "" {
			apis = append(apis, coll.CDXAPI)
		}
	}
	return apis, nil
}

// commonCrawlPageCount asks one snapshot's CDX index how many pages the full result for
// urlPattern spans, at the finest granularity (pageSize=1) so each page fetch stays small.
func commonCrawlPageCount(ctx context.Context, c httpClient, cdxAPI, urlPattern string) (int, error) {
	var resp struct {
		Pages int `json:"pages"`
	}
	url := fmt.Sprintf("%s?url=%s&output=json&showNumPages=true&pageSize=1", cdxAPI, urlPattern)
	if err := c.GetJSON(ctx, url, &resp); err != nil {
		return 0, err
	}
	return resp.Pages, nil
}

// commonCrawlCandidates discovers candidate board slugs matching urlPattern from the most
// recent Common Crawl snapshots, slicing each matched URL to a board id with slugOf. A
// snapshot whose page count can't be read, or whose every page fails to fetch, is a failed
// snapshot: it is skipped and logged, and the sweep continues with the remaining snapshots.
// An error is returned only when every swept snapshot fails.
// urlPattern is the CDX query's `url=` value in full, glob and all; callers spell their own
// because the shapes are not interchangeable. A platform on one host globs its paths
// ("boards.greenhouse.io/*", or a prefix like "www.workstream.us/j/*"). A platform with a
// subdomain per tenant globs the host instead ("*.myworkdayjobs.com") and must carry NO path
// glob: the index answers "*.myworkdayjobs.com/*" with 502.
func commonCrawlCandidates(ctx context.Context, c httpClient, urlPattern string,
	slugOf func(string) (string, bool)) ([]string, error) {
	apis, err := commonCrawlSnapshots(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("commoncrawl: collinfo: %w", err)
	}

	seen := map[string]struct{}{}
	failures := 0
	for _, api := range apis {
		pages, err := commonCrawlPageCount(ctx, c, api, urlPattern)
		if err != nil {
			log.Printf("commoncrawl: %s: page count: %v", api, err)
			failures++
			continue
		}
		if pages > commonCrawlMaxPages {
			pages = commonCrawlMaxPages
		}
		fetched := 0
		for page := 0; page < pages; page++ {
			pageURL := fmt.Sprintf("%s?url=%s&output=json&page=%d&pageSize=1", api, urlPattern, page)
			body, err := c.GetText(ctx, pageURL)
			if err != nil {
				if !truncatedButUsable(body, err) {
					log.Printf("commoncrawl: %s page %d: %v", api, page, err)
					continue
				}
				log.Printf("commoncrawl: %s page %d: %v — keeping the complete records read before the cut", api, page, err)
			}
			fetched++
			for _, slug := range commonCrawlParsePage(body, slugOf) {
				seen[slug] = struct{}{}
			}
		}
		if pages > 0 && fetched == 0 {
			failures++
		}
	}
	if len(apis) > 0 && failures == len(apis) {
		return nil, fmt.Errorf("commoncrawl: all %d snapshots failed", failures)
	}

	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out, nil
}
