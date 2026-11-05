package sources

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// doverSlugResp is a minimal careers-page-slug/{slug} fixture: only the client id matters to
// the adapter.
const doverSlugResp = `{"id":"72c2afbf-36e4-4eec-8d78-5d2600498018","slug":"qompyl","name":"Qompyl"}`

// doverDeepspatialSlugResp is the Deepspatial slug resolution as the live API answered it
// 2026-11-05: client id 1aeeba55-…, the company behind the submitted jobs.dover.com link —
// whose link uuid (a5546acf-…) is a POSTING id, not this client id.
const doverDeepspatialSlugResp = `{"id":"1aeeba55-65e0-4803-b70f-fac7ca8a20c7","slug":"deepspatial","name":"Deepspatial"}`

func doverListingPage(jobsJSON, next string) string {
	nextField := "null"
	if next != "" {
		nextField = `"` + next + `"`
	}
	return `{"count":1,"next":` + nextField + `,"previous":null,"results":[` + jobsJSON + `]}`
}

const doverJobPublished = `{"id":"f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5","title":"Cloud Infra Engineer","locations":[],"workplace_type":"REMOTE","is_published":true,"is_sample":false}`
const doverJobSample = `{"id":"sample-1","title":"Sample Job","locations":[],"workplace_type":"REMOTE","is_published":true,"is_sample":true}`
const doverJobUnpublished = `{"id":"unpub-1","title":"Unpublished Job","locations":[],"workplace_type":"REMOTE","is_published":false,"is_sample":false}`

func doverDetail(id string, overrides ...string) string {
	base := map[string]string{
		"id":                        `"` + id + `"`,
		"title":                     `"Cloud Infra Engineer"`,
		"user_provided_description": `"<p>Do things</p>"`,
		"locations":                 `[]`,
		"workplace_type":            `"REMOTE"`,
		"active":                    `true`,
		"is_private":                `false`,
		"compensation":              `{"employment_type":"PART_TIME","open_to_sharing_comp":false}`,
		"visa_support":              `false`,
	}
	for i := 0; i+1 < len(overrides); i += 2 {
		base[overrides[i]] = overrides[i+1]
	}
	out := "{"
	first := true
	for _, k := range []string{"id", "title", "user_provided_description", "locations", "workplace_type", "active", "is_private", "compensation", "visa_support"} {
		if !first {
			out += ","
		}
		first = false
		out += `"` + k + `":` + base[k]
	}
	return out + "}"
}

func TestDoverFetchNewResolvesListsAndHydrates(t *testing.T) {
	fake := &routedHTTP{}
	fake.route("careers-page-slug/qompyl", doverSlugResp)
	fake.route("careers-page/72c2afbf-36e4-4eec-8d78-5d2600498018/jobs", doverListingPage(doverJobPublished, ""))
	fake.route("inbound/application-portal-job/f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5", doverDetail("f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5"))

	jobs, err := NewDover(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "qompyl", Company: "Qompyl"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %d: %+v", len(jobs), jobs)
	}
	j := jobs[0]
	if j.ExternalID != "f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.URL != "https://app.dover.com/apply/qompyl/f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Company != "Qompyl" {
		t.Errorf("Company = %q, want configured board company", j.Company)
	}
	if j.Description == "" {
		t.Errorf("Description empty, want hydrated body")
	}
}

func TestDoverFetchNewFollowsPagination(t *testing.T) {
	fake := &routedHTTP{}
	fake.route("careers-page-slug/qompyl", doverSlugResp)
	page1 := `{"count":2,"next":null,"previous":null,"results":[{"id":"second-job","title":"Second","locations":[],"workplace_type":"REMOTE","is_published":true,"is_sample":false}]}`
	// The more specific "next page" route must be registered before the general listing
	// route, since routedHTTP matches in registration order (see manatal_test.go).
	fake.route("offset=300", page1)
	page0 := `{"count":2,"next":"https://app.dover.com/api/v1/careers-page/72c2afbf-36e4-4eec-8d78-5d2600498018/jobs?limit=300&offset=300","previous":null,"results":[` + doverJobPublished + `]}`
	fake.route("careers-page/72c2afbf-36e4-4eec-8d78-5d2600498018/jobs", page0)
	fake.route("inbound/application-portal-job/f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5", doverDetail("f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5"))
	fake.route("inbound/application-portal-job/second-job", doverDetail("second-job"))

	jobs, err := NewDover(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "qompyl", Company: "Qompyl"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("want 2 jobs across both pages, got %d: %+v", len(jobs), jobs)
	}
}

