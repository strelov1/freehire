package sources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// scalisPage1HTML is a trimmed but byte-faithful RSC-flight fixture modeled directly on a
// live-captured Scalis listing page (boldbusiness.scalis.ai/jobs): two postings, each
// carrying its description as a "$<id>" reference into the flight's own text rows.
const scalisPage1HTML = `<html><body>
<script>self.__next_f.push([1,"27:{\"initialData\":{\"results\":[{\"id\":\"aaaa1111-0000-4000-8000-000000000001\",\"title\":\"Senior Backend Engineer\",\"company\":{\"name\":\"BOLD Business\"},\"locations\":[{\"city\":\"Bogota\",\"country\":\"CO\"}],\"employment\":\"FULL_TIME\",\"workplace\":\"REMOTE\",\"payment\":\"SALARY\",\"skills\":[\"Go\",\"Postgres\"],\"salary\":{\"min\":null,\"max\":null,\"currency\":null},\"description\":\"$28\",\"descriptionHtml\":\"$29\",\"createdAt\":\"2026-06-18T17:10:41.385Z\"},{\"id\":\"aaaa1111-0000-4000-8000-000000000002\",\"title\":\"Support Engineer\",\"company\":{\"name\":\"BOLD Business\"},\"locations\":[{\"city\":\"Lima\",\"country\":\"PE\"}],\"employment\":\"CONTRACTOR\",\"workplace\":\"HYBRID\",\"payment\":\"HOURLY\",\"skills\":[],\"salary\":{\"min\":20,\"max\":30,\"currency\":\"USD\"},\"description\":\"$2a\",\"descriptionHtml\":\"$2b\",\"createdAt\":\"2026-07-01T09:00:00.000Z\"}],\"count\":2,\"paginationCount\":2}}\n28:T15,Reports to: Team Lead\n29:T14,<p>Build things.</p>\n2a:Tf,Reports to: CTO\n2b:T15,<p>Ship features.</p>\n"])</script>
</body></html>`

// scalisPage2HTML is a genuinely distinct second page (a different posting, a different
// running count) — TestScalisFetchPaginatesToExhaustion uses this, not a repeat of page 1,
// so the test actually proves cross-page accumulation rather than re-checking single-page
// mapping under a different name.
const scalisPage2HTML = `<html><body>
<script>self.__next_f.push([1,"27:{\"initialData\":{\"results\":[{\"id\":\"aaaa1111-0000-4000-8000-000000000003\",\"title\":\"Data Analyst\",\"company\":{\"name\":\"BOLD Business\"},\"locations\":[{\"city\":\"Manila\",\"country\":\"PH\"}],\"employment\":\"FULL_TIME\",\"workplace\":\"ON_SITE\",\"payment\":\"SALARY\",\"skills\":[\"SQL\"],\"salary\":{\"min\":null,\"max\":null,\"currency\":null},\"description\":\"$28\",\"descriptionHtml\":\"$29\",\"createdAt\":\"2026-08-01T00:00:00.000Z\"}],\"count\":3,\"paginationCount\":3}}\n28:T15,Reports to: Data Lead\n29:T14,<p>Analyze data.</p>\n"])</script>
</body></html>`

// scalisEmptyPageHTML is a listing page past the last real page: an empty results array,
// the confirmed live termination signal (no redirect trap).
const scalisEmptyPageHTML = `<html><body>
<script>self.__next_f.push([1,"1:{\"initialData\":{\"results\":[],\"count\":2,\"paginationCount\":2}}\n"])</script>
</body></html>`

func scalisListingURLFor(page int) string {
	return fmt.Sprintf("https://boldbusiness.scalis.ai/jobs?page=%d&limit=10&sortBy=SORT_BEST_MATCH", page)
}

func TestScalisProvider(t *testing.T) {
	if got := NewScalis(nil).Provider(); got != "scalis" {
		t.Errorf("Provider() = %q, want %q", got, "scalis")
	}
}

