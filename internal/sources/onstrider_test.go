package sources

import (
	"context"
	"testing"
)

// onstriderDetailHTML builds an onstrider vacancy page carrying a schema.org JobPosting ld+json
// block in the site's real shape: identifier is a PropertyValue object (value is the UUID),
// applicantLocationRequirements and employmentType are arrays, jobLocationType is the remote
// signal, and the description is already HTML (not double-escaped).
func onstriderDetailHTML(id, title, description, datePosted, jobLocationType string, countries ...string) string {
	var alr string
	for i, c := range countries {
		if i > 0 {
			alr += ","
		}
		alr += `{"@type":"Country","name":"` + c + `"}`
	}
	return `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org/","@type":"JobPosting",` +
		`"title":"` + title + `",` +
		`"description":"` + description + `",` +
		`"identifier":{"@type":"PropertyValue","name":"Strider","value":"` + id + `"},` +
		`"datePosted":"` + datePosted + `",` +
		`"employmentType":["PART_TIME","CONTRACTOR"],` +
		`"jobLocationType":"` + jobLocationType + `",` +
		`"applicantLocationRequirements":[` + alr + `]}` +
		`</script></head><body></body></html>`
}

func onstriderSitemapXML(locs ...string) string {
	s := `<?xml version="1.0" encoding="UTF-8"?><urlset>`
	for _, l := range locs {
		s += `<url><loc>` + l + `</loc></url>`
	}
	return s + `</urlset>`
}

func TestOnstriderDetailMapsJobPosting(t *testing.T) {
	jobURL := "https://www.onstrider.com/jobs/full-stack-engineer-975f80e4"
	detail := onstriderDetailHTML(
		"975f80e4-ee1e-4652-b18c-d2defecf83aa",
		"Full-stack Engineer",
		"<ul><li>Build <b>things</b></li></ul>",
		"2024-10-01",
		"TELECOMMUTE",
		"BR", "MX", "CO", "AR")
	fake := (&routedHTTP{}).route("/jobs/full-stack-engineer-975f80e4", detail)

	j, ok := onstrider{http: fake}.detail(context.Background(),
		CompanyEntry{Company: "Strider", Provider: "onstrider"}, jobURL)
	if !ok {
		t.Fatal("detail returned ok=false for an open vacancy")
	}
	if j.ExternalID != "975f80e4-ee1e-4652-b18c-d2defecf83aa" {
		t.Errorf("ExternalID = %q, want the identifier.value UUID", j.ExternalID)
	}
	if j.URL != jobURL {
		t.Errorf("URL = %q, want %q", j.URL, jobURL)
	}
	if j.Title != "Full-stack Engineer" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Strider" {
		t.Errorf("Company = %q, want Strider (real employer is hidden)", j.Company)
	}
	// The description is already HTML (not double-escaped); sanitizeHTML keeps safe tags.
	if want := "<ul><li>Build <b>things</b></li></ul>"; j.Description != want {
		t.Errorf("Description = %q, want %q (HTML sanitized)", j.Description, want)
	}
	// ISO codes expand to English country names so the location parser resolves them as LATAM
	// countries, not the US subdivisions the bare codes CO/AR collide with.
	if j.Location != "Brazil, Mexico, Colombia, Argentina" {
		t.Errorf("Location = %q, want expanded country names", j.Location)
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote/WorkMode = %v/%q, want true/remote (jobLocationType TELECOMMUTE)", j.Remote, j.WorkMode)
	}
	if j.EmploymentType != "part_time" {
		t.Errorf("EmploymentType = %q, want part_time (first of the employmentType array)", j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2024-10-01" {
		t.Errorf("PostedAt = %v, want 2024-10-01", j.PostedAt)
	}
}

func TestOnstriderDropsClosedVacancy(t *testing.T) {
	// A closed vacancy keeps its URL but drops the JobPosting markup — only an Organization
	// ld+json block remains, so detail() must return ok=false.
	closed := `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org/","@type":"Organization","name":"Strider"}` +
		`</script></head><body></body></html>`
	fake := (&routedHTTP{}).route("/jobs/closed-role-0f0f72b5", closed)

	if _, ok := (onstrider{http: fake}).detail(context.Background(),
		CompanyEntry{Company: "Strider"}, "https://www.onstrider.com/jobs/closed-role-0f0f72b5"); ok {
		t.Error("detail returned ok=true for a closed vacancy (no JobPosting block)")
	}
}

func TestOnstriderDropsPostingWithoutIdentifier(t *testing.T) {
	// A JobPosting whose identifier.value is empty has no dedup key and must be dropped.
	detail := onstriderDetailHTML("", "No ID Role", "<p>x</p>", "2026-01-02", "TELECOMMUTE", "BR")
	fake := (&routedHTTP{}).route("/jobs/no-id-role-12345678", detail)

	if _, ok := (onstrider{http: fake}).detail(context.Background(),
		CompanyEntry{Company: "Strider"}, "https://www.onstrider.com/jobs/no-id-role-12345678"); ok {
		t.Error("detail returned ok=true for a posting with an empty identifier.value")
	}
}