func TestDoverFetchNewExcludesSampleAndUnpublished(t *testing.T) {
	fake := &routedHTTP{}
	fake.route("careers-page-slug/qompyl", doverSlugResp)
	listing := doverListingPage(doverJobPublished+","+doverJobSample+","+doverJobUnpublished, "")
	fake.route("careers-page/72c2afbf-36e4-4eec-8d78-5d2600498018/jobs", listing)
	fake.route("inbound/application-portal-job/f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5", doverDetail("f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5"))

	jobs, err := NewDover(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "qompyl", Company: "Qompyl"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want only the published, non-sample job, got %d: %+v", len(jobs), jobs)
	}
	if jobs[0].ExternalID != "f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5" {
		t.Errorf("unexpected survivor: %+v", jobs[0])
	}
}

func TestDoverFetchNewSkipsDetailForSeenPostings(t *testing.T) {
	fake := &routedHTTP{}
	fake.route("careers-page-slug/qompyl", doverSlugResp)
	fake.route("careers-page/72c2afbf-36e4-4eec-8d78-5d2600498018/jobs", doverListingPage(doverJobPublished, ""))
	// Deliberately no detail route registered for this posting id: if FetchNew calls it for a
	// seen posting, routedHTTP.GetJSON errors ("no route"), which the assertion below catches.
	jobs, err := NewDover(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "qompyl", Company: "Qompyl"}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].SeenRefresh {
		t.Fatalf("want one SeenRefresh job with no detail call, got %+v", jobs)
	}
}

func TestDoverFetchNewDropsInactiveAndPrivate(t *testing.T) {
	fake := &routedHTTP{}
	fake.route("careers-page-slug/qompyl", doverSlugResp)
	listing := doverListingPage(doverJobPublished, "")
	fake.route("careers-page/72c2afbf-36e4-4eec-8d78-5d2600498018/jobs", listing)
	fake.route("inbound/application-portal-job/f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5",
		doverDetail("f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5", "active", "false"))

	jobs, err := NewDover(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "qompyl", Company: "Qompyl"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("want inactive posting dropped, got %+v", jobs)
	}
}

func TestDoverFetchNewFallsBackToListOnlyOnDetailError(t *testing.T) {
	fake := &routedHTTP{}
	fake.route("careers-page-slug/qompyl", doverSlugResp)
	fake.route("careers-page/72c2afbf-36e4-4eec-8d78-5d2600498018/jobs", doverListingPage(doverJobPublished, ""))
	fake.routeErr("inbound/application-portal-job/f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5", errors.New("dover: transport boom"))

	jobs, err := NewDover(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "qompyl", Company: "Qompyl"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want the posting kept list-only, got %+v", jobs)
	}
	if jobs[0].Description != "" {
		t.Errorf("want empty description on a failed detail fetch, got %q", jobs[0].Description)
	}
	if jobs[0].ExternalID != "f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5" {
		t.Errorf("want listing identity preserved, got %+v", jobs[0])
	}
}

func doverFetchOne(t *testing.T, detailJSON string) Job {
	t.Helper()
	fake := &routedHTTP{}
	fake.route("careers-page-slug/qompyl", doverSlugResp)
	fake.route("careers-page/72c2afbf-36e4-4eec-8d78-5d2600498018/jobs", doverListingPage(doverJobPublished, ""))
	fake.route("inbound/application-portal-job/f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5", detailJSON)
	jobs, err := NewDover(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "qompyl", Company: "Qompyl"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want exactly 1 job, got %d: %+v", len(jobs), jobs)
	}
	return jobs[0]
}

func TestDoverMapsCountryTypedLocations(t *testing.T) {
	detail := doverDetail("f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5", "locations",
		`[{"location_type":"REMOTE","location_option":{"display_name":"Brazil","location_type":"COUNTRY","country":"BR"}},`+
			`{"location_type":"REMOTE","location_option":{"display_name":"International","location_type":"REGION","country":""}}]`)
	job := doverFetchOne(t, detail)
	if len(job.Countries) != 1 || job.Countries[0] != "br" {
		t.Errorf("Countries = %+v, want [\"br\"] (region-typed entry excluded)", job.Countries)
	}
	if job.WorkMode != "remote" || !job.Remote {
		t.Errorf("WorkMode/Remote = %q/%v, want remote/true", job.WorkMode, job.Remote)
	}
}

func TestDoverMapsEmploymentType(t *testing.T) {
	detail := doverDetail("f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5", "compensation",
		`{"employment_type":"FULL_TIME","open_to_sharing_comp":false}`)
	job := doverFetchOne(t, detail)
	if job.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", job.EmploymentType)
	}
}

