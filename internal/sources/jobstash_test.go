package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// jobstashJob builds one inline posting fragment (body inline — no detail call). access is
// "public" (url is the downstream ATS link) or "protected" (url is the JobStash page).
func jobstashJob(shortUUID, title, org, url, access, location, locationType string, ts int64, responsibilities, requirements string) string {
	return `{"shortUUID":"` + shortUUID + `","title":"` + title +
		`","organization":{"name":"` + org + `"},"url":"` + url +
		`","access":"` + access + `","location":"` + location +
		`","locationType":"` + locationType + `","timestamp":` + itoa64(ts) +
		`,"description":"Build things.","responsibilities":["` + responsibilities +
		`"],"requirements":["` + requirements + `"],"benefits":[]}`
}

// jobstashListPage wraps postings in the page/count/total/data envelope.
func jobstashListPage(total int, jobs ...string) string {
	return `{"page":1,"count":` + itoa(len(jobs)) + `,"total":` + itoa(total) +
		`,"data":[` + strings.Join(jobs, ",") + `]}`
}

func itoa64(n int64) string { return itoa(int(n)) }

func TestJobStashProvider(t *testing.T) {
	if got := NewJobStash(nil).Provider(); got != "jobstash" {
		t.Errorf("Provider() = %q, want %q", got, "jobstash")
	}
}

func TestJobStashIsBoardlessAggregator(t *testing.T) {
	s := NewJobStash(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("jobstash should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("jobstash should implement the aggregator marker (multi-company)")
	}
}

func TestJobStashRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["jobstash"]; !ok {
		t.Error("All() should register provider jobstash")
	}
	if !slices.Contains(FilterableProviders(), "jobstash") {
		t.Error("FilterableProviders() should include the jobstash aggregator")
	}
}

func TestJobStashBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/jobstash.yml")
	if err != nil {
		t.Fatalf("LoadConfig(sources/jobstash.yml): %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/jobstash.yml fails registry validation: %v", err)
	}
}

func TestJobStashFetchPaginatesAndMaps(t *testing.T) {
	// total=300 with page size 200: page 1 then page 2 (after which len(all)=2... the
	// loop stops on the empty page 3). Each posting's body is inline — no detail call.
	fake := (&routedHTTP{}).
		route("page=1", jobstashListPage(300,
			jobstashJob("Z2IGg7", "Associate Tech Lead", "KAST",
				"https://kastcard.pinpointhq.com/en/postings/abc", "public",
				"Remote", "HYBRID", 1781521356188,
				"Design back-end components", "6+ years experience"),
		)).
		route("page=2", jobstashListPage(300,
			jobstashJob("PqCopo", "Head of Capital", "Symbiotic",
				"https://jobstash.xyz/jobs/PqCopo/details", "protected",
				"New York", "REMOTE", 1746057600000,
				"Lead fundraising", "Network"),
		)).
		route("page=3", jobstashListPage(300)) // empty tail page ends pagination

	jobs, err := NewJobStash(fake).Fetch(context.Background(), CompanyEntry{Provider: "jobstash"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 across two pages", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	// Public posting: company from organization.name, url is the downstream ATS link.
	a, ok := byID["Z2IGg7"]
	if !ok {
		t.Fatal("posting Z2IGg7 missing")
	}
	if a.Title != "Associate Tech Lead" {
		t.Errorf("Title = %q", a.Title)
	}
	if a.Company != "KAST" {
		t.Errorf("Company = %q, want organization.name KAST", a.Company)
	}
	if want := "https://kastcard.pinpointhq.com/en/postings/abc"; a.URL != want {
		t.Errorf("URL = %q, want downstream ATS link %q", a.URL, want)
	}
	if a.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid from locationType HYBRID", a.WorkMode)
	}
	for _, want := range []string{"Build things", "Design back-end components", "6+ years experience"} {
		if !strings.Contains(a.Description, want) {
			t.Errorf("Description missing %q, got %q", want, a.Description)
		}
	}
	// HYBRID work mode, but the location text "Remote" still flags the job remote
	// (the isRemote fallback wins over a non-remote work mode).
	if !a.Remote {
		t.Error("Remote = false, want true: location text 'Remote' triggers the isRemote fallback")
	}
	if a.PostedAt == nil || a.PostedAt.Year() != 2026 {
		t.Errorf("PostedAt = %v, want parsed from epoch-ms timestamp", a.PostedAt)
	}

	// Protected posting: url is the JobStash detail page; REMOTE → remote work mode + flag.
	b := byID["PqCopo"]
	if want := "https://jobstash.xyz/jobs/PqCopo/details"; b.URL != want {
		t.Errorf("protected URL = %q, want JobStash page %q", b.URL, want)
	}
	if b.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote from locationType REMOTE", b.WorkMode)
	}
	if !b.Remote {
		t.Error("Remote = false, want true for a REMOTE posting")
	}
}

// On /jobs/list a protected posting can carry url:null (unlike /public/jobs). The adapter
// falls back to the JobStash detail page built from shortUUID, so the job keeps a working
// link instead of an empty URL.
func TestJobStashProtectedNullURLFallsBackToJobStashPage(t *testing.T) {
	nullURLJob := `{"shortUUID":"UHWdXV","title":"Head of Product","organization":{"name":"Layer3"},` +
		`"url":null,"access":"protected","location":"","locationType":"REMOTE","timestamp":1746057600000,` +
		`"description":"d","responsibilities":[],"requirements":[],"benefits":[]}`
	fake := (&routedHTTP{}).route("page=1", `{"total":1,"data":[`+nullURLJob+`]}`)

	jobs, err := NewJobStash(fake).Fetch(context.Background(), CompanyEntry{Provider: "jobstash"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if want := "https://jobstash.xyz/jobs/UHWdXV/details"; jobs[0].URL != want {
		t.Errorf("URL = %q, want synthesized JobStash page %q for a null-url protected posting", jobs[0].URL, want)
	}
}

// A posting with no company (empty organization.name) or no native id would break the
// company slug / dedup key, so it is dropped rather than persisted.
func TestJobStashSkipsPostingWithoutCompanyOrID(t *testing.T) {
	noCompany := `{"shortUUID":"X1","title":"T","organization":{"name":""},"url":"https://x.co/a",` +
		`"access":"public","location":"","locationType":"","timestamp":0,"description":"d",` +
		`"responsibilities":[],"requirements":[],"benefits":[]}`
	noID := `{"shortUUID":"","title":"T2","organization":{"name":"Acme"},"url":"https://x.co/b",` +
		`"access":"public","location":"","locationType":"","timestamp":0,"description":"d",` +
		`"responsibilities":[],"requirements":[],"benefits":[]}`
	fake := (&routedHTTP{}).route("page=1", `{"total":2,"data":[`+noCompany+`,`+noID+`]}`)

	jobs, err := NewJobStash(fake).Fetch(context.Background(), CompanyEntry{Provider: "jobstash"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("len(jobs) = %d, want 0 (both postings are unusable)", len(jobs))
	}
}

func TestJobStashEmptyListYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route("page=1", jobstashListPage(0))

	jobs, err := NewJobStash(fake).Fetch(context.Background(), CompanyEntry{Provider: "jobstash"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}
