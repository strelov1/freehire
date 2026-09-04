package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestCryptocurrencyJobsProvider(t *testing.T) {
	if got := NewCryptocurrencyJobs(nil).Provider(); got != "cryptocurrencyjobs" {
		t.Errorf("Provider() = %q, want cryptocurrencyjobs", got)
	}
}

func TestCryptocurrencyJobsIsBoardlessAggregator(t *testing.T) {
	s := NewCryptocurrencyJobs(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("cryptocurrencyjobs should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("cryptocurrencyjobs should implement the aggregator marker")
	}
}

func TestCryptocurrencyJobsRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["cryptocurrencyjobs"]; !ok {
		t.Error("All() should register provider cryptocurrencyjobs")
	}
	if !slices.Contains(FilterableProviders(), "cryptocurrencyjobs") {
		t.Error("FilterableProviders() should include cryptocurrencyjobs")
	}
}

// Same feed generator family as nodesk (see the adapter's doc comment): reproduces the raw,
// non-CDATA "&rsquo;" entity shape so a regression back to strict GetXML decoding fails loudly.
func TestCryptocurrencyJobsFetchSplitsTitleAndMaps(t *testing.T) {
	feed := `<?xml version="1.0" encoding="utf-8"?><rss version="2.0"><channel>
<item><title>Senior Platform Engineer at Chronicle</title>
<link>https://cryptocurrencyjobs.co/engineering/chronicle-senior-platform-engineer/</link>
<guid>https://cryptocurrencyjobs.co/engineering/chronicle-senior-platform-engineer/</guid>
<pubDate>Mon, 10 Aug 2026 16:40:02 +0200</pubDate>
<description>Join today&rsquo;s team.</description></item>
<item><title>NoCompanySeparator Role</title>
<link>https://cryptocurrencyjobs.co/x-1/</link>
<guid>https://cryptocurrencyjobs.co/x-1/</guid>
<pubDate>Mon, 10 Aug 2026 16:40:02 +0200</pubDate></item>
</channel></rss>`
	fake := (&routedHTTP{}).route("index.xml", feed)
	jobs, err := NewCryptocurrencyJobs(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the no-separator title dropped)", len(jobs))
	}
	j := jobs[0]
	if j.Company != "Chronicle" || j.Title != "Senior Platform Engineer" {
		t.Errorf("title split wrong: company=%q title=%q", j.Company, j.Title)
	}
	if j.ExternalID != "https://cryptocurrencyjobs.co/engineering/chronicle-senior-platform-engineer/" {
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

// The board is not all-remote: a "(<City> only)" title suffix marks a posting restricted to
// one office (confirmed live for "Product Designer (New York only)" at Loopscale, whose page
// states the role requires working from their NYC office) with no "remote" wording anywhere
// in the feed, unlike a geo-eligibility-restricted remote posting.
func TestCryptocurrencyJobsOnsiteTitleSuffix(t *testing.T) {
	feed := `<?xml version="1.0" encoding="utf-8"?><rss version="2.0"><channel>
<item><title>Product Designer (New York only) at Loopscale</title>
<link>https://cryptocurrencyjobs.co/design/loopscale-product-designer-new-york-only/</link>
<guid>https://cryptocurrencyjobs.co/design/loopscale-product-designer-new-york-only/</guid>
<pubDate>Wed, 05 Aug 2026 08:00:00 +0200</pubDate>
<description>Loopscale is looking to hire a Product Designer to join their team. This is a full-time position.</description></item>
</channel></rss>`
	fake := (&routedHTTP{}).route("index.xml", feed)
	jobs, err := NewCryptocurrencyJobs(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.Title != "Product Designer" {
		t.Errorf("Title = %q, want the (New York only) suffix stripped", j.Title)
	}
	if j.Remote || j.WorkMode != "onsite" || j.Location != "New York" {
		t.Errorf("Remote=%v WorkMode=%q Location=%q, want a non-remote onsite posting in New York", j.Remote, j.WorkMode, j.Location)
	}
}

// The feed's <description> is only the opening blurb; the full posting lives on the item's
// own page as a schema.org JobPosting ld+json block (confirmed live: ~750 vs. ~7000 chars).
func TestCryptocurrencyJobsFetchUsesDetailDescription(t *testing.T) {
	feed := `<?xml version="1.0" encoding="utf-8"?><rss version="2.0"><channel>
<item><title>Senior Platform Engineer at Chronicle</title>
<link>https://cryptocurrencyjobs.co/engineering/chronicle-senior-platform-engineer/</link>
<guid>https://cryptocurrencyjobs.co/engineering/chronicle-senior-platform-engineer/</guid>
<pubDate>Mon, 10 Aug 2026 16:40:02 +0200</pubDate>
<description>Chronicle is looking to hire a Senior Platform Engineer.</description></item>
</channel></rss>`
	fake := (&routedHTTP{}).
		route("index.xml", feed).
		route("chronicle-senior-platform-engineer", jobPostingHTML("Senior Platform Engineer", "<p>The full responsibilities and requirements.</p>"))
	jobs, err := NewCryptocurrencyJobs(fake).Fetch(context.Background(), CompanyEntry{})
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
