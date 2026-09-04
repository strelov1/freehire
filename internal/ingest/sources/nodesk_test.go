package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestNoDeskProvider(t *testing.T) {
	if got := NewNoDesk(nil).Provider(); got != "nodesk" {
		t.Errorf("Provider() = %q, want nodesk", got)
	}
}

func TestNoDeskIsBoardlessAggregator(t *testing.T) {
	s := NewNoDesk(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("nodesk should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("nodesk should implement the aggregator marker")
	}
}

func TestNoDeskRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["nodesk"]; !ok {
		t.Error("All() should register provider nodesk")
	}
	if !slices.Contains(FilterableProviders(), "nodesk") {
		t.Error("FilterableProviders() should include nodesk")
	}
}

// The live feed embeds raw named entities like "&rsquo;" outside CDATA, which the strict
// XML decoder rejects ("invalid character entity") — this fixture reproduces that shape
// rather than a hand-cleaned one, so a regression back to GetXML/strict decoding fails loudly.
func TestNoDeskFetchSplitsTitleAndMaps(t *testing.T) {
	feed := `<?xml version="1.0" encoding="utf-8"?><rss version="2.0"><channel>
<item><title>Senior Backend Engineer at Skylight</title>
<link>https://nodesk.co/remote-jobs/skylight-senior-backend-engineer/</link>
<guid>https://nodesk.co/remote-jobs/skylight-senior-backend-engineer/</guid>
<pubDate>Thu, 06 Aug 2026 08:00:00 +0200</pubDate>
<description>Join today&rsquo;s team.</description></item>
<item><title>NoCompanySeparator Role</title>
<link>https://nodesk.co/remote-jobs/x-1/</link>
<guid>https://nodesk.co/remote-jobs/x-1/</guid>
<pubDate>Thu, 06 Aug 2026 08:00:00 +0200</pubDate></item>
</channel></rss>`
	fake := (&routedHTTP{}).route("index.xml", feed)
	jobs, err := NewNoDesk(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the no-separator title dropped)", len(jobs))
	}
	j := jobs[0]
	if j.Company != "Skylight" || j.Title != "Senior Backend Engineer" {
		t.Errorf("title split wrong: company=%q title=%q", j.Company, j.Title)
	}
	if j.ExternalID != "https://nodesk.co/remote-jobs/skylight-senior-backend-engineer/" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.Description != "Join today’s team." {
		t.Errorf("Description = %q, want the &rsquo; entity resolved to U+2019", j.Description)
	}
	if !j.Remote || j.WorkMode != "remote" || j.Location != "Remote" {
		t.Errorf("Remote=%v WorkMode=%q Location=%q", j.Remote, j.WorkMode, j.Location)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed RFC1123Z timestamp")
	}
}

// The feed's <description> is only the opening blurb; the full posting lives on the item's
// own page as a schema.org JobPosting ld+json block (confirmed live: ~700 vs. ~7000 chars).
func TestNoDeskFetchUsesDetailDescription(t *testing.T) {
	feed := `<?xml version="1.0" encoding="utf-8"?><rss version="2.0"><channel>
<item><title>Senior Backend Engineer at Skylight</title>
<link>https://nodesk.co/remote-jobs/skylight-senior-backend-engineer/</link>
<guid>https://nodesk.co/remote-jobs/skylight-senior-backend-engineer/</guid>
<pubDate>Thu, 06 Aug 2026 08:00:00 +0200</pubDate>
<description>Skylight is looking to hire a Senior Backend Engineer.</description></item>
</channel></rss>`
	fake := (&routedHTTP{}).
		route("index.xml", feed).
		route("skylight-senior-backend-engineer", jobPostingHTML("Senior Backend Engineer", "<p>The full responsibilities and requirements.</p>"))
	jobs, err := NewNoDesk(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if got := jobs[0].Description; !strings.Contains(got, "full responsibilities and requirements") {
		t.Errorf("Description = %q, want the detail page's ld+json description, not the feed blurb", got)
	}
}
