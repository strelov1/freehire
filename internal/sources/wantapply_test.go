package sources

import (
	"context"
	"strings"
	"testing"
)

func TestWantapplyVacancySlug(t *testing.T) {
	cases := map[string]string{
		"https://wantapply.cy/android-team-lead-at-tradingview-3":   "android-team-lead-at-tradingview-3",
		"https://wantapply.cy/head-of-qa-remote-only-at-cloudlinux": "head-of-qa-remote-only-at-cloudlinux",
		"https://wantapply.cy/company/tradingview":                  "", // company page, multi-segment
		"https://wantapply.cy/jobs/data-analyst":                    "", // taxonomy, multi-segment
		"https://wantapply.cy":                                      "", // root
		"https://wantapply.cy/":                                     "", // root with slash
		"https://wantapply.cy/create":                               "", // reserved static
		"https://wantapply.cy/sign-in":                              "", // reserved static
		"https://wantapply.cy/sign-up":                              "", // reserved static
		"https://wantapply.cy/privacy-policy":                       "", // reserved static
		"https://wantapply.cy/terms-of-service":                     "", // reserved static
		"https://example.com/some-role-at-acme":                     "", // foreign host
		"::not a url::":                                             "",
	}
	for loc, want := range cases {
		if got := wantapplyVacancySlug(loc); got != want {
			t.Errorf("wantapplyVacancySlug(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestWantapplyPostingLocation(t *testing.T) {
	// City + region + country join; empty parts are skipped; duplicate places collapse.
	p := wantapplyPosting{JobLocation: []wantapplyPlace{
		{Address: wantapplyAddress{AddressLocality: "Limassol", AddressCountry: "Cyprus"}},
		{Address: wantapplyAddress{AddressCountry: "Cyprus"}},
	}}
	if got, want := p.location(), "Limassol, Cyprus; Cyprus"; got != want {
		t.Errorf("location() = %q, want %q", got, want)
	}
	if got := (wantapplyPosting{}).location(); got != "" {
		t.Errorf("location() = %q, want empty", got)
	}
	// Duplicate places collapse to one entry.
	dup := wantapplyPosting{JobLocation: []wantapplyPlace{
		{Address: wantapplyAddress{AddressCountry: "Cyprus"}},
		{Address: wantapplyAddress{AddressCountry: "Cyprus"}},
	}}
	if got, want := dup.location(), "Cyprus"; got != want {
		t.Errorf("location() = %q, want %q", got, want)
	}
}

// wantapplyDetailHTML builds a vacancy page carrying a JobPosting ld+json block. employmentType
// is emitted as an ARRAY (as wantapply does); jobLocationType is optional.
func wantapplyDetailHTML(title, company, description, datePosted, jobLocationType string, places ...[3]string) string {
	var jl string
	for i, p := range places {
		if i > 0 {
			jl += ","
		}
		jl += `{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"` +
			p[0] + `","addressRegion":"` + p[1] + `","addressCountry":"` + p[2] + `"}}`
	}
	locType := ""
	if jobLocationType != "" {
		locType = `"jobLocationType":"` + jobLocationType + `",`
	}
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org/","@type":"JobPosting",` +
		`"title":"` + title + `",` +
		`"description":"` + description + `",` +
		`"datePosted":"` + datePosted + `",` +
		`"employmentType":["FULL_TIME"],` +
		locType +
		`"hiringOrganization":{"@type":"Organization","name":"` + company + `","sameAs":"https://x.example"},` +
		`"jobLocation":[` + jl + `]}` +
		`</script></head><body></body></html>`
}

func wantapplySitemapXML(locs ...string) string {
	s := `<?xml version="1.0" encoding="UTF-8"?><urlset>`
	for _, l := range locs {
		s += `<url><loc>` + l + `</loc></url>`
	}
	return s + `</urlset>`
}

func TestWantapplyFetchSitemapThenDetailAndMaps(t *testing.T) {
	jobURL := "https://wantapply.cy/android-team-lead-at-tradingview-3"
	detail := wantapplyDetailHTML(
		"Android Team Lead", "TradingView",
		"&lt;p&gt;Lead the &lt;b&gt;Android&lt;/b&gt; team.&lt;/p&gt;&lt;script&gt;x&lt;/script&gt;",
		"2026-07-15T18:31:30.246Z", "",
		[3]string{"", "", "Cyprus"})

	fake := (&routedHTTP{}).
		route("sitemap.xml", wantapplySitemapXML(
			jobURL,
			"https://wantapply.cy/company/tradingview", // company page, skipped
			"https://wantapply.cy/jobs/data-analyst",   // taxonomy, skipped
			"https://wantapply.cy/sign-in",             // reserved, skipped
		)).
		route("/android-team-lead-at-tradingview-3", detail)

	jobs, err := NewWantapply(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Wantapply", Provider: "wantapply",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (company/jobs/reserved URLs must be excluded)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "android-team-lead-at-tradingview-3" {
		t.Errorf("ExternalID = %q, want the slug", j.ExternalID)
	}
	if j.Title != "Android Team Lead" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "TradingView" {
		t.Errorf("Company = %q, want TradingView (from hiringOrganization.name)", j.Company)
	}
	if j.URL != jobURL {
		t.Errorf("URL = %q, want %q", j.URL, jobURL)
	}
	if j.Location != "Cyprus" {
		t.Errorf("Location = %q, want Cyprus", j.Location)
	}
	if want := "<p>Lead the <b>Android</b> team.</p>"; j.Description != want {
		t.Errorf("Description = %q, want %q (HTML unescaped + sanitized)", j.Description, want)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-07-15" {
		t.Errorf("PostedAt = %v, want 2026-07-15", j.PostedAt)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if j.WorkMode != "" {
		t.Errorf("WorkMode = %q, want empty (no TELECOMMUTE)", j.WorkMode)
	}
}

func TestWantapplyDetailPrefersFormattedDescriptionDiv(t *testing.T) {
	jobURL := "https://wantapply.cy/senior-backend-developer-at-acclaim"
	// The JobPosting `description` is flat run-together text; the visible <div class="Description">
	// carries the real formatting (headings + lists). The adapter must use the div.
	page := `<html><head><script type="application/ld+json">` +
		`{"@type":"JobPosting","title":"Senior Backend Developer","datePosted":"2026-07-15T10:00:00Z",` +
		`"employmentType":["FULL_TIME"],` +
		`"hiringOrganization":{"name":"Acclaim"},` +
		`"jobLocation":[{"@type":"Place","address":{"addressCountry":"Cyprus"}}],` +
		`"description":"REQUIREMENTS 6+ years Python RESPONSIBILITIES Build services"}` +
		`</script></head><body>` +
		`<div class="Description"><p>Intro paragraph.</p><h3><strong>REQUIREMENTS</strong></h3>` +
		`<ul><li><p>6+ years Python</p></li></ul><script>alert(1)</script></div>` +
		`</body></html>`
	fake := (&routedHTTP{}).route("/senior-backend-developer-at-acclaim", page)

	j, ok := wantapply{http: fake}.detail(context.Background(),
		wantapplyVacancy{slug: "senior-backend-developer-at-acclaim", url: jobURL})
	if !ok {
		t.Fatal("detail returned ok=false")
	}
	for _, want := range []string{"<h3>", "<strong>REQUIREMENTS</strong>", "<ul>", "<li>", "<p>6+ years Python</p>"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q; got %q", want, j.Description)
		}
	}
	if strings.Contains(j.Description, "<script>") {
		t.Errorf("Description kept <script>; got %q", j.Description)
	}
}

func TestWantapplyDetailFallsBackToJSONLDDescription(t *testing.T) {
	jobURL := "https://wantapply.cy/role-no-div"
	// No <div class="Description"> → fall back to the (sanitized) JSON-LD description.
	detail := wantapplyDetailHTML("Some Role", "Acme",
		"&lt;p&gt;Plain body.&lt;/p&gt;", "2026-07-15T10:00:00Z", "",
		[3]string{"", "", "Cyprus"})
	fake := (&routedHTTP{}).route("/role-no-div", detail)

	j, ok := wantapply{http: fake}.detail(context.Background(),
		wantapplyVacancy{slug: "role-no-div", url: jobURL})
	if !ok {
		t.Fatal("detail returned ok=false")
	}
	if j.Description != "<p>Plain body.</p>" {
		t.Errorf("Description = %q, want the JSON-LD fallback", j.Description)
	}
}

func TestWantapplyDetailRemoteWorkMode(t *testing.T) {
	jobURL := "https://wantapply.cy/head-of-qa-remote-at-cloudlinux"
	detail := wantapplyDetailHTML("Head of QA", "CloudLinux", "d", "2026-07-15T10:00:00.000Z",
		"TELECOMMUTE")
	fake := (&routedHTTP{}).route("/head-of-qa-remote-at-cloudlinux", detail)

	j, ok := wantapply{http: fake}.detail(context.Background(),
		wantapplyVacancy{slug: "head-of-qa-remote-at-cloudlinux", url: jobURL})
	if !ok {
		t.Fatal("detail returned ok=false")
	}
	if j.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (jobLocationType TELECOMMUTE)", j.WorkMode)
	}
	if !j.Remote {
		t.Error("Remote = false, want true")
	}
}

func TestWantapplyDetailDropsPageWithoutJobPosting(t *testing.T) {
	jobURL := "https://wantapply.cy/closed-role-at-acme"
	// A closed/empty page: valid HTML, no JobPosting ld+json.
	fake := (&routedHTTP{}).route("/closed-role-at-acme", `<html><body><h1>Not found</h1></body></html>`)
	if _, ok := (wantapply{http: fake}).detail(context.Background(),
		wantapplyVacancy{slug: "closed-role-at-acme", url: jobURL}); ok {
		t.Error("detail returned ok=true, want false (no JobPosting on the page)")
	}
}

func TestWantapplyDetailDropsPageWithoutCompany(t *testing.T) {
	jobURL := "https://wantapply.cy/role-no-company"
	detail := wantapplyDetailHTML("Some Role", "", "d", "2026-07-15T10:00:00.000Z", "",
		[3]string{"", "", "Cyprus"})
	fake := (&routedHTTP{}).route("/role-no-company", detail)
	if _, ok := (wantapply{http: fake}).detail(context.Background(),
		wantapplyVacancy{slug: "role-no-company", url: jobURL}); ok {
		t.Error("detail returned ok=true, want false (aggregator needs an employer name)")
	}
}

func TestWantapplyFetchNewSeenSkipsDetail(t *testing.T) {
	seenURL := "https://wantapply.cy/seen-role-at-acme"
	newURL := "https://wantapply.cy/new-role-at-acme"
	newDetail := wantapplyDetailHTML("New Role", "Acme", "d", "2026-07-15T10:00:00.000Z", "",
		[3]string{"", "", "Cyprus"})

	// Deliberately DO NOT route the seen slug's detail: a SeenRefresh must not fetch it, so a
	// missing route (which would error the fetch) still yields a refresh job for it.
	fake := (&routedHTTP{}).
		route("sitemap.xml", wantapplySitemapXML(seenURL, newURL)).
		route("/new-role-at-acme", newDetail)

	seen := func(externalID string) bool { return externalID == "seen-role-at-acme" }
	jobs, err := NewWantapply(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Wantapply", Provider: "wantapply"}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (one refresh + one hydrated)", len(jobs))
	}
	var refresh, hydrated *Job
	for i := range jobs {
		if jobs[i].SeenRefresh {
			refresh = &jobs[i]
		} else {
			hydrated = &jobs[i]
		}
	}
	if refresh == nil {
		t.Fatal("no SeenRefresh job for the seen slug")
	}
	if refresh.ExternalID != "seen-role-at-acme" {
		t.Errorf("refresh.ExternalID = %q, want seen-role-at-acme", refresh.ExternalID)
	}
	if hydrated == nil || hydrated.Title != "New Role" {
		t.Errorf("hydrated job = %+v, want the New Role detail", hydrated)
	}
}

func TestWantapplyProviderBoardlessAggregatorNotSelfClosing(t *testing.T) {
	var s Source = NewWantapply(nil)
	if s.Provider() != "wantapply" {
		t.Errorf("Provider() = %q, want wantapply", s.Provider())
	}
	if _, ok := s.(boardless); !ok {
		t.Error("wantapply must be boardless (no per-tenant board id)")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("wantapply must be an aggregator (many employers)")
	}
	if _, ok := s.(selfClosing); ok {
		t.Error("wantapply must NOT be self-closing (it relies on the unseen-sweep)")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("wantapply must be a HydratingSource (detail only for unseen)")
	}
}

func TestWantapplyIsProxied(t *testing.T) {
	// The .cy edge IP-blocks the prod datacenter IP, so wantapply must egress through the proxy.
	if _, ok := proxiedProviders["wantapply"]; !ok {
		t.Error("wantapply must be in proxiedProviders (its edge blocks the prod datacenter IP)")
	}
}

func TestWantapplyRegisteredInAll(t *testing.T) {
	reg := All(NewClient())
	if _, ok := reg["wantapply"]; !ok {
		t.Error("wantapply not registered in sources.All")
	}
}
