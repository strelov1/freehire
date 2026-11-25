package sources

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestWeWorkRemotelyProvider(t *testing.T) {
	if got := NewWeWorkRemotely(nil).Provider(); got != "weworkremotely" {
		t.Errorf("Provider() = %q, want weworkremotely", got)
	}
}

func TestWeWorkRemotelyIsBoardlessAggregator(t *testing.T) {
	s := NewWeWorkRemotely(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("weworkremotely should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("weworkremotely should implement the aggregator marker")
	}
}

func TestWeWorkRemotelyRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["weworkremotely"]; !ok {
		t.Error("All() should register provider weworkremotely")
	}
	if !slices.Contains(FilterableProviders(), "weworkremotely") {
		t.Error("FilterableProviders() should include weworkremotely")
	}
}

func TestWeWorkRemotelyJobID(t *testing.T) {
	cases := map[string]string{
		"https://weworkremotely.com/remote-jobs/proxify-ab-senior-fullstack-developer-python-3": "proxify-ab-senior-fullstack-developer-python-3",
		"https://weworkremotely.com/remote-jobs/foo/":                                           "foo",
		"": "",
	}
	for link, want := range cases {
		if got := wwrJobID(link); got != want {
			t.Errorf("wwrJobID(%q) = %q, want %q", link, got, want)
		}
	}
}

func TestWeWorkRemotelyFetchSplitsTitleAndMaps(t *testing.T) {
	feed := `<rss version="2.0"><channel>
<item><title>Proxify AB: Senior Fullstack Developer (Python)</title>
<link>https://weworkremotely.com/remote-jobs/proxify-ab-senior-fullstack-developer-python-3</link>
<region>Anywhere in the World</region><country></country><state>London</state>
<skills>Outreach, Sales</skills><category>Sales and Marketing</category><type>Contract</type>
<pubDate>Wed, 17 Jun 2026 17:33:39 +0000</pubDate>
<description>&lt;p&gt;Join us.&lt;/p&gt;</description></item>
<item><title>NoCompanySeparator Role</title>
<link>https://weworkremotely.com/remote-jobs/x-1</link><pubDate>Wed, 17 Jun 2026 17:33:39 +0000</pubDate></item>
</channel></rss>`
	fake := (&routedHTTP{}).route("weworkremotely.com/", feed)
	jobs, err := NewWeWorkRemotely(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the no-separator title dropped; the same feed answering every URL de-duplicates by slug)", len(jobs))
	}
	j := jobs[0]
	if j.Company != "Proxify AB" || j.Title != "Senior Fullstack Developer (Python)" {
		t.Errorf("title split wrong: company=%q title=%q", j.Company, j.Title)
	}
	if j.ExternalID != "proxify-ab-senior-fullstack-developer-python-3" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.EmploymentType != "contract" || j.Location != "Anywhere in the World" {
		t.Errorf("EmploymentType=%q Location=%q", j.EmploymentType, j.Location)
	}
	if j.WorkMode != "remote" || j.PostedAt == nil {
		t.Errorf("WorkMode=%q PostedAt=%v", j.WorkMode, j.PostedAt)
	}
}

func TestWeWorkRemotelyReadsEveryCategoryFeedAndDedupes(t *testing.T) {
	item := func(slug string) string {
		return `<item><title>Co: ` + slug + `</title><link>https://weworkremotely.com/remote-jobs/` + slug + `</link>` +
			`<region>USA Only</region><type>Full-Time</type><pubDate>Wed, 17 Jun 2026 17:33:39 +0000</pubDate></item>`
	}
	feed := func(items ...string) string {
		return `<rss><channel>` + strings.Join(items, "") + `</channel></rss>`
	}
	fake := (&routedHTTP{}).
		route("weworkremotely.com/remote-jobs.rss", feed(item("a-1"), item("b-2"))).
		route("/categories/remote-design-jobs.rss", feed(item("b-2"), item("c-3"))).
		route("/categories/all-other-remote-jobs.rss", feed(item("d-4")))
	// every other category feed is empty, but must still be requested
	for _, c := range weworkremotelyCategoryFeeds {
		if c != "remote-design-jobs" && c != "all-other-remote-jobs" {
			fake.route("/categories/"+c+".rss", feed())
		}
	}
	jobs, err := NewWeWorkRemotely(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	var ids []string
	for _, j := range jobs {
		ids = append(ids, j.ExternalID)
		if j.EmploymentType != "full_time" {
			t.Errorf("%s EmploymentType = %q", j.ExternalID, j.EmploymentType)
		}
	}
	slices.Sort(ids)
	if want := []string{"a-1", "b-2", "c-3", "d-4"}; !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}

func TestWeWorkRemotelyFailedCategoryFailsTheCrawl(t *testing.T) {
	fake := (&routedHTTP{}).route("weworkremotely.com/remote-jobs.rss", `<rss><channel></channel></rss>`).
		routeErr("/categories/remote-design-jobs.rss", errors.New("boom"))
	for _, c := range weworkremotelyCategoryFeeds {
		if c != "remote-design-jobs" {
			fake.route("/categories/"+c+".rss", `<rss><channel></channel></rss>`)
		}
	}
	if _, err := NewWeWorkRemotely(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a failed category feed must fail the crawl")
	}
}
