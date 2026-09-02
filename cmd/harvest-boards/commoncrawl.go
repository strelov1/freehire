package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
)

// commonCrawlCollInfoURL lists every Common Crawl monthly snapshot, newest first, each with
// its own CDX API root.
const commonCrawlCollInfoURL = "https://index.commoncrawl.org/collinfo.json"

// commonCrawlSnapshotCount is how many of the most recent snapshots discovery sweeps per
// run. Common Crawl cuts a new snapshot roughly monthly, and snapshots a few months apart
// overlap heavily in which companies they've seen, so the marginal new-candidate yield per
// additional snapshot drops fast while cost (CDX requests) keeps climbing.
const commonCrawlSnapshotCount = 3

// commonCrawlMaxPages bounds the CDX pages read per snapshot, mirroring gupyMaxOffset's
// safety-cap pattern: a spike run found a single host's full result fits in a handful of
// pages, so this is a backstop against a runaway query, not an expected limit.
const commonCrawlMaxPages = 20

// commonCrawlSlug extracts a candidate board id from a Common Crawl-matched URL: the first
// non-empty path segment, exactly as crawled. Greenhouse and Ashby both key a board by that
// segment (boards.greenhouse.io/<slug>/..., jobs.ashbyhq.com/<slug>/...), so no per-provider
// variant is needed. Case is preserved here rather than folded — both providers' APIs are in
// fact case-insensitive, but that's handled once, at the dedup layer (see dedupKey on
// greenhouseProber/ashbyProber in prober.go), not duplicated into every URL this function
// parses. A URL with no path segments (bare host, or root "/") yields no candidate.
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

// commonCrawlParsePage extracts candidate slugs from one CDX page response body: JSON-lines,
// one record per line, each URL sliced to a board id by slugOf. GetText's response-size cap can
// truncate the last line mid-record; a line that fails to parse (truncated or otherwise
// malformed) is silently skipped rather than sinking the whole page, since every complete line
// before it is independently valid JSON.
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

// commonCrawlPageCount asks one snapshot's CDX index how many pages its full result for
// hostPrefix spans, at the finest page granularity (pageSize=1) so a fetch of any one page
// stays as small as the index's own block structure allows.
func commonCrawlPageCount(ctx context.Context, c httpClient, cdxAPI, hostPrefix string) (int, error) {
	var resp struct {
		Pages int `json:"pages"`
	}
	url := fmt.Sprintf("%s?url=%s/*&output=json&showNumPages=true&pageSize=1", cdxAPI, hostPrefix)
	if err := c.GetJSON(ctx, url, &resp); err != nil {
		return 0, err
	}
	return resp.Pages, nil
}

// commonCrawlCandidates discovers candidate board slugs for hostPrefix (e.g.
// "boards.greenhouse.io", or a path prefix like "www.workstream.us/j" where the boards do not
// sit at the host root) from the most recent Common Crawl snapshots, slicing each matched URL
// to a board id with slugOf. A snapshot whose page count can't be read, or whose every page
// fails to fetch, is a failed snapshot: it is skipped and logged, and the sweep continues with
// the remaining snapshots. An error is returned only when every swept snapshot fails.
func commonCrawlCandidates(ctx context.Context, c httpClient, hostPrefix string,
	slugOf func(string) (string, bool)) ([]string, error) {
	apis, err := commonCrawlSnapshots(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("commoncrawl: collinfo: %w", err)
	}

	seen := map[string]struct{}{}
	failures := 0
	for _, api := range apis {
		pages, err := commonCrawlPageCount(ctx, c, api, hostPrefix)
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
			pageURL := fmt.Sprintf("%s?url=%s/*&output=json&page=%d&pageSize=1", api, hostPrefix, page)
			body, err := c.GetText(ctx, pageURL)
			if err != nil {
				log.Printf("commoncrawl: %s page %d: %v", api, page, err)
				continue
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
