package sources

import "testing"

// TestWellfoundPacedThroughFirecrawl guards against a real production incident: crawling
// several role-slice boards for one provider in a single ingest run hits the pipeline's own
// per-board concurrency, and unbounded requests through the hosted Firecrawl client collided
// hard enough to blow Firecrawl's own account-level rate ceiling — 4 of 11 boards failed
// with "vendor rate limit did not lift after 4 attempts" within the first three minutes of a
// live run (2026-09-14), each discarding an otherwise-successful partial crawl (this
// adapter's own "a later page failure discards the whole board" rule, by design).
//
// A first fix (rate-limiting the request START rate alone) was NOT enough: a re-run under
// that fix still failed 10 of 11 boards the same way within ~3 minutes, because a slow
// in-flight request (Firecrawl bypassing Wellfound's Cloudflare challenge takes several
// seconds) overlaps with the next paced request, leaving several requests in flight
// simultaneously despite spaced-out starts. The real ceiling is on CONCURRENT requests, not
// request rate — the same shape whatjobs/trudvsem/emagine already document in pacer.go. Every
// request through the hosted client must share one concurrency cap regardless of how many
// boards or goroutines call Fetch concurrently.
func TestWellfoundPacedThroughFirecrawl(t *testing.T) {
	build, ok := firecrawlProviders["wellfound"]
	if !ok {
		t.Fatal("wellfound is not in firecrawlProviders")
	}
	s := build(&firecrawlClient{}, nil)
	wf, ok := s.(wellfound)
	if !ok {
		t.Fatalf("firecrawlProviders[\"wellfound\"] built a %T, want wellfound", s)
	}
	if _, limited := wf.http.(concurrencyLimitedHTMLGetter); !limited {
		t.Error("wellfound built over the hosted Firecrawl client is not concurrency-limited — " +
			"concurrent boards will collide on Firecrawl's own account-level ceiling, as they did live")
	}
}
