package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestArbeitnowProvider(t *testing.T) {
	if got := NewArbeitnow(nil).Provider(); got != "arbeitnow" {
		t.Errorf("Provider() = %q, want arbeitnow", got)
	}
}

func TestArbeitnowIsBoardlessAggregator(t *testing.T) {
	s := NewArbeitnow(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("arbeitnow should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("arbeitnow should implement the aggregator marker")
	}
}

func TestArbeitnowRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["arbeitnow"]; !ok {
		t.Error("All() should register provider arbeitnow")
	}
	if !slices.Contains(FilterableProviders(), "arbeitnow") {
		t.Error("FilterableProviders() should include arbeitnow")
	}
}

func TestArbeitnowFetchPaginatesAndMaps(t *testing.T) {
	page1 := `{"data":[
{"slug":"data-engineer-berlin-1","company_name":"Passerelle","title":"Data Engineer","description":"<p>Build &amp; ship.</p>","remote":true,"url":"https://www.arbeitnow.com/jobs/companies/passerelle/data-engineer-berlin-1","location":"Berlin","created_at":1781713837},
{"slug":"","company_name":"NoID","title":"skip me","url":"x","location":"Berlin","created_at":1}
],"links":{"next":"https://www.arbeitnow.com/api/job-board-api?page=2"}}`
	page2 := `{"data":[],"links":{"next":null}}`
	// page=2 routed first so the more specific match wins over the base job-board-api route.
	fake := (&routedHTTP{}).route("page=2", page2).route("job-board-api", page1)

	jobs, err := NewArbeitnow(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the empty-slug posting dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "data-engineer-berlin-1" || j.Company != "Passerelle" || j.Title != "Data Engineer" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("WorkMode=%q Remote=%v, want remote/true", j.WorkMode, j.Remote)
	}
	// The API description is already real HTML, so the valid &amp; entity is preserved.
	if !strings.Contains(j.Description, "Build") || !strings.Contains(j.Description, "ship") {
		t.Errorf("Description lost content: %q", j.Description)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed epoch")
	}
}

// About a tenth of the arbeitnow feed serves the employer's body with its HTML
// entity-encoded, then appends the board's own live-HTML promo footer. sanitizeHTML alone
// cannot recover that: bluemonday reads "&lt;h2&gt;" as a text node and re-encodes it, so
// the tags used to reach the catalogue as literals visible to the reader.
func TestArbeitnowDecodesEntityEncodedDescription(t *testing.T) {
	page1 := `{"data":[
{"slug":"senior-cloud-platform-engineer-140883","company_name":"Prolific","title":"Senior Cloud Platform Engineer","description":"&lt;h2 style=&quot;text-align: center;&quot;&gt;&lt;strong&gt;Senior Cloud Platform Engineer&lt;/strong&gt;&lt;/h2&gt;\n&lt;p&gt;&amp;nbsp;&lt;/p&gt;\n&lt;ul&gt;&lt;li&gt;Kubernetes at scale&lt;/li&gt;&lt;/ul&gt;<p>Find more <a href=\"https://www.arbeitnow.co.uk/english-speaking-jobs\">jobs</a> on Arbeitnow</p>","remote":true,"url":"https://www.arbeitnow.co.uk/jobs/companies/prolific/senior-cloud-platform-engineer-140883","location":"Remote","created_at":1781713837}
],"links":{"next":null}}`
	fake := (&routedHTTP{}).route("job-board-api", page1)

	jobs, err := NewArbeitnow(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	got := jobs[0].Description

	// The encoded body is now real markup the browser renders as structure.
	for _, want := range []string{"<h2>", "<strong>Senior Cloud Platform Engineer</strong>", "<li>Kubernetes at scale</li>"} {
		if !strings.Contains(got, want) {
			t.Errorf("Description missing decoded markup %q\ngot: %s", want, got)
		}
	}
	// No encoded tag may survive, or the reader sees the tag text itself.
	if strings.Contains(got, "&lt;") {
		t.Errorf("Description still carries encoded markup\ngot: %s", got)
	}
	// Decoding turns style="..." into a real attribute, which the policy then drops.
	if strings.Contains(got, "text-align") {
		t.Errorf("Description kept a disallowed style attribute\ngot: %s", got)
	}
	// The board's live-HTML footer keeps its words but loses its link, like every other
	// anchor in the catalogue.
	if want := `<p>Find more jobs on Arbeitnow</p>`; !strings.Contains(got, want) {
		t.Errorf("Description missing the unwrapped footer %q\ngot: %s", want, got)
	}
	if strings.Contains(got, "href=") {
		t.Errorf("Description kept a link\ngot: %s", got)
	}
}