func TestScalisFetchSinglePageAndMaps(t *testing.T) {
	fake := (&routedHTTP{}).
		route(scalisListingURLFor(1), scalisPage1HTML).
		route(scalisListingURLFor(2), scalisEmptyPageHTML)

	jobs, err := NewScalis(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Fallback Co", Provider: "scalis", Board: "boldbusiness",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	j1 := jobs[0]
	if j1.ExternalID != "aaaa1111-0000-4000-8000-000000000001" {
		t.Errorf("ExternalID = %q", j1.ExternalID)
	}
	if j1.Title != "Senior Backend Engineer" {
		t.Errorf("Title = %q", j1.Title)
	}
	// A Scalis board is single-tenant, so the posting's own "company" field is never
	// read — the curator-configured name is authoritative, and it must be even when
	// the posting happens to carry a literal company object (see
	// TestScalisFetchIgnoresCompanyBackreferenceString for the case it isn't literal).
	if j1.Company != "Fallback Co" {
		t.Errorf("Company = %q, want the configured CompanyEntry.Company", j1.Company)
	}
	if j1.Location != "Bogota, CO" {
		t.Errorf("Location = %q, want %q", j1.Location, "Bogota, CO")
	}
	if j1.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j1.EmploymentType)
	}
	if j1.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote", j1.WorkMode)
	}
	if !j1.Remote {
		t.Errorf("Remote = false, want true (WorkMode is remote)")
	}
	if strings.Join(j1.Skills, ",") != "Go,Postgres" {
		t.Errorf("Skills = %v", j1.Skills)
	}
	if j1.Description != "<p>Build things.</p>" {
		t.Errorf("Description = %q, want the resolved HTML row", j1.Description)
	}
	if j1.SalaryMin != nil || j1.SalaryMax != nil {
		t.Errorf("SalaryMin/Max = %v/%v, want nil (null in source)", j1.SalaryMin, j1.SalaryMax)
	}

	j2 := jobs[1]
	if j2.EmploymentType != "contract" {
		t.Errorf("EmploymentType = %q, want contract (CONTRACTOR)", j2.EmploymentType)
	}
	if j2.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid", j2.WorkMode)
	}
	if j2.SalaryMin == nil || *j2.SalaryMin != 20 || j2.SalaryMax == nil || *j2.SalaryMax != 30 {
		t.Errorf("SalaryMin/Max = %v/%v, want 20/30", j2.SalaryMin, j2.SalaryMax)
	}
	if j2.SalaryCurrency != "USD" {
		t.Errorf("SalaryCurrency = %q, want USD", j2.SalaryCurrency)
	}
	if j2.SalaryPeriod != "hour" {
		t.Errorf("SalaryPeriod = %q, want hour (HOURLY payment)", j2.SalaryPeriod)
	}
	if j2.Description != "<p>Ship features.</p>" {
		t.Errorf("Description = %q", j2.Description)
	}
}

