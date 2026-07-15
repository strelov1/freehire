package sources

import (
	"context"
	"strings"
	"testing"
)

func TestLikeitProvider(t *testing.T) {
	if got := NewLikeit(nil).Provider(); got != "likeit" {
		t.Errorf("Provider() = %q, want %q", got, "likeit")
	}
}

// likeitFeed mirrors a live Likeit AdvertRss.jsp document: dc-namespaced, one item per
// open advert with the posting id in the AdvertShow link's query, the location in
// <category>, an escaped HTML <description>, and both a pubDate and dc:date. The second
// item carries no <category> (location empty) and the third's link has no id (dropped).
const likeitFeed = `<?xml version="1.0" encoding="utf-8"?>
<rss xmlns:dc="http://purl.org/dc/elements/1.1/" version="2.0">
  <channel>
    <title>Työilmoitukset</title>
    <link>https://rekrymesta.likeit.fi/jsp/duuni/AdvertList.jsp?guest=1</link>
    <description />
    <item>
      <title>Nuorempi LVI-asentaja Pirkanmaalle</title>
      <link>https://rekrymesta.likeit.fi/jsp/duuni/AdvertShow.jsp?id=8074439&amp;guest=1</link>
      <description>&lt;p&gt;Haemme &lt;strong&gt;LVI-asentajaa&lt;/strong&gt; Sein&amp;auml;joelle.&lt;/p&gt;</description>
      <category>Pirkanmaa, Tampere</category>
      <pubDate>Wed, 15 Jul 2026 05:39:11 GMT</pubDate>
      <guid>https://rekrymesta.likeit.fi/jsp/duuni/AdvertShow.jsp?id=8074439&amp;guest=1</guid>
      <dc:date>2026-07-15T05:39:11Z</dc:date>
    </item>
    <item>
      <title>Avoin hakemus</title>
      <link>https://rekrymesta.likeit.fi/jsp/duuni/AdvertShow.jsp?id=2145755&amp;guest=1</link>
      <description>&lt;p&gt;Lähetä avoin hakemus.&lt;/p&gt;</description>
      <pubDate>Tue, 14 Jul 2026 09:00:00 GMT</pubDate>
      <guid>https://rekrymesta.likeit.fi/jsp/duuni/AdvertShow.jsp?id=2145755&amp;guest=1</guid>
      <dc:date>2026-07-14T09:00:00Z</dc:date>
    </item>
    <item>
      <title>No id here</title>
      <link>https://rekrymesta.likeit.fi/jsp/duuni/AdvertList.jsp?guest=1</link>
      <description>&lt;p&gt;x&lt;/p&gt;</description>
      <category>Uusimaa, Helsinki</category>
      <pubDate>Tue, 14 Jul 2026 09:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`

func TestLikeitFetch(t *testing.T) {
	fake := &fakeHTTP{body: likeitFeed}

	jobs, err := NewLikeit(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Rekrymesta", Provider: "likeit", Board: "rekrymesta",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// The board is the tenant subdomain; the feed is the guest AdvertRss endpoint.
	if !strings.Contains(fake.gotURL, "rekrymesta.likeit.fi/jsp/duuni/AdvertRss.jsp") {
		t.Errorf("requested URL %q should target the board's AdvertRss feed", fake.gotURL)
	}
	// The third item's link carries no posting id and must be dropped.
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (id-less item dropped)", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "8074439" {
		t.Errorf("ExternalID = %q, want the id from the AdvertShow query", j.ExternalID)
	}
	if j.Title != "Nuorempi LVI-asentaja Pirkanmaalle" {
		t.Errorf("Title = %q, want the item title", j.Title)
	}
	if j.Company != "Rekrymesta" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if j.URL != "https://rekrymesta.likeit.fi/jsp/duuni/AdvertShow.jsp?id=8074439&guest=1" {
		t.Errorf("URL = %q, want the item link", j.URL)
	}
	if j.Location != "Pirkanmaa, Tampere" {
		t.Errorf("Location = %q, want the category value", j.Location)
	}
	// The description is double-escaped (XML then HTML entity); the Finnish umlaut must survive.
	if !strings.Contains(j.Description, "Seinäjoelle") {
		t.Errorf("Description should decode the Finnish umlaut, got %q", j.Description)
	}
	if j.Remote {
		t.Error("Remote = true for a Pirkanmaa location, want false")
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2026 {
		t.Errorf("PostedAt = %v, want the parsed dc:date (2026)", j.PostedAt)
	}
	if jobs[1].Location != "" {
		t.Errorf("second job Location = %q, want empty (no category)", jobs[1].Location)
	}
}
