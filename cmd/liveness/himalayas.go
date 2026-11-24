package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/liveness"
)

// himalayasSitemapIndexURL is himalayas.app's own index of what is currently live. Unlike the job page, which Cloudflare's bot management 403s on every plain GET, it answers a normal 200. himalayas pages its own ingest feed to a recency-limited slice (see himalayasMaxPages in internal/ingest/sources/himalayas.go), which leaves an aged-out company's posting unreachable by the ingest sweep; the sitemap has no such limit, so it is real evidence at the cost of one run-wide fetch instead of one GET per candidate.
const himalayasSitemapIndexURL = "https://himalayas.app/sitemap-jobs.xml.gz"

// himalayasMaxSitemapBytes caps one sitemap document once inflated. A shard is ~10 MB
// (three shards on 2026-11-24); the cap only stops a runaway body.
const himalayasMaxSitemapBytes = 64 << 20

// fetchHimalayasLiveJobURLs downloads himalayas's sitemap and returns the set of URLs it lists. Any failure discards what was fetched and returns an error rather than a partial set: a partial set would make a live job look absent and wrongly strike it, and under-closing is the only acceptable failure mode.
func fetchHimalayasLiveJobURLs(ctx context.Context, client *http.Client) (map[string]struct{}, error) {
	return fetchSitemapLiveURLs(ctx, client, himalayasSitemapIndexURL)
}

// fetchSitemapLiveURLs is fetchHimalayasLiveJobURLs with the index URL as a parameter,
// so a test can point it at a stub server instead of the hardcoded constant. The shard
// count is discovered from the index, not hardcoded. A few non-job pages are mixed in
// (himalayas.app itself, /jobs) — harmless for a membership check.
func fetchSitemapLiveURLs(ctx context.Context, client *http.Client, indexURL string) (map[string]struct{}, error) {
	index, err := sources.FetchSitemap(ctx, client, indexURL, liveness.UserAgent, himalayasMaxSitemapBytes)
	if err != nil {
		return nil, fmt.Errorf("sitemap index: %w", err)
	}
	if len(index.Sitemaps) == 0 {
		return nil, fmt.Errorf("sitemap index: no shards listed")
	}

	live := make(map[string]struct{})
	for _, s := range index.Sitemaps {
		shard, err := sources.FetchSitemap(ctx, client, s.Loc, liveness.UserAgent, himalayasMaxSitemapBytes)
		if err != nil {
			return nil, fmt.Errorf("sitemap shard %s: %w", s.Loc, err)
		}
		for _, u := range shard.URLs {
			live[u.Loc] = struct{}{}
		}
	}
	return live, nil
}
