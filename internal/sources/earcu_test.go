package sources

import (
	"context"
	"encoding/xml"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestEarcuProvider(t *testing.T) {
	if got := NewEarcu(nil).Provider(); got != "earcu" {
		t.Errorf("Provider() = %q, want %q", got, "earcu")
	}
}

// earcuFeed is a minimal two-item eArcu RSS document plus one item whose link carries no
// posting id (must be dropped). The description mirrors the live feed: a "Key: value, …"
// metadata prefix followed by the escaped rssjobdesc body.
const earcuFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <title>Jobs at Cambridge</title>
  <link>https://careers.cambridge.org/jobs/</link>
  <item>
    <guid isPermaLink="true">https://careers.cambridge.org/jobs/vacancy/transfer-pricing-manager-cambridge/7384/description/</guid>
    <link>https://careers.cambridge.org/jobs/vacancy/transfer-pricing-manager-cambridge/7384/description/</link>
    <title>Transfer Pricing Manager (7367)</title>
    <description>Country: UK, Business Unit: Finance, Salary: &#163;75,300-&#163;90,000, Location: Cambridge, eArcu Vacancy Reference: 7367&lt;div id="rssjobdesc"&gt;Job Title: Transfer Pricing Manager&lt;br /&gt;Build the finance pipeline.&lt;/div&gt;</description>
    <pubDate>Tue, 07 Jul 2026 15:53:15 Z</pubDate>
  </item>
  <item>
    <guid isPermaLink="true">https://careers.cambridge.org/jobs/vacancy/senior-go-engineer-remote/7375/description/</guid>
    <link>https://careers.cambridge.org/jobs/vacancy/senior-go-engineer-remote/7375/description/</link>
    <title>Senior Go Engineer (7358)</title>
    <description>Country: UK, Location: Remote, eArcu Vacancy Reference: 7358&lt;div id="rssjobdesc"&gt;Work from anywhere.&lt;/div&gt;</description>
    <pubDate>Mon, 06 Jul 2026 09:00:00 Z</pubDate>
  </item>
  <item>
    <guid isPermaLink="true">https://careers.cambridge.org/jobs/</guid>
    <link>https://careers.cambridge.org/jobs/</link>
    <title>No id here</title>
    <description>Location: Cambridge, eArcu Vacancy Reference: 0&lt;div id="rssjobdesc"&gt;x&lt;/div&gt;</description>
    <pubDate>Mon, 06 Jul 2026 09:00:00 Z</pubDate>
  </item>
</channel></rss>`

func TestEarcuFetch(t *testing.T) {
	fake := &fakeHTTP{body: earcuFeed}

	jobs, err := NewEarcu(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Cambridge University Press & Assessment", Provider: "earcu", Board: "careers.cambridge.org",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(fake.gotURL, "careers.cambridge.org/jobs/rss") {
		t.Errorf("requested URL %q should target the board rss feed", fake.gotURL)
	}
	// The third item has no posting id in its link and must be dropped.
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (id-less item dropped)", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "7384" {
		t.Errorf("ExternalID = %q, want the id from the URL path", j.ExternalID)
	}
	if j.Title != "Transfer Pricing Manager" {
		t.Errorf("Title = %q, want the reference suffix stripped", j.Title)
	}
	if j.Company != "Cambridge University Press & Assessment" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if j.URL != "https://careers.cambridge.org/jobs/vacancy/transfer-pricing-manager-cambridge/7384/description/" {
		t.Errorf("URL = %q, want the item link", j.URL)
	}
	if j.Location != "Cambridge" {
		t.Errorf("Location = %q, want the metadata Location value", j.Location)
	}
	if !strings.Contains(j.Description, "Build the finance pipeline.") {
		t.Errorf("Description missing the inline body, got %q", j.Description)
	}
	if strings.Contains(j.Description, "Country:") {
		t.Errorf("Description should be the rssjobdesc body only, not the metadata prefix: %q", j.Description)
	}
	if j.Remote {
		t.Error("Remote = true for a Cambridge location, want false")
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2026 {
		t.Errorf("PostedAt = %v, want the parsed pubDate (2026)", j.PostedAt)
	}
	if !jobs[1].Remote {
		t.Error("second job Location=Remote should set Remote=true")
	}
}

// earcuRouted is a fake that returns the RSS feed for GetXML and a separate detail HTML
// body (or error) for GetHTML, so the JSON-LD body fallback and its failure path can be
// exercised.
type earcuRouted struct {
	feed      string
	detail    string
	detailErr error
}

func (f *earcuRouted) GetXML(_ context.Context, _ string, v any) error {
	return xml.Unmarshal([]byte(f.feed), v)
}

func (f *earcuRouted) GetHTML(_ context.Context, _ string) (*html.Node, error) {
	if f.detailErr != nil {
		return nil, f.detailErr
	}
	return html.Parse(strings.NewReader(f.detail))
}

func TestEarcuDetailFallback(t *testing.T) {
	// Feed item with no rssjobdesc body → adapter must fetch the detail page's JobPosting.
	feed := `<?xml version="1.0"?><rss version="2.0"><channel>
  <item>
    <link>https://careers.cambridge.org/jobs/vacancy/no-body-role/7400/description/</link>
    <title>No Body Role (7399)</title>
    <description>Country: UK, Location: London, eArcu Vacancy Reference: 7399</description>
    <pubDate>Tue, 07 Jul 2026 15:53:15 Z</pubDate>
  </item>
</channel></rss>`
	detail := `<html><head>
    <script type="application/ld+json">{"@type":"JobPosting","description":"<p>Detail body from JSON-LD.</p>"}</script>
    </head><body></body></html>`

	jobs, err := NewEarcu(&earcuRouted{feed: feed, detail: detail}).Fetch(context.Background(), CompanyEntry{
		Company: "Cambridge", Provider: "earcu", Board: "careers.cambridge.org",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if !strings.Contains(jobs[0].Description, "Detail body from JSON-LD.") {
		t.Errorf("Description = %q, want the JSON-LD fallback body", jobs[0].Description)
	}
}

// TestEarcuLocation covers the metadata-prefix parsing: a comma-bearing Location value,
// the Country fallback when Location is absent, and that a stray "Country:" in the HTML
// body never leaks into the location (parsing is scoped to the prefix).
func TestEarcuLocation(t *testing.T) {
	feed := `<?xml version="1.0"?><rss version="2.0"><channel>
  <item>
    <link>https://careers.cambridge.org/jobs/vacancy/multi-part/201/description/</link>
    <title>Multi Part (200)</title>
    <description>Country: UK, Location: London, UK, eArcu Vacancy Reference: 200&lt;div id="rssjobdesc"&gt;Body.&lt;/div&gt;</description>
    <pubDate>Tue, 07 Jul 2026 15:53:15 Z</pubDate>
  </item>
  <item>
    <link>https://careers.cambridge.org/jobs/vacancy/country-only/203/description/</link>
    <title>Country Only (202)</title>
    <description>Country: UK, eArcu Vacancy Reference: 202&lt;div id="rssjobdesc"&gt;Country: Ireland is mentioned in the body.&lt;/div&gt;</description>
    <pubDate>Tue, 07 Jul 2026 15:53:15 Z</pubDate>
  </item>
</channel></rss>`

	jobs, err := NewEarcu(&earcuRouted{feed: feed}).Fetch(context.Background(), CompanyEntry{
		Company: "Cambridge", Provider: "earcu", Board: "careers.cambridge.org",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}
	if jobs[0].Location != "London, UK" {
		t.Errorf("Location = %q, want the full comma-bearing value %q", jobs[0].Location, "London, UK")
	}
	if jobs[1].Location != "UK" {
		t.Errorf("Location = %q, want the Country fallback %q (body 'Country: Ireland' must not leak)", jobs[1].Location, "UK")
	}
}

// TestEarcuDetailFetchFailureNonFatal asserts a body-less item whose detail fetch fails is
// still emitted (empty description), not dropped and not aborting the board.
func TestEarcuDetailFetchFailureNonFatal(t *testing.T) {
	feed := `<?xml version="1.0"?><rss version="2.0"><channel>
  <item>
    <link>https://careers.cambridge.org/jobs/vacancy/no-body/300/description/</link>
    <title>No Body (299)</title>
    <description>Location: London, eArcu Vacancy Reference: 299</description>
    <pubDate>Tue, 07 Jul 2026 15:53:15 Z</pubDate>
  </item>
</channel></rss>`

	jobs, err := NewEarcu(&earcuRouted{feed: feed, detailErr: context.DeadlineExceeded}).Fetch(context.Background(), CompanyEntry{
		Company: "Cambridge", Provider: "earcu", Board: "careers.cambridge.org",
	})
	if err != nil {
		t.Fatalf("Fetch should not fail when a detail fetch errors: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 (item kept despite detail-fetch failure)", len(jobs))
	}
	if jobs[0].Description != "" {
		t.Errorf("Description = %q, want empty after a failed detail fetch", jobs[0].Description)
	}
}

// TestEarcuPubDateUnpadded covers an unpadded RFC822 day, which the padded RFC1123 layouts
// reject; the adapter's primary layout must still parse it.
func TestEarcuPubDateUnpadded(t *testing.T) {
	if got := earcuPubDate("Fri, 3 Jul 2026 09:00:00 Z"); got == nil || got.UTC().Day() != 3 {
		t.Errorf("earcuPubDate(unpadded day) = %v, want a parsed 2026-07-03", got)
	}
}

func TestEarcuRegisteredInAll(t *testing.T) {
	if _, ok := All(nil)["earcu"]; !ok {
		t.Error("earcu adapter not registered in All")
	}
}
