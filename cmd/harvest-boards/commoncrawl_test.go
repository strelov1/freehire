package main

import (
	"context"
	"slices"
	"testing"
)

func TestCommonCrawlSlugFromURL(t *testing.T) {
	cases := []struct {
		url  string
		want string
		ok   bool
	}{
		{"https://boards.greenhouse.io/acme/jobs/12345", "acme", true},
		{"https://boards.greenhouse.io/Acme/jobs/12345?utm_source=x&utm_campaign=y", "Acme", true},
		{"https://jobs.ashbyhq.com/AcmeInc/embed/job/abc-123", "AcmeInc", true},
		{"https://boards.greenhouse.io/", "", false},
		{"https://boards.greenhouse.io", "", false},
	}
	for _, c := range cases {
		got, ok := commonCrawlSlug(c.url)
		if ok != c.ok || got != c.want {
			t.Errorf("commonCrawlSlug(%q) = (%q, %v), want (%q, %v)", c.url, got, ok, c.want, c.ok)
		}
	}
}

// TestCommonCrawlParsePageSkipsMalformedLines covers the 2MiB response-size cap on the
// shared client's GetText: a truncated last line must not sink the whole page, since every
// earlier complete line is still valid JSON on its own.
func TestCommonCrawlParsePageSkipsMalformedLines(t *testing.T) {
	body := `{"url": "https://boards.greenhouse.io/acme/jobs/1"}
{"url": "https://boards.greenhouse.io/acme/jobs/2"}
{"url": "https://boards.greenhouse.io/beta/jobs/3"}
not valid json at all
{"url": "https://boards.greenhouse.io/truncated/jobs/4"`

	got := commonCrawlParsePage(body, commonCrawlSlug)
	want := []string{"acme", "acme", "beta"}
	if !slices.Equal(got, want) {
		t.Errorf("commonCrawlParsePage = %v, want %v", got, want)
	}
}

// commonCrawlCollInfoBody is a 3-snapshot collinfo.json, newest first, matching the real
// shape (only the field this tool reads, cdx-api, is populated).
const commonCrawlCollInfoBody = `[
  {"id": "CC-MAIN-2026-30", "cdx-api": "https://index.commoncrawl.org/CC-MAIN-2026-30-index"},
  {"id": "CC-MAIN-2026-25", "cdx-api": "https://index.commoncrawl.org/CC-MAIN-2026-25-index"},
  {"id": "CC-MAIN-2026-21", "cdx-api": "https://index.commoncrawl.org/CC-MAIN-2026-21-index"},
  {"id": "CC-MAIN-2026-18", "cdx-api": "https://index.commoncrawl.org/CC-MAIN-2026-18-index"}
]`

func TestCommonCrawlCandidatesMergesAcrossSnapshotsAndDedupes(t *testing.T) {
	f := fakeGetter{
		"https://index.commoncrawl.org/collinfo.json": commonCrawlCollInfoBody,

		// Snapshot 1 (newest): 2 pages, contributes acme (twice, on two pages) and beta.
		"https://index.commoncrawl.org/CC-MAIN-2026-30-index?url=boards.greenhouse.io/*&output=json&showNumPages=true&pageSize=1": `{"pages":2}`,
		"https://index.commoncrawl.org/CC-MAIN-2026-30-index?url=boards.greenhouse.io/*&output=json&page=0&pageSize=1":            `{"url": "https://boards.greenhouse.io/acme/jobs/1"}`,
		"https://index.commoncrawl.org/CC-MAIN-2026-30-index?url=boards.greenhouse.io/*&output=json&page=1&pageSize=1":            `{"url": "https://boards.greenhouse.io/beta/jobs/2"}`,

		// Snapshot 2: its page-count query itself fails (unmapped) — a failing snapshot,
		// not present in this map at all.

		// Snapshot 3 (oldest of the 3 taken): 1 page, re-discovers acme (dedup) only.
		"https://index.commoncrawl.org/CC-MAIN-2026-21-index?url=boards.greenhouse.io/*&output=json&showNumPages=true&pageSize=1": `{"pages":1}`,
		"https://index.commoncrawl.org/CC-MAIN-2026-21-index?url=boards.greenhouse.io/*&output=json&page=0&pageSize=1":            `{"url": "https://boards.greenhouse.io/acme/jobs/9"}`,

		// The 4th collinfo entry (CC-MAIN-2026-18) must never be queried: only the 3 most
		// recent snapshots are swept. No entry for it here — a query would fail the test.
	}

	got, err := commonCrawlCandidates(context.Background(), f, "boards.greenhouse.io", commonCrawlSlug)
	if err != nil {
		t.Fatalf("commonCrawlCandidates: %v", err)
	}
	want := []string{"acme", "beta"}
	if !slices.Equal(got, want) {
		t.Errorf("commonCrawlCandidates = %v, want %v", got, want)
	}
}

