package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestCutshortProvider(t *testing.T) {
	if got := NewCutshort(nil).Provider(); got != "cutshort" {
		t.Errorf("Provider() = %q, want cutshort", got)
	}
}

func TestCutshortIsBoardlessAggregatorFullBoard(t *testing.T) {
	s := NewCutshort(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("cutshort should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("cutshort should implement the aggregator marker")
	}
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("cutshort should implement the fullBoardListing marker")
	}
}

func TestCutshortRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["cutshort"]; !ok {
		t.Error("All() should register provider cutshort")
	}
	if !slices.Contains(FilterableProviders(), "cutshort") {
		t.Error("FilterableProviders() should include cutshort")
	}
}

func TestCutshortFetchCollapsesCitiesAndMaps(t *testing.T) {
	feed := `<?xml version="1.0" encoding="utf-8"?>
<source>
<job>
	<jobtype><![CDATA[fulltime]]></jobtype>
	<country><![CDATA[India]]></country>
	<title><![CDATA[AWS Developer]]></title>
	<date><![CDATA[2026-11-10T12:40:49.580Z]]></date>
	<partnerJobId><![CDATA[6af310d10ec665ea0ed441c7]]></partnerJobId>
	<url><![CDATA[https://cutshort.io/job/AWS-Developer-TeleApps-DjBYmMJP]]></url>
	<applyUrl><![CDATA[https://cutshort.io/job/AWS-Developer-TeleApps-DjBYmMJP?utm_source=freehire-feed&utm_medium=xml]]></applyUrl>
	<company><![CDATA[TeleApps]]></company>
	<city><![CDATA[Bengaluru (Bangalore)]]></city>
	<location><![CDATA[Bengaluru (Bangalore)]]></location>
	<workplaceTypes><![CDATA[On-site]]></workplaceTypes>
	<description><![CDATA[<p>Build AWS infra.</p>]]></description>
	<salaries>
		<salary>
			<lowEnd><amount><![CDATA[700000]]></amount><currencyCode>INR</currencyCode></lowEnd>
			<highEnd><amount><![CDATA[1100000]]></amount><currencyCode>INR</currencyCode></highEnd>
			<period><![CDATA[yearly]]></period>
		</salary>
	</salaries>
	<experienceLevel>MID_SENIOR_LEVEL</experienceLevel>
	<experience><![CDATA[4-7]]></experience>
</job>
<job>
	<jobtype><![CDATA[fulltime]]></jobtype>
	<country><![CDATA[India]]></country>
	<title><![CDATA[AWS Developer]]></title>
	<date><![CDATA[2026-11-10T12:40:49.580Z]]></date>
	<partnerJobId><![CDATA[6af310d10ec665ea0ed441c7]]></partnerJobId>
	<url><![CDATA[https://cutshort.io/job/AWS-Developer-TeleApps-DjBYmMJP]]></url>
	<applyUrl><![CDATA[https://cutshort.io/job/AWS-Developer-TeleApps-DjBYmMJP?utm_source=freehire-feed&utm_medium=xml]]></applyUrl>
	<company><![CDATA[TeleApps]]></company>
	<city><![CDATA[Chennai]]></city>
	<location><![CDATA[Chennai]]></location>
	<workplaceTypes><![CDATA[On-site]]></workplaceTypes>
	<description><![CDATA[<p>Build AWS infra.</p>]]></description>
	<experienceLevel>MID_SENIOR_LEVEL</experienceLevel>
	<experience><![CDATA[4-7]]></experience>
</job>
<job>
	<jobtype><![CDATA[fulltime]]></jobtype>
	<country><![CDATA[India]]></country>
	<title><![CDATA[E3D Piping Designer]]></title>
	<date><![CDATA[2026-11-10T12:31:15.886Z]]></date>
	<partnerJobId><![CDATA[6af30e93a577fa82d26f5fcc]]></partnerJobId>
	<url><![CDATA[https://cutshort.io/job/E3D-Piping-Designer-JfA5BpPy]]></url>
	<applyUrl><![CDATA[https://cutshort.io/job/E3D-Piping-Designer-JfA5BpPy?utm_source=freehire-feed]]></applyUrl>
	<company><![CDATA[OwnThriveProner Consultants]]></company>
	<city><![CDATA[India]]></city>
	<isRemote>Yes</isRemote>
	<workplaceTypes>Remote</workplaceTypes>
	<location><![CDATA[India]]></location>
	<description><![CDATA[<p>Piping design.</p>]]></description>
	<experienceLevel>ASSOCIATE</experienceLevel>
	<experience><![CDATA[1-3]]></experience>
</job>
<job>
	<title><![CDATA[No Identity]]></title>
	<company><![CDATA[Acme]]></company>
	<url><![CDATA[https://cutshort.io/job/x]]></url>
</job>
</source>`
	fake := (&routedHTTP{}).route("jobs_data_for_freehire.xml", feed)
	jobs, err := NewCutshort(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (city rows collapsed, no-id dropped)", len(jobs))
	}
	aws := jobs[0]
	if aws.ExternalID != "6af310d10ec665ea0ed441c7" || aws.Company != "TeleApps" {
		t.Errorf("aws identity = %q %q", aws.ExternalID, aws.Company)
	}
	if aws.Location != "Bengaluru (Bangalore); Chennai" {
		t.Errorf("Location = %q, want collapsed cities", aws.Location)
	}
	if !strings.Contains(aws.URL, "utm_source=freehire-feed") {
		t.Errorf("URL should prefer applyUrl with UTMs, got %q", aws.URL)
	}
	if aws.WorkMode != "onsite" || aws.EmploymentType != "full_time" || aws.Seniority != "middle" {
		t.Errorf("facets = mode=%q type=%q seniority=%q", aws.WorkMode, aws.EmploymentType, aws.Seniority)
	}
	if aws.SalaryMin == nil || *aws.SalaryMin != 700000 || aws.SalaryMax == nil || *aws.SalaryMax != 1100000 ||
		aws.SalaryCurrency != "INR" || aws.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %s/%s", aws.SalaryMin, aws.SalaryMax, aws.SalaryCurrency, aws.SalaryPeriod)
	}
	if aws.ExperienceYearsMin == nil || *aws.ExperienceYearsMin != 4 {
		t.Errorf("ExperienceYearsMin = %v, want 4", aws.ExperienceYearsMin)
	}
	if !slices.Equal(aws.Countries, []string{"in"}) || aws.PostedAt == nil {
		t.Errorf("Countries=%v PostedAt=%v", aws.Countries, aws.PostedAt)
	}

	remote := jobs[1]
	if !remote.Remote || remote.WorkMode != "remote" || remote.Seniority != "junior" {
		t.Errorf("remote = Remote=%v WorkMode=%q Seniority=%q", remote.Remote, remote.WorkMode, remote.Seniority)
	}
}

