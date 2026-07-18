package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// jobspressoFeed mirrors a live jobspresso.co WordPress Job Manager RSS document. The single
// item is a real posting: the title carries an HTML char reference (&#124;), the guid is a URL
// whose numeric post id sits in a `p=` param (the `&#038;` XML-decodes to `&`), the dc:creator
// is a CDATA `Company<br>⚲&nbsp;Location` string, and both a truncated <description> summary and
// a fuller <content:encoded> body are present so the adapter's body preference is verifiable.
const jobspressoFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"
	xmlns:content="http://purl.org/rss/1.0/modules/content/"
	xmlns:dc="http://purl.org/dc/elements/1.1/">
<channel>
	<title>Jobs &#8211; Jobspresso</title>
	<link>https://jobspresso.co</link>
	<item>
		<title>Mitarbeiter/in (m/w/d) f&#252;r Datenanalysen &#124; Remote</title>
		<link>https://jobspresso.co/job/wet-germany-various-mitarbeiter-in-fur-datenanalysen-remote/</link>
		<dc:creator><![CDATA[WET Beratungs- und Beteiligungsgesellschaft mbH<br>⚲&nbsp;Germany , Austria, Luxemburg]]></dc:creator>
		<pubDate>Thu, 16 Jul 2026 15:15:16 +0000</pubDate>
		<guid isPermaLink="false">https://jobspresso.co/?post_type=job_listing&#038;p=163363</guid>
		<description><![CDATA[<p>Wir suchen Sie als Mitarbeiter/in f&uuml;r Datenanalysen im Minijob.</p>]]></description>
		<content:encoded><![CDATA[<p>Wir suchen Sie als Mitarbeiter/in f&uuml;r Datenanalysen.</p><p>Komplett remote &#8211; arbeiten Sie von zu Hause aus.</p>]]></content:encoded>
	</item>
</channel>
</rss>`

func TestJobspressoFetchMapsItem(t *testing.T) {
	fake := &fakeHTTP{body: jobspressoFeed}

	jobs, err := NewJobspresso(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Jobspresso", Provider: "jobspresso",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(fake.gotURL, "jobspresso.co/feed/?post_type=job_listing") {
		t.Errorf("requested URL %q should target the WPJM feed", fake.gotURL)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	j := jobs[0]

	// ExternalID is the numeric post id from the guid's p= param.
	if j.ExternalID != "163363" {
		t.Errorf("ExternalID = %q, want 163363", j.ExternalID)
	}
	if j.URL != "https://jobspresso.co/job/wet-germany-various-mitarbeiter-in-fur-datenanalysen-remote/" {
		t.Errorf("URL = %q, want the item link", j.URL)
	}
	if j.Title != "Mitarbeiter/in (m/w/d) für Datenanalysen | Remote" {
		t.Errorf("Title = %q, want the char-reference-decoded title", j.Title)
	}
	// Company and location split out of dc:creator (Company<br>⚲&nbsp;Location).
	if j.Company != "WET Beratungs- und Beteiligungsgesellschaft mbH" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Germany , Austria, Luxemburg" {
		t.Errorf("Location = %q, want the ⚲-stripped, nbsp-trimmed location", j.Location)
	}
	// The fuller content:encoded body is preferred over the truncated description.
	if !strings.Contains(j.Description, "Komplett remote") {
		t.Errorf("Description should come from content:encoded, got %q", j.Description)
	}
	// jobspresso lists only remote work.
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want true/remote", j.Remote, j.WorkMode)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2026 {
		t.Errorf("PostedAt = %v, want the parsed pubDate (2026)", j.PostedAt)
	}
}

func TestJobspressoID(t *testing.T) {
	cases := []struct {
		name, guid, link, want string
	}{
		{"guid p= param", "https://jobspresso.co/?post_type=job_listing&p=163363", "https://jobspresso.co/job/x/", "163363"},
		{"no p= falls back to link slug", "https://jobspresso.co/job/acme-role/", "https://jobspresso.co/job/acme-role/", "acme-role"},
		{"trailing slash trimmed", "", "https://jobspresso.co/job/acme-role", "acme-role"},
		{"empty guid and link", "", "", ""},
	}
	for _, c := range cases {
		if got := jobspressoID(c.guid, c.link); got != c.want {
			t.Errorf("%s: jobspressoID(%q, %q) = %q, want %q", c.name, c.guid, c.link, got, c.want)
		}
	}
}

func TestJobspressoCompanyLocation(t *testing.T) {
	cases := []struct {
		name, creator, wantCompany, wantLocation string
	}{
		{"company and location", "Acme<br>⚲&nbsp;Remote", "Acme", "Remote"},
		{"multi-region location", "WET mbH<br>⚲&nbsp;Germany , Austria", "WET mbH", "Germany , Austria"},
		{"no br is company-only", "Acme", "Acme", ""},
		{"empty company", "<br>⚲&nbsp;Remote", "", "Remote"},
		// dc:creator is CDATA, so the company half keeps its raw entities until unescaped here.
		{"entity-encoded company", "AT&amp;T<br>⚲&nbsp;Remote", "AT&T", "Remote"},
	}
	for _, c := range cases {
		gotC, gotL := jobspressoCompanyLocation(c.creator)
		if gotC != c.wantCompany || gotL != c.wantLocation {
			t.Errorf("%s: jobspressoCompanyLocation(%q) = (%q, %q), want (%q, %q)",
				c.name, c.creator, gotC, gotL, c.wantCompany, c.wantLocation)
		}
	}
}

// jobspressoEdgeFeed exercises the field-fallback and drop rules: item A has no guid p= (id
// from the link slug); item B has neither guid p= nor a link slug (dropped); item C resolves to
// an empty company (dropped); item D carries only a <description> summary (body falls back to it).
const jobspressoEdgeFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"
	xmlns:content="http://purl.org/rss/1.0/modules/content/"
	xmlns:dc="http://purl.org/dc/elements/1.1/">
<channel>
	<item>
		<title>A Role</title>
		<link>https://jobspresso.co/job/acme-a-role/</link>
		<dc:creator><![CDATA[Acme<br>⚲&nbsp;Worldwide]]></dc:creator>
		<pubDate>Thu, 16 Jul 2026 15:15:16 +0000</pubDate>
		<guid isPermaLink="true">https://jobspresso.co/job/acme-a-role/</guid>
		<content:encoded><![CDATA[<p>Full body A.</p>]]></content:encoded>
	</item>
	<item>
		<title>No Id</title>
		<link></link>
		<dc:creator><![CDATA[Beta<br>⚲&nbsp;Remote]]></dc:creator>
		<guid></guid>
	</item>
	<item>
		<title>No Company</title>
		<link>https://jobspresso.co/job/mystery/</link>
		<dc:creator><![CDATA[<br>⚲&nbsp;Remote]]></dc:creator>
		<guid isPermaLink="false">https://jobspresso.co/?post_type=job_listing&#038;p=99</guid>
	</item>
	<item>
		<title>Summary Only</title>
		<link>https://jobspresso.co/job/delta-role/</link>
		<dc:creator><![CDATA[Delta<br>⚲&nbsp;Canada]]></dc:creator>
		<pubDate>Thu, 16 Jul 2026 15:15:16 +0000</pubDate>
		<guid isPermaLink="false">https://jobspresso.co/?post_type=job_listing&#038;p=42</guid>
		<description><![CDATA[<p>Summary body D.</p>]]></description>
	</item>
</channel>
</rss>`

func TestJobspressoFetchAppliesRules(t *testing.T) {
	jobs, err := NewJobspresso(&fakeHTTP{body: jobspressoEdgeFeed}).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (no-id and no-company items dropped)", len(jobs))
	}
	// Item A: id from the link slug (guid carries no p=).
	if jobs[0].ExternalID != "acme-a-role" {
		t.Errorf("jobs[0].ExternalID = %q, want the link slug acme-a-role", jobs[0].ExternalID)
	}
	// Item D: body falls back to <description> when content:encoded is absent.
	if jobs[1].ExternalID != "42" || !strings.Contains(jobs[1].Description, "Summary body D") {
		t.Errorf("jobs[1] = {id:%q, desc:%q}, want id 42 with the description body", jobs[1].ExternalID, jobs[1].Description)
	}
}

func TestJobspressoProvider(t *testing.T) {
	if got := NewJobspresso(nil).Provider(); got != "jobspresso" {
		t.Errorf("Provider() = %q, want jobspresso", got)
	}
}

func TestJobspressoIsBoardlessAggregator(t *testing.T) {
	s := NewJobspresso(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("jobspresso should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("jobspresso should implement the aggregator marker")
	}
}

func TestJobspressoRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["jobspresso"]; !ok {
		t.Error("All() should register provider jobspresso")
	}
	if !slices.Contains(FilterableProviders(), "jobspresso") {
		t.Error("FilterableProviders() should include jobspresso")
	}
}

func TestJobspressoBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/jobspresso.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/jobspresso.yml fails validation: %v", err)
	}
}
