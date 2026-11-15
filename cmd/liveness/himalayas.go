package main

import (
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// himalayasSitemapIndexURL is himalayas.app's own index of what is currently live. Unlike the job page, which Cloudflare's bot management 403s on every plain GET, it answers a normal 200. himalayas pages its own ingest feed to a recency-limited slice (see himalayasMaxPages in internal/ingest/sources/himalayas.go), which leaves an aged-out company's posting unreachable by the ingest sweep; the sitemap has no such limit, so it is real evidence at the cost of one run-wide fetch instead of one GET per candidate.
const himalayasSitemapIndexURL = "https://himalayas.app/sitemap-jobs.xml.gz"

// sitemapIndex is the top-level <sitemapindex> at himalayasSitemapIndexURL, listing the per-shard sitemap files that cover every live URL. The shard count must be discovered from the index, not hardcoded.
type sitemapIndex struct {
	Sitemaps []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
}

// sitemapURLSet is one shard: a flat list of live URLs. A few non-job pages are mixed in
// (himalayas.app itself, /jobs) — harmless for a membership check.
type sitemapURLSet struct {
	URLs []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
}

// fetchHimalayasLiveJobURLs downloads himalayas's sitemap and returns the set of URLs it lists. Any failure discards what was fetched and returns an error rather than a partial set: a partial set would make a live job look absent and wrongly strike it, and under-closing is the only acceptable failure mode.
func fetchHimalayasLiveJobURLs(ctx context.Context, client *http.Client) (map[string]struct{}, error) {
	return fetchSitemapLiveURLs(ctx, client, himalayasSitemapIndexURL)
}

// fetchSitemapLiveURLs is fetchHimalayasLiveJobURLs with the index URL as a parameter,
// so a test can point it at a stub server instead of the hardcoded constant.
func fetchSitemapLiveURLs(ctx context.Context, client *http.Client, indexURL string) (map[string]struct{}, error) {
	var index sitemapIndex
	if err := fetchSitemapXML(ctx, client, indexURL, &index); err != nil {
		return nil, fmt.Errorf("sitemap index: %w", err)
	}
	if len(index.Sitemaps) == 0 {
		return nil, fmt.Errorf("sitemap index: no shards listed")
	}

	live := make(map[string]struct{})
	for _, s := range index.Sitemaps {
		var shard sitemapURLSet
		if err := fetchSitemapXML(ctx, client, s.Loc, &shard); err != nil {
			return nil, fmt.Errorf("sitemap shard %s: %w", s.Loc, err)
		}
		for _, u := range shard.URLs {
			live[u.Loc] = struct{}{}
		}
	}
	return live, nil
}

// fetchSitemapXML GETs a gzip-compressed sitemap XML document and decodes it into v.
func fetchSitemapXML(ctx context.Context, client *http.Client, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", liveness.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()
	return xml.NewDecoder(gz).Decode(v)
}