func TestCutshortSeniorityAndEmploymentHelpers(t *testing.T) {
	cases := map[string]string{
		"INTERNSHIP": "intern", "ENTRY_LEVEL": "junior", "ASSOCIATE": "junior",
		"MID_SENIOR_LEVEL": "middle", "DIRECTOR": "", "EXECUTIVE": "", "": "",
	}
	for in, want := range cases {
		if got := cutshortSeniority(in); got != want {
			t.Errorf("cutshortSeniority(%q) = %q, want %q", in, got, want)
		}
	}
	if got := cutshortEmploymentType("fulltime"); got != "full_time" {
		t.Errorf("employment fulltime = %q", got)
	}
}

func TestCutshortRowEdgeCases(t *testing.T) {
	base := func() cutshortItem {
		return cutshortItem{PartnerJobID: "id1", Company: "Acme", Title: "Dev", URL: "https://cutshort.io/job/x", Country: "India"}
	}

	noCountry := base()
	noCountry.Country = ""
	if job, ok := noCountry.toJob(); !ok || job.Countries != nil || !slices.Equal(job.FallbackCountries, []string{"in"}) {
		t.Errorf("missing country: Countries=%v FallbackCountries=%v", job.Countries, job.FallbackCountries)
	}

	for name, band := range map[string]cutshortSalary{
		"no period":    {Period: ""},
		"empty amount": {Period: "yearly"},
		"not a number": func() cutshortSalary {
			var b cutshortSalary
			b.Period, b.LowEnd.Amount, b.HighEnd.Amount = "yearly", "seven lakh", "n/a"
			return b
		}(),
	} {
		it := base()
		it.Salaries = []cutshortSalary{band}
		job, _ := it.toJob()
		if job.SalaryMin != nil || job.SalaryMax != nil || job.SalaryPeriod != "" || job.SalaryCurrency != "" {
			t.Errorf("%s: salary should stay unset, got %v-%v %s/%s", name, job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod)
		}
	}
}

func TestCutshortFetchRemoteCityRowWins(t *testing.T) {
	row := func(city, mode string) string {
		return `<job><partnerJobId>id1</partnerJobId><company>Acme</company><title>Dev</title>` +
			`<url>https://cutshort.io/job/x</url><country>India</country><city>` + city +
			`</city><workplaceTypes>` + mode + `</workplaceTypes></job>`
	}
	feed := `<source>` + row("Pune", "On-site") + row("Delhi", "Remote") + `</source>`
	jobs, err := NewCutshort((&routedHTTP{}).route("jobs_data_for_freehire.xml", feed)).Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("Fetch: %d jobs, err %v", len(jobs), err)
	}
	if j := jobs[0]; !j.Remote || j.WorkMode != "remote" || j.Location != "Pune; Delhi" {
		t.Errorf("merged = Remote=%v WorkMode=%q Location=%q", j.Remote, j.WorkMode, j.Location)
	}
}