func TestScalisFetchPaginatesToExhaustion(t *testing.T) {
	fake := (&routedHTTP{}).
		route(scalisListingURLFor(1), scalisPage1HTML).
		route(scalisListingURLFor(2), scalisPage2HTML).
		route(scalisListingURLFor(3), scalisEmptyPageHTML)

	jobs, err := NewScalis(fake).Fetch(context.Background(), CompanyEntry{Board: "boldbusiness"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (the union of page 1's two postings and page 2's one)", len(jobs))
	}
	ids := map[string]bool{}
	for _, j := range jobs {
		ids[j.ExternalID] = true
	}
	for _, want := range []string{
		"aaaa1111-0000-4000-8000-000000000001",
		"aaaa1111-0000-4000-8000-000000000002",
		"aaaa1111-0000-4000-8000-000000000003",
	} {
		if !ids[want] {
			t.Errorf("missing job %q in %v", want, ids)
		}
	}
}

func TestScalisEmptyFirstPageYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route(scalisListingURLFor(1), scalisEmptyPageHTML)
	jobs, err := NewScalis(fake).Fetch(context.Background(), CompanyEntry{Board: "boldbusiness"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

// A later-page failure must abort the whole Fetch, never return a partial result as
// success — the property the fullBoardListing marker rests on.
func TestScalisFetchFailsWholeBoardOnLaterPageError(t *testing.T) {
	fake := (&routedHTTP{}).
		route(scalisListingURLFor(1), scalisPage1HTML).
		routeErr(scalisListingURLFor(2), errors.New("boom"))

	if _, err := NewScalis(fake).Fetch(context.Background(), CompanyEntry{Board: "boldbusiness"}); err == nil {
		t.Fatal("Fetch succeeded despite a later-page listing error")
	}
}

// scalisUnresolvableRefsHTML carries postings whose description references name ids no
// text row on the page actually provides — the shape a marker-format change would produce.
const scalisUnresolvableRefsHTML = `<html><body>
<script>self.__next_f.push([1,"27:{\"initialData\":{\"results\":[{\"id\":\"aaaa1111-0000-4000-8000-000000000009\",\"title\":\"Ghost Role\",\"company\":{\"name\":\"BOLD Business\"},\"locations\":[],\"employment\":\"FULL_TIME\",\"workplace\":\"REMOTE\",\"payment\":\"SALARY\",\"skills\":[],\"salary\":{\"min\":null,\"max\":null,\"currency\":null},\"description\":\"$99\",\"descriptionHtml\":\"$98\",\"createdAt\":\"2026-06-18T17:10:41.385Z\"}],\"count\":1,\"paginationCount\":1}}\n"])</script>
</body></html>`

// A board where every description reference fails to resolve must fail loudly rather than
// ship a board of empty-bodied jobs — the same posture deel.Fetch already takes, since a
// resolution failure this total means the marker format itself likely changed.
func TestScalisFetchFailsWhenAllDescriptionReferencesUnresolved(t *testing.T) {
	fake := (&routedHTTP{}).
		route(scalisListingURLFor(1), scalisUnresolvableRefsHTML).
		route(scalisListingURLFor(2), scalisEmptyPageHTML)

	if _, err := NewScalis(fake).Fetch(context.Background(), CompanyEntry{Board: "boldbusiness"}); err == nil {
		t.Fatal("Fetch succeeded despite every description reference failing to resolve")
	}
}

// A board whose listing never returns an empty page has not proven it ended — reaching the
// safety ceiling must fail the whole Fetch, never quietly return the partial result gathered
// so far (the same truncation bug already found and fixed once in this codebase for
// teamtailor's ttMaxPages).
func TestScalisFetchFailsAtSafetyCeiling(t *testing.T) {
	fake := (&routedHTTP{}).route("boldbusiness.scalis.ai", scalisPage1HTML) // matches every page URL

	_, err := NewScalis(fake).Fetch(context.Background(), CompanyEntry{Board: "boldbusiness"})
	if err == nil {
		t.Fatal("Fetch succeeded despite never seeing an empty page")
	}
	if !strings.Contains(err.Error(), "safety ceiling") {
		t.Errorf("error = %q, want it to name the safety ceiling", err.Error())
	}
}

// scalisFallbackHTML's posting has a descriptionHtml reference to a row that isn't present,
// but its plain-text description reference IS — the adapter must fall back to it rather
// than losing the description entirely.
const scalisFallbackHTML = `<html><body>
<script>self.__next_f.push([1,"27:{\"initialData\":{\"results\":[{\"id\":\"aaaa1111-0000-4000-8000-00000000000a\",\"title\":\"Fallback Role\",\"company\":{\"name\":\"BOLD Business\"},\"locations\":[],\"employment\":\"FULL_TIME\",\"workplace\":\"REMOTE\",\"payment\":\"SALARY\",\"skills\":[],\"salary\":{\"min\":null,\"max\":null,\"currency\":null},\"description\":\"$28\",\"descriptionHtml\":\"$97\",\"createdAt\":\"2026-06-18T17:10:41.385Z\"}],\"count\":1,\"paginationCount\":1}}\n28:Tf,Plain text body\n"])</script>
</body></html>`

func TestScalisDescriptionFallsBackToPlainTextWhenHTMLUnresolved(t *testing.T) {
	fake := (&routedHTTP{}).
		route(scalisListingURLFor(1), scalisFallbackHTML).
		route(scalisListingURLFor(2), scalisEmptyPageHTML)

	jobs, err := NewScalis(fake).Fetch(context.Background(), CompanyEntry{Board: "boldbusiness"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Description != "Plain text body" {
		t.Fatalf("got %+v, want the plain-text description as a fallback", jobs)
	}
}

// scalisWrongShapeHTML's initialData carries "results" as an object, not an array — a
// markup change the adapter has never observed live.
const scalisWrongShapeHTML = `<html><body>
<script>self.__next_f.push([1,"27:{\"initialData\":{\"results\":{}}}\n"])</script>
</body></html>`

func TestScalisFetchFailsOnUnrecognizedPageShape(t *testing.T) {
	fake := (&routedHTTP{}).route(scalisListingURLFor(1), scalisWrongShapeHTML)
	if _, err := NewScalis(fake).Fetch(context.Background(), CompanyEntry{Board: "boldbusiness"}); err == nil {
		t.Fatal("Fetch succeeded despite results being an object, not an array")
	}
}

// scalisDedupedCompanyHTML mirrors a live capture from boldbusiness.scalis.ai: React's RSC
// flight deduplicates repeated identical objects, so every posting after the first has its
// "company" field replaced with a path-backreference STRING ("$5:2:props:...") rather than
// the literal {"name": "..."} object earlier postings carry. This is not a text-row
// reference (nextFlightTextRows never resolves it) — it names a JSON PATH elsewhere in the
// same tree, which no existing decoder helper understands. A struct field trying to
// unmarshal it as an object fails outright: this is the shape that broke prod (issue: scalis
// "boldbusiness" page 1: decode initialData: json: cannot unmarshal string into Go struct
// field scalisPosting.results.company of type struct { Name string "json:\"name\"" }).
const scalisDedupedCompanyHTML = `<html><body>
<script>self.__next_f.push([1,"27:{\"initialData\":{\"results\":[{\"id\":\"aaaa1111-0000-4000-8000-000000000001\",\"title\":\"Senior Backend Engineer\",\"company\":{\"name\":\"BOLD Business\"},\"locations\":[],\"employment\":\"FULL_TIME\",\"workplace\":\"REMOTE\",\"payment\":\"SALARY\",\"skills\":[],\"salary\":{\"min\":null,\"max\":null,\"currency\":null},\"description\":\"$28\",\"descriptionHtml\":\"$29\",\"createdAt\":\"2026-06-18T17:10:41.385Z\"},{\"id\":\"aaaa1111-0000-4000-8000-000000000002\",\"title\":\"Support Engineer\",\"company\":\"$5:2:props:initialData:results:0:company\",\"locations\":[],\"employment\":\"CONTRACTOR\",\"workplace\":\"HYBRID\",\"payment\":\"HOURLY\",\"skills\":[],\"salary\":{\"min\":null,\"max\":null,\"currency\":null},\"description\":\"$2a\",\"descriptionHtml\":\"$2b\",\"createdAt\":\"2026-07-01T09:00:00.000Z\"}],\"count\":2,\"paginationCount\":2}}\n28:T15,Reports to: Team Lead\n29:T14,<p>Build things.</p>\n2a:Tf,Reports to: CTO\n2b:T15,<p>Ship features.</p>\n"])</script>
</body></html>`

func TestScalisFetchIgnoresCompanyBackreferenceString(t *testing.T) {
	fake := (&routedHTTP{}).
		route(scalisListingURLFor(1), scalisDedupedCompanyHTML).
		route(scalisListingURLFor(2), scalisEmptyPageHTML)

	jobs, err := NewScalis(fake).Fetch(context.Background(), CompanyEntry{
		Company: "BOLD Business", Provider: "scalis", Board: "boldbusiness",
	})
	if err != nil {
		t.Fatalf("Fetch: %v, want the dedup-backreference company field to be ignored, not decoded", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	for _, j := range jobs {
		if j.Company != "BOLD Business" {
			t.Errorf("job %q Company = %q, want the configured CompanyEntry.Company", j.ExternalID, j.Company)
		}
	}
}

func TestScalisRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["scalis"] {
		t.Error("FullBoardListingProviders(All(nil)) should include scalis")
	}
}

func TestScalisRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["scalis"]
	if !ok {
		t.Fatal("All() missing provider scalis")
	}
	if s.Provider() != "scalis" {
		t.Errorf("All()[scalis].Provider() = %q", s.Provider())
	}
}
