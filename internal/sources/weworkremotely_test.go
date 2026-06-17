package sources

import (
	"context"
	"slices"
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

func TestWeWorkRemotelyBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/weworkremotely.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/weworkremotely.yml fails validation: %v", err)
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
<region>Anywhere in the World</region>
<pubDate>Wed, 17 Jun 2026 17:33:39 +0000</pubDate>
<description>&lt;p&gt;Join us.&lt;/p&gt;</description></item>
<item><title>NoCompanySeparator Role</title>
<link>https://weworkremotely.com/remote-jobs/x-1</link><pubDate>Wed, 17 Jun 2026 17:33:39 +0000</pubDate></item>
</channel></rss>`
	fake := (&routedHTTP{}).route("remote-jobs.rss", feed)
	jobs, err := NewWeWorkRemotely(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the no-separator title dropped)", len(jobs))
	}
	j := jobs[0]
	if j.Company != "Proxify AB" || j.Title != "Senior Fullstack Developer (Python)" {
		t.Errorf("title split wrong: company=%q title=%q", j.Company, j.Title)
	}
	if j.ExternalID != "proxify-ab-senior-fullstack-developer-python-3" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.WorkMode != "remote" || j.PostedAt == nil {
		t.Errorf("WorkMode=%q PostedAt=%v", j.WorkMode, j.PostedAt)
	}
}