func TestDoverUnrecognizedEmploymentTypeLeftUnset(t *testing.T) {
	detail := doverDetail("f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5", "compensation",
		`{"employment_type":"SOMETHING_NEW","open_to_sharing_comp":false}`)
	job := doverFetchOne(t, detail)
	if job.EmploymentType != "" {
		t.Errorf("EmploymentType = %q, want empty for an unrecognized value", job.EmploymentType)
	}
}

func TestDoverSalaryPublishedOnlyWhenSharingOptedIn(t *testing.T) {
	shared := doverDetail("f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5", "compensation",
		`{"lower_bound":80000,"upper_bound":120000,"currency_code":"USD","open_to_sharing_comp":true,"employment_type":"FULL_TIME"}`)
	job := doverFetchOne(t, shared)
	if job.SalaryMin == nil || *job.SalaryMin != 80000 || job.SalaryMax == nil || *job.SalaryMax != 120000 || job.SalaryCurrency != "USD" {
		t.Errorf("want published salary range, got min=%v max=%v currency=%q", job.SalaryMin, job.SalaryMax, job.SalaryCurrency)
	}
}

func TestDoverSalaryWithheldWhenNotSharingOptedIn(t *testing.T) {
	notShared := doverDetail("f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5", "compensation",
		`{"lower_bound":80000,"upper_bound":120000,"currency_code":"USD","open_to_sharing_comp":false,"employment_type":"FULL_TIME"}`)
	job := doverFetchOne(t, notShared)
	if job.SalaryMin != nil || job.SalaryMax != nil || job.SalaryCurrency != "" {
		t.Errorf("want no salary published when not opted in, got min=%v max=%v currency=%q", job.SalaryMin, job.SalaryMax, job.SalaryCurrency)
	}
}

func TestDoverFoldsEquityAndVisaIntoDescription(t *testing.T) {
	detail := doverDetail("f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5",
		"compensation", `{"employment_type":"PART_TIME","open_to_sharing_comp":false,"offers_equity":true}`,
		"visa_support", "true")
	job := doverFetchOne(t, detail)
	if !strings.Contains(job.Description, "equity") {
		t.Errorf("Description = %q, want it to mention equity", job.Description)
	}
	if !strings.Contains(job.Description, "visa") {
		t.Errorf("Description = %q, want it to mention visa sponsorship", job.Description)
	}
}

func TestDoverNoExtraTextWithoutEquityOrVisa(t *testing.T) {
	job := doverFetchOne(t, doverDetail("f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5"))
	if strings.Contains(job.Description, "equity") || strings.Contains(job.Description, "visa") {
		t.Errorf("Description = %q, want no equity/visa text when neither fact is stated", job.Description)
	}
}

// TestDoverFetchNewJobsDoverBoardCrawlsThroughSlug pins the jobs.dover.com board shape: the
// recognizer files such links under the Org slug alone (the uuid in the link may be a posting
// id — on the submitted Deepspatial link it is — so the slug lookup is the adapter's one
// supported address path, and it doubles as the existence check the host's marketing shell
// cannot provide). A board spelled by a jobs.dover.com link must crawl exactly like an
// app.dover.com board, yielding /apply/<slug>/<jobId> URLs.
func TestDoverFetchNewJobsDoverBoardCrawlsThroughSlug(t *testing.T) {
	fake := &routedHTTP{}
	fake.route("careers-page-slug/Deepspatial", doverDeepspatialSlugResp)
	fake.route("careers-page/1aeeba55-65e0-4803-b70f-fac7ca8a20c7/jobs", doverListingPage(doverJobPublished, ""))
	fake.route("inbound/application-portal-job/f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5", doverDetail("f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5"))

	jobs, err := NewDover(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "Deepspatial", Company: "Deepspatial"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %d: %+v", len(jobs), jobs)
	}
	if want := "https://app.dover.com/apply/Deepspatial/f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5"; jobs[0].URL != want {
		t.Errorf("URL = %q, want %q", jobs[0].URL, want)
	}
}

// TestDoverFetchNewPlainSlugStillResolves pins the app.dover.com board shape: the slug lookup
// is the adapter's resolve step and stays untouched by the jobs.dover.com support.
func TestDoverFetchNewPlainSlugStillResolves(t *testing.T) {
	fake := &routedHTTP{}
	fake.route("careers-page-slug/qompyl", doverSlugResp)
	fake.route("careers-page/72c2afbf-36e4-4eec-8d78-5d2600498018/jobs", doverListingPage(doverJobPublished, ""))
	fake.route("inbound/application-portal-job/f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5", doverDetail("f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5"))

	jobs, err := NewDover(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Board: "qompyl", Company: "Qompyl"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %d: %+v", len(jobs), jobs)
	}
}