func TestCommonCrawlCandidatesErrorsWhenEverySnapshotFails(t *testing.T) {
	f := fakeGetter{
		"https://index.commoncrawl.org/collinfo.json": commonCrawlCollInfoBody,
		// No page-count entries for any snapshot: every one of the 3 swept snapshots fails.
	}

	_, err := commonCrawlCandidates(context.Background(), f, "boards.greenhouse.io", commonCrawlSlug)
	if err == nil {
		t.Error("commonCrawlCandidates: want an error when every snapshot fails, got nil")
	}
}

func TestGreenhouseProberDiscoversFromCommonCrawl(t *testing.T) {
	f := fakeGetter{
		"https://index.commoncrawl.org/collinfo.json": commonCrawlCollInfoBody,
		"https://index.commoncrawl.org/CC-MAIN-2026-30-index?url=boards.greenhouse.io/*&output=json&showNumPages=true&pageSize=1": `{"pages":1}`,
		"https://index.commoncrawl.org/CC-MAIN-2026-30-index?url=boards.greenhouse.io/*&output=json&page=0&pageSize=1":            `{"url": "https://boards.greenhouse.io/acme/jobs/1"}`,
		"https://index.commoncrawl.org/CC-MAIN-2026-25-index?url=boards.greenhouse.io/*&output=json&showNumPages=true&pageSize=1": `{"pages":0}`,
		"https://index.commoncrawl.org/CC-MAIN-2026-21-index?url=boards.greenhouse.io/*&output=json&showNumPages=true&pageSize=1": `{"pages":0}`,
	}

	got, err := (greenhouseProber{}).discover(context.Background(), f)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	want := []string{"acme"}
	if !slices.Equal(got, want) {
		t.Errorf("discover = %v, want %v", got, want)
	}
}

func TestAshbyProberDiscoversFromCommonCrawl(t *testing.T) {
	f := fakeGetter{
		"https://index.commoncrawl.org/collinfo.json": commonCrawlCollInfoBody,
		"https://index.commoncrawl.org/CC-MAIN-2026-30-index?url=jobs.ashbyhq.com/*&output=json&showNumPages=true&pageSize=1": `{"pages":1}`,
		"https://index.commoncrawl.org/CC-MAIN-2026-30-index?url=jobs.ashbyhq.com/*&output=json&page=0&pageSize=1":            `{"url": "https://jobs.ashbyhq.com/acme-inc/jobid"}`,
		"https://index.commoncrawl.org/CC-MAIN-2026-25-index?url=jobs.ashbyhq.com/*&output=json&showNumPages=true&pageSize=1": `{"pages":0}`,
		"https://index.commoncrawl.org/CC-MAIN-2026-21-index?url=jobs.ashbyhq.com/*&output=json&showNumPages=true&pageSize=1": `{"pages":0}`,
	}

	got, err := (ashbyProber{}).discover(context.Background(), f)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	want := []string{"acme-inc"}
	if !slices.Equal(got, want) {
		t.Errorf("discover = %v, want %v", got, want)
	}
}
