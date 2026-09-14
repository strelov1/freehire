package sources

import "testing"

// TestWellfoundPacedThroughFirecrawl guards against a real production incident: crawling
// several role-slice boards for one provider in a single ingest run hits the pipeline's own
// per-board concurrency, and unpaced requests through the hosted Firecrawl client collided
// hard enough to blow Firecrawl's own account-level rate ceiling — 4 of 11 boards failed
// with "vendor rate limit did not lift after 4 attempts" within the first three minutes of a
// live run (2026-09-14), each discarding an otherwise-successful partial crawl (this
// adapter's own "a later page failure discards the whole board" rule, by design). Every
// request through the hosted client must share one limiter regardless of how many boards or
// goroutines call Fetch concurrently.
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
	if _, paced := wf.http.(rateLimitedHTMLGetter); !paced {
		t.Error("wellfound built over the hosted Firecrawl client is not paced — concurrent " +
			"boards will collide on Firecrawl's own rate ceiling, as they did live")
	}
}