func TestOnstriderFetchFiltersSitemapToVacancies(t *testing.T) {
	jobURL := "https://www.onstrider.com/jobs/full-stack-engineer-975f80e4"
	fake := (&routedHTTP{}).
		route("sitemap.xml", onstriderSitemapXML(
			jobURL,
			"https://www.onstrider.com/preview-slug-0f985246-f06e-5582cab4c338/mobile-engineer-53604156", // preview dup
			"https://www.onstrider.com/pt/blog/linguagens-de-programacao",                                // blog
			"https://www.onstrider.com/hire/express-developers",                                          // marketing
			"https://www.onstrider.com/jobs",                                                             // listing root
		)).
		route("/jobs/full-stack-engineer-975f80e4",
			onstriderDetailHTML("975f80e4-ee1e-4652-b18c-d2defecf83aa",
				"Full-stack Engineer", "<p>x</p>", "2024-10-01", "TELECOMMUTE", "BR"))

	jobs, err := NewOnstrider(fake).Fetch(context.Background(),
		CompanyEntry{Company: "Strider", Provider: "onstrider"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (only the canonical /jobs/<slug>-<8hex> URL is a vacancy)", len(jobs))
	}
	if jobs[0].ExternalID != "975f80e4-ee1e-4652-b18c-d2defecf83aa" {
		t.Errorf("ExternalID = %q", jobs[0].ExternalID)
	}
}

func TestOnstriderScalarEmploymentTypeIsNotDropped(t *testing.T) {
	// schema.org permits employmentType as a bare string; the posting must still map (a fixed
	// []string decode would error and ldJobPosting would silently drop the open vacancy).
	detail := `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org/","@type":"JobPosting",` +
		`"title":"Solo Eng","description":"<p>x</p>",` +
		`"identifier":{"@type":"PropertyValue","value":"11111111-2222-3333-4444-555555555555"},` +
		`"employmentType":"FULL_TIME","jobLocationType":"TELECOMMUTE",` +
		`"applicantLocationRequirements":[{"@type":"Country","name":"BR"}]}` +
		`</script></head><body></body></html>`
	fake := (&routedHTTP{}).route("/jobs/solo-eng-11111111", detail)

	j, ok := (onstrider{http: fake}).detail(context.Background(),
		CompanyEntry{Company: "Strider"}, "https://www.onstrider.com/jobs/solo-eng-11111111")
	if !ok {
		t.Fatal("detail returned ok=false for a scalar employmentType (posting was dropped)")
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time (from the scalar employmentType)", j.EmploymentType)
	}
}

func TestOnstriderFetchFailsWhenNoVacancyMaps(t *testing.T) {
	// The sitemap lists a vacancy but its detail is closed (no JobPosting) → 0 jobs. Fetch must
	// return an error, not an empty success, so the unseen sweep does not false-close the catalogue.
	closed := `<html><head><script type="application/ld+json">` +
		`{"@context":"https://schema.org/","@type":"Organization","name":"Strider"}` +
		`</script></head><body></body></html>`
	fake := (&routedHTTP{}).
		route("sitemap.xml", onstriderSitemapXML("https://www.onstrider.com/jobs/closed-role-0f0f72b5")).
		route("/jobs/closed-role-0f0f72b5", closed)

	if _, err := NewOnstrider(fake).Fetch(context.Background(),
		CompanyEntry{Company: "Strider", Provider: "onstrider"}); err == nil {
		t.Error("Fetch returned nil error when the sitemap had vacancy URLs but none mapped")
	}
}

func TestOnstriderLocationExpandsCountryCodes(t *testing.T) {
	// Known LATAM codes expand to names; an unmapped code (that collides with a US state) is
	// kept raw as a graceful fallback rather than dropped.
	p := onstriderPosting{ApplicantLocationRequirements: []struct {
		Name string `json:"name"`
	}{{Name: "PA"}, {Name: "PE"}, {Name: "ZZ"}}}
	if got, want := p.location(), "Panama, Peru, ZZ"; got != want {
		t.Errorf("location() = %q, want %q", got, want)
	}
}

func TestOnstriderVacancyURL(t *testing.T) {
	cases := map[string]bool{
		"https://www.onstrider.com/jobs/full-stack-engineer-975f80e4":                                true,
		"https://www.onstrider.com/jobs/senior-back-end-engineer-node-express-nest-2172e283":         true,
		"https://www.onstrider.com/jobs/full-stack-engineer-975f80e4/":                               true,
		"https://www.onstrider.com/jobs":                                                             false, // listing root
		"https://www.onstrider.com/pt/blog/linguagens":                                               false, // localized blog
		"https://www.onstrider.com/hire/express-developers":                                          false, // marketing
		"https://www.onstrider.com/preview-slug-0f985246-f06e-5582cab4c338/mobile-engineer-53604156": false, // preview dup
	}
	for u, want := range cases {
		if got := onstriderVacancyURL(u); got != want {
			t.Errorf("onstriderVacancyURL(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestOnstriderProviderBoardlessAndProxied(t *testing.T) {
	var s Source = NewOnstrider(nil)
	if s.Provider() != "onstrider" {
		t.Errorf("Provider() = %q, want onstrider", s.Provider())
	}
	if _, ok := s.(boardless); !ok {
		t.Error("onstrider must be boardless (single-company, no board id)")
	}
	if _, ok := All(nil)["onstrider"]; !ok {
		t.Error("onstrider must be registered in sources.All")
	}
	if _, ok := proxiedProviders["onstrider"]; !ok {
		t.Error("onstrider must be in proxiedProviders (its Cloudflare edge may block the prod IP)")
	}
}
