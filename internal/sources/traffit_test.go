package sources

import (
	"context"
	"strings"
	"testing"
)

func TestTraffitProvider(t *testing.T) {
	if got := NewTraffit(nil).Provider(); got != "traffit" {
		t.Errorf("Provider() = %q, want %q", got, "traffit")
	}
}

func TestTraffitFetchMapsFieldsAndPaginates(t *testing.T) {
	// Page 1 is a full page (traffitPageSize items), so the walk continues; page 2 carries
	// the interesting cases and, once collected, reaches count so no third page is fetched.
	// The reported count is the raw item total (103): the walk pages by it, then drops the
	// id-less posting, so 102 jobs survive.
	var page1 strings.Builder
	page1.WriteString(`{"count":103,"items":[`)
	for i := 0; i < traffitPageSize; i++ {
		if i > 0 {
			page1.WriteString(",")
		}
		page1.WriteString(`{"advertId":`)
		page1.WriteString(itoa(1000 + i))
		page1.WriteString(`,"title":"Role","name":"Role","url":"https://acme.traffit.com/public/an/x`)
		page1.WriteString(itoa(1000 + i))
		page1.WriteString(`"}`)
	}
	page1.WriteString(`]}`)

	// One fully-populated posting, one with a null geolocation, one with no id (dropped).
	page2 := `{"count":103,"items":[
		{"advertId":2001,"title":"  Senior Java Engineer  ","name":"Senior Java Engineer",
		 "url":"https://acme.traffit.com/public/an/abc","validStart":1770284174,
		 "description":"<p>Build things</p><script>evil()</script>",
		 "geolocation":"{\"iso\":\"pl\",\"locality\":\"Warszawa\",\"region1\":\"Mazowieckie\",\"country\":\"Polska\"}"},
		{"advertId":2002,"title":"","name":"Remote Fallback Role",
		 "url":"https://acme.traffit.com/public/an/def","geolocation":null},
		{"advertId":0,"advertPublishId":0,"title":"No Id","url":"https://acme.traffit.com/public/an/ghi"}
	]}`

	fake := (&routedHTTP{}).
		route("offset=0", page1.String()).
		route("offset=100", page2)

	jobs, err := NewTraffit(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "traffit", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// 100 from page 1 + 2 from page 2 (the id-less posting dropped) = 102.
	if len(jobs) != traffitPageSize+2 {
		t.Fatalf("len(jobs) = %d, want %d across two pages with one id-less posting dropped", len(jobs), traffitPageSize+2)
	}
	// Two requests only: the walk reaches count and must not fetch a third page.
	if fake.calls != 2 {
		t.Fatalf("fake.calls = %d, want 2 (no third page once count is reached)", fake.calls)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	if _, ok := byID["0"]; ok {
		t.Error("id-less posting should be dropped, but a job with ExternalID \"0\" was kept")
	}

	full, ok := byID["2001"]
	if !ok {
		t.Fatal("job 2001 missing")
	}
	if full.Title != "Senior Java Engineer" {
		t.Errorf("Title = %q, want trimmed %q", full.Title, "Senior Java Engineer")
	}
	if full.Company != "Acme" {
		t.Errorf("Company = %q, want %q", full.Company, "Acme")
	}
	if strings.Contains(full.Description, "<script") {
		t.Errorf("Description not sanitized: %q", full.Description)
	}
	if want := "Warszawa, Mazowieckie, Polska"; full.Location != want {
		t.Errorf("Location = %q, want %q", full.Location, want)
	}
	if full.PostedAt == nil || full.PostedAt.Year() != 2026 {
		t.Errorf("PostedAt = %v, want a 2026 date from validStart", full.PostedAt)
	}
	if full.URL != "https://acme.traffit.com/public/an/abc" {
		t.Errorf("URL = %q, want the posting url", full.URL)
	}

	fb, ok := byID["2002"]
	if !ok {
		t.Fatal("job 2002 missing")
	}
	if fb.Title != "Remote Fallback Role" {
		t.Errorf("Title = %q, want name fallback %q", fb.Title, "Remote Fallback Role")
	}
	if fb.Location != "" {
		t.Errorf("Location = %q, want empty for null geolocation", fb.Location)
	}
}

func TestTraffitFetchStopsOnEmptyPage(t *testing.T) {
	// count over-reports (999) but the tenant runs out of postings on page 2. An empty page
	// must end the walk instead of spinning up to the page cap.
	var page1 strings.Builder
	page1.WriteString(`{"count":999,"items":[`)
	for i := 0; i < traffitPageSize; i++ {
		if i > 0 {
			page1.WriteString(",")
		}
		page1.WriteString(`{"advertId":`)
		page1.WriteString(itoa(1000 + i))
		page1.WriteString(`,"title":"Role","url":"https://acme.traffit.com/public/an/x"}`)
	}
	page1.WriteString(`]}`)

	fake := (&routedHTTP{}).
		route("offset=0", page1.String()).
		route("offset=100", `{"count":999,"items":[]}`)

	jobs, err := NewTraffit(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != traffitPageSize {
		t.Fatalf("len(jobs) = %d, want %d (empty second page ends the walk)", len(jobs), traffitPageSize)
	}
	if fake.calls != 2 {
		t.Fatalf("fake.calls = %d, want 2 (stop at the empty page)", fake.calls)
	}
}
