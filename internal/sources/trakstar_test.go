package sources

import (
	"context"
	"strings"
	"testing"
)

func TestTrakstarProvider(t *testing.T) {
	if got := NewTrakstar(nil).Provider(); got != "trakstar" {
		t.Errorf("Provider() = %q, want %q", got, "trakstar")
	}
}

func TestTrakstarFetch(t *testing.T) {
	fake := &fakeHTTP{body: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:job="https://recruiterbox.com/rss/job/">
  <channel>
    <title>Cheesecake Labs Jobs</title>
    <item>
      <title>Senior Go Engineer</title>
      <link>http://cheesecakelabs.hire.trakstar.com/jobs/fk0zjzb</link>
      <guid>http://cheesecakelabs.hire.trakstar.com/jobs/fk0zjzb</guid>
      <pubDate>Thu, 18 Jun 2026 00:00:00 +0530</pubDate>
      <description>&lt;p&gt;Build the &lt;strong&gt;pipeline&lt;/strong&gt;.&lt;/p&gt;</description>
      <job:locationCity>Florian&#243;polis</job:locationCity>
      <job:locationState>SC</job:locationState>
      <job:locationCountry>Brazil</job:locationCountry>
      <job:positionType>full_time</job:positionType>
    </item>
    <item>
      <title>Remote Platform Engineer</title>
      <link>http://cheesecakelabs.hire.trakstar.com/jobs/ab1cdef</link>
      <guid>http://cheesecakelabs.hire.trakstar.com/jobs/ab1cdef</guid>
      <pubDate>Thu, 18 Jun 2026 00:00:00 +0530</pubDate>
      <description>&lt;p&gt;Work from anywhere.&lt;/p&gt;</description>
      <job:locationCity>Remote</job:locationCity>
      <job:locationCountry>Brazil</job:locationCountry>
      <job:positionType>full_time</job:positionType>
    </item>
  </channel>
</rss>`}

	jobs, err := NewTrakstar(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Cheesecake Labs", Provider: "trakstar", Board: "cheesecakelabs",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(fake.gotURL, "cheesecakelabs.hire.trakstar.com/jobfeeds/cheesecakelabs") {
		t.Errorf("requested URL %q should target the board jobfeed", fake.gotURL)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "fk0zjzb" {
		t.Errorf("ExternalID = %q, want the job id parsed from the link", j.ExternalID)
	}
	if j.Title != "Senior Go Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Cheesecake Labs" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	// http→https upgrade on the posting URL.
	if j.URL != "https://cheesecakelabs.hire.trakstar.com/jobs/fk0zjzb" {
		t.Errorf("URL = %q, want the https posting URL", j.URL)
	}
	// Location joins city + state + country, skipping empties.
	if j.Location != "Florianópolis, SC, Brazil" {
		t.Errorf("Location = %q, want city/state/country joined", j.Location)
	}
	// Description is entity-unescaped then sanitized.
	if !strings.Contains(j.Description, "Build the") || !strings.Contains(j.Description, "<strong>pipeline</strong>") {
		t.Errorf("Description = %q, want unescaped + sanitized HTML", j.Description)
	}
	if j.Remote {
		t.Error("Remote = true for a Florianópolis office, want false")
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2026 {
		t.Errorf("PostedAt = %v, want parsed pubDate (2026)", j.PostedAt)
	}

	// Second job: locationCity contains "Remote" → remote, and state is empty so the
	// location join skips it.
	r := jobs[1]
	if !r.Remote {
		t.Error("second job locationCity=Remote should set Remote = true")
	}
	if r.Location != "Remote, Brazil" {
		t.Errorf("Location = %q, want city + country with empty state skipped", r.Location)
	}
}

func TestTrakstarFetchEmptyFeed(t *testing.T) {
	fake := &fakeHTTP{body: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Empty</title></channel></rss>`}
	jobs, err := NewTrakstar(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Way2", Provider: "trakstar", Board: "way2",
	})
	if err != nil {
		t.Fatalf("Fetch on empty feed should not error: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("len(jobs) = %d, want 0 for an empty feed", len(jobs))
	}
}
