package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// avatureDetailHTML mirrors an Avature JobDetail page: og:title carries the title, and the
// body is a series of article__content__view__field blocks, each a __label + __value. The
// description is the richest (longest) field value and carries markup to verify sanitizing.
const avatureDetailHTML = `<html><head>
<meta property="og:title" content="Senior Product Manager"/>
<meta property="og:site_name" content="Electronic Arts"/>
</head><body class="body--job-detail">
<div class='article__content'>
  <div class='article__content__view__field'>
    <div class='article__content__view__field__label'>Role ID</div>
    <div class='article__content__view__field__value'>214840</div>
  </div>
  <div class='article__content__view__field'>
    <div class='article__content__view__field__label'>Work Model</div>
    <div class='article__content__view__field__value'>Hybrid</div>
  </div>
  <div class='article__content__view__field'>
    <div class='article__content__view__field__value'><strong>Locations</strong>: Austin, Texas, United States of America&nbsp; <br></div>
  </div>
  <div class='article__content__view__field'>
    <div class='article__content__view__field__value'><h2>About</h2><p>Build the games that connect the world. We are looking for a Senior Product Manager to lead our roadmap, partner with engineering, and ship features players love every single day.</p><script>alert(1)</script></div>
  </div>
</div></body></html>`

func avatureSitemapIndex(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><sitemapindex>`)
	for _, l := range locs {
		b.WriteString(`<sitemap><loc>` + l + `</loc></sitemap>`)
	}
	b.WriteString(`</sitemapindex>`)
	return b.String()
}

func avatureURLset(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><urlset>`)
	for _, l := range locs {
		b.WriteString(`<url><loc>` + l + `</loc><lastmod>2026-06-06</lastmod></url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

func TestAvatureProvider(t *testing.T) {
	if got := NewAvature(nil).Provider(); got != "avature" {
		t.Errorf("Provider() = %q, want %q", got, "avature")
	}
}

func TestAvatureJobID(t *testing.T) {
	cases := map[string]string{
		"https://jobs.ea.com/en_US/careers/JobDetail/Senior-Product-Manager/214840": "214840",
		"https://jobs.ea.com/en_US/careers/JobDetail/Quality-Designer/208332/":      "208332",
		"https://jobs.ea.com/en_US/careers/JobDetail/No-Id":                         "",
	}
	for loc, want := range cases {
		if got := avatureJobID(loc); got != want {
			t.Errorf("avatureJobID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestAvatureFetchSelectsEnUSSitemapAndMaps(t *testing.T) {
	job := "https://jobs.ea.com/en_US/careers/JobDetail/Senior-Product-Manager/214840"
	util := "https://jobs.ea.com/en_US/careers/AgentCreate" // non-JobDetail utility page, must be skipped
	fake := (&routedHTTP{}).
		route("/careers/sitemap_index.xml", avatureSitemapIndex(
			"https://jobs.ea.com/en_US/careers/sitemap.xml",
			"https://jobs.ea.com/es_ES/careers/sitemap.xml",
		)).
		route("/en_US/careers/sitemap.xml", avatureURLset(job, util)).
		route("/JobDetail/Senior-Product-Manager/214840", avatureDetailHTML)

	jobs, err := NewAvature(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Electronic Arts", Provider: "avature", Board: "jobs.ea.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (utility page must be filtered)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "214840" {
		t.Errorf("ExternalID = %q, want 214840", j.ExternalID)
	}
	if j.URL != job {
		t.Errorf("URL = %q, want %q", j.URL, job)
	}
	if j.Title != "Senior Product Manager" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Electronic Arts" {
		t.Errorf("Company = %q, want config company", j.Company)
	}
	if j.Location != "Austin, Texas, United States of America" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid", j.WorkMode)
	}
	if j.Remote {
		t.Error("Remote = true, want false for a hybrid posting")
	}
	if strings.Contains(j.Description, "<script>") || !strings.Contains(j.Description, "<h2>About</h2>") {
		t.Errorf("Description not sanitized/assembled: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-06", j.PostedAt)
	}
}

func TestAvatureWorkModeNormalization(t *testing.T) {
	cases := map[string]string{
		"Remote":       "remote",
		"Fully Remote": "remote",
		"Hybrid":       "hybrid",
		"On-site":      "onsite",
		"Onsite":       "onsite",
		"In-office":    "onsite",
		"":             "",
		"Flexible":     "", // unknown → no structured signal
	}
	for in, want := range cases {
		if got := avatureWorkMode(in); got != want {
			t.Errorf("avatureWorkMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAvatureLabeledValueTakesPrimaryLocation(t *testing.T) {
	// A multi-location posting appends a hidden location list after the primary value,
	// separated by a non-breaking space; only the first (primary) location is kept.
	h := `<div class="article__content__view__field__value"><strong>Locations</strong>: Redwood City, California, United States of America&nbsp; Location: AustinState: TexasCountry: United States of America</div>`
	root := parseHTML(t, h)
	if got := avatureLabeledValue(root, "Locations"); got != "Redwood City, California, United States of America" {
		t.Errorf("avatureLabeledValue = %q, want primary location only", got)
	}
}

func TestAvatureDescriptionIgnoresLongLocationDump(t *testing.T) {
	// A many-location posting renders a long concatenated location dump inside the Locations
	// field value; it must not be mistaken for the (shorter) job description, which is the
	// value carrying prose markup.
	h := `<div class='article__content'>
	  <div class='article__content__view__field'><div class='article__content__view__field__value'><strong>Locations</strong>: A, B&nbsp; Location: CState: DCountry: ELocation: FState: GCountry: HLocation: IState: JCountry: KLocation: LState: MCountry: NLocation: OState: PCountry: Q</div></div>
	  <div class='article__content__view__field'><div class='article__content__view__field__value'><p>Short job body.</p></div></div>
	</div>`
	root := parseHTML(t, h)
	got := avatureDescription(root)
	if !strings.Contains(got, "Short job body.") || strings.Contains(got, "Location:") {
		t.Errorf("description picked the location dump instead of the prose body: %q", got)
	}
}

func TestAvatureDropsJobWithNoParseableID(t *testing.T) {
	loc := "https://jobs.ea.com/en_US/careers/JobDetail/No-Numeric-Id"
	fake := (&routedHTTP{}).
		route("/careers/sitemap_index.xml", avatureSitemapIndex("https://jobs.ea.com/en_US/careers/sitemap.xml")).
		route("/en_US/careers/sitemap.xml", avatureURLset(loc)).
		route("/JobDetail/No-Numeric-Id", avatureDetailHTML)
	jobs, err := NewAvature(fake).Fetch(context.Background(), CompanyEntry{Board: "jobs.ea.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0 (unparseable id dropped)", len(jobs))
	}
}

func TestAvatureMissingEnUSSitemapErrors(t *testing.T) {
	// The index must advertise an en_US locale sitemap; without it the board can't be crawled,
	// and robots.txt (routed here to advertise nothing beyond /careers/) offers no alternative.
	fake := (&routedHTTP{}).
		route("/careers/sitemap_index.xml", avatureSitemapIndex("https://jobs.ea.com/es_ES/careers/sitemap.xml")).
		route("/robots.txt", "Sitemap: https://jobs.ea.com/careers/sitemap_index.xml\n")
	_, err := NewAvature(fake).Fetch(context.Background(), CompanyEntry{Board: "jobs.ea.com"})
	if err == nil {
		t.Fatal("expected error when no en_US sitemap is advertised")
	}
}

func TestAvatureFallsBackToRobotsTxtPortalWhenCareersIsEmpty(t *testing.T) {
	// Qatar Airways' actual shape: the conventional /careers/ portal doesn't exist on this
	// tenant at all (no route registered → GetXML errors), but robots.txt advertises a
	// same-host "global" portal that carries the real public listing — and that portal's index
	// has no locale split at all (a single sub-sitemap, no /en_US/ or any other locale segment
	// in it), unlike EA's per-locale index. The adapter must fall back to the robots.txt portal
	// and accept its lone sub-sitemap as-is.
	job := "https://careers.qatarairways.com/global/JobDetail/Administration-Coordinator/27906"
	fake := (&routedHTTP{}).
		route("/robots.txt", strings.Join([]string{
			"User-agent: *",
			"Sitemap: https://qatarairways.avature.net/hiringmanager/sitemap_index.xml", // different host: must be skipped
			"Sitemap: https://careers.qatarairways.com/global/sitemap_index.xml",
		}, "\n")).
		route("/global/sitemap_index.xml", avatureSitemapIndex(
			"https://careers.qatarairways.com/global/sitemap.xml",
		)).
		route("/global/sitemap.xml", avatureURLset(job)).
		route("/JobDetail/Administration-Coordinator/27906", avatureDetailHTML)

	jobs, err := NewAvature(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Qatar Airways", Board: "careers.qatarairways.com",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (fallback portal must be crawled)", len(jobs))
	}
	if jobs[0].URL != job {
		t.Errorf("URL = %q, want %q", jobs[0].URL, job)
	}
}

func TestAvatureNoLivePortalAnywhereErrors(t *testing.T) {
	// Bain's shape: /careers/ doesn't exist, and every same-host portal robots.txt advertises
	// (jobs, recruits, bainonyourcampus) resolves to an index with no en_US JobDetail postings.
	// A tenant with no reachable public listing must error, not silently return zero jobs.
	fake := (&routedHTTP{}).
		route("/robots.txt", strings.Join([]string{
			"Sitemap: https://careers.bain.com/jobs/sitemap_index.xml",
			"Sitemap: https://careers.bain.com/recruits/sitemap_index.xml",
		}, "\n")).
		route("/jobs/sitemap_index.xml", avatureSitemapIndex("https://careers.bain.com/en_US/jobs/sitemap.xml")).
		route("/en_US/jobs/sitemap.xml", avatureURLset("https://careers.bain.com/en_US/jobs/AgentCreate")).
		route("/recruits/sitemap_index.xml", avatureSitemapIndex("https://careers.bain.com/en_US/recruits/sitemap.xml")).
		route("/en_US/recruits/sitemap.xml", avatureURLset("https://careers.bain.com/en_US/recruits/SearchPositions"))

	_, err := NewAvature(fake).Fetch(context.Background(), CompanyEntry{Board: "careers.bain.com"})
	if err == nil {
		t.Fatal("expected error when no candidate portal carries any JobDetail postings")
	}
}

func TestAvatureSkipsRobotsTxtWhenCareersAlreadyWorks(t *testing.T) {
	// The common case (every board onboarded before robots.txt discovery existed): /careers/
	// resolves directly to postings, so robots.txt must never be consulted — no route is
	// registered for it, so the adapter would error if it tried.
	job := "https://jobs.ea.com/en_US/careers/JobDetail/Senior-Product-Manager/214840"
	fake := (&routedHTTP{}).
		route("/careers/sitemap_index.xml", avatureSitemapIndex("https://jobs.ea.com/en_US/careers/sitemap.xml")).
		route("/en_US/careers/sitemap.xml", avatureURLset(job)).
		route("/JobDetail/Senior-Product-Manager/214840", avatureDetailHTML)

	jobs, err := NewAvature(fake).Fetch(context.Background(), CompanyEntry{Board: "jobs.ea.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
}
