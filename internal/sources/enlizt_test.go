package sources

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// enliztFake serves a canned HTML body per URL substring for the listing + detail fetches.
type enliztFake struct {
	routes []struct{ match, body string }
}

func (f *enliztFake) route(match, body string) *enliztFake {
	f.routes = append(f.routes, struct{ match, body string }{match, body})
	return f
}

func (f *enliztFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	for _, r := range f.routes {
		if strings.Contains(url, r.match) {
			return html.Parse(strings.NewReader(r.body))
		}
	}
	return html.Parse(strings.NewReader("<html></html>"))
}

func TestEnliztProvider(t *testing.T) {
	if got := NewEnlizt(nil).Provider(); got != "enlizt" {
		t.Errorf("Provider() = %q, want %q", got, "enlizt")
	}
}

func TestEnliztFetchListsAndMapsDetail(t *testing.T) {
	listing := `<html><body>
		<a href="/vagas/full-stack-280426">Full-Stack</a>
		<a href="/vagas/analista-090426">Analista</a>
		<a href="/sobre">Sobre</a>
	</body></html>`

	full := `<html><head><script type="application/ld+json">{
		"@context":"https://schema.org","@type":"JobPosting","title":"Desenvolvedor(a) Full-Stack",
		"description":"&lt;p&gt;Build things&lt;/p&gt;","datePosted":"2026-04-28T17:52:33.490Z",
		"employmentType":"FULL_TIME",
		"identifier":{"@type":"PropertyValue","name":"Tributo Devido","value":"07f9c460-432b-11f1-aa90-89630f1e6003"},
		"hiringOrganization":{"@type":"Organization","name":"Tributo Devido"},
		"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Florianópolis","addressRegion":"SC","addressCountry":"BR"}}
	}</script></head><body></body></html>`

	analista := `<html><head><script type="application/ld+json">{
		"@context":"https://schema.org","@type":"JobPosting","title":"Analista Tributário",
		"description":"<p>Tax</p>","datePosted":"2026-04-09T00:00:00.000Z",
		"identifier":{"@type":"PropertyValue","value":"11111111-1111-1111-1111-111111111111"},
		"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Remote","addressRegion":"","addressCountry":"BR"}}
	}</script></head></html>`

	fake := (&enliztFake{}).
		route("/vagas/full-stack-280426", full).
		route("/vagas/analista-090426", analista).
		route("tributodevido.enlizt.me/", listing)

	jobs, err := NewEnlizt(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Tributo Devido", Provider: "enlizt", Board: "tributodevido",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (non-vaga link ignored)", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	fs, ok := byID["07f9c460-432b-11f1-aa90-89630f1e6003"]
	if !ok {
		t.Fatalf("full-stack job missing; got ids %v", byID)
	}
	if fs.Title != "Desenvolvedor(a) Full-Stack" {
		t.Errorf("Title = %q", fs.Title)
	}
	if fs.Company != "Tributo Devido" {
		t.Errorf("Company = %q", fs.Company)
	}
	if !strings.Contains(fs.URL, "tributodevido.enlizt.me/vagas/full-stack-280426") {
		t.Errorf("URL = %q", fs.URL)
	}
	if fs.Location != "Florianópolis, SC, BR" {
		t.Errorf("Location = %q, want %q", fs.Location, "Florianópolis, SC, BR")
	}
	if !strings.Contains(fs.Description, "Build things") || strings.Contains(fs.Description, "&lt;") {
		t.Errorf("Description not unescaped+sanitized: %q", fs.Description)
	}
	if fs.PostedAt == nil || fs.PostedAt.Year() != 2026 {
		t.Errorf("PostedAt = %v", fs.PostedAt)
	}

	// The Remote-location posting flags remote via the shared heuristic.
	an := byID["11111111-1111-1111-1111-111111111111"]
	if !an.Remote {
		t.Errorf("analista Remote = false, want true (location 'Remote')")
	}
}

func TestEnliztDropsPagesWithoutJobPostingAndSubActionLinks(t *testing.T) {
	// The listing carries a canonical posting, an /apply sub-action of it, and a page with
	// no JobPosting ld+json. Only the canonical posting yields a Job: the sub-action link is
	// excluded by the end-anchored pattern, and the bodyless page is dropped (ok=false).
	listing := `<html><body>
		<a href="/vagas/real-role-42">Real</a>
		<a href="/vagas/real-role-42/candidatar">Apply</a>
		<a href="/vagas/empty-99">Empty</a>
	</body></html>`
	real := `<html><head><script type="application/ld+json">{
		"@type":"JobPosting","title":"Real Role","description":"<p>x</p>","datePosted":"2026-05-01T00:00:00Z",
		"identifier":{"value":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"},
		"jobLocation":{"address":{"addressLocality":"SP","addressCountry":"BR"}}
	}</script></head></html>`
	empty := `<html><body><p>No structured data here</p></body></html>`

	fake := (&enliztFake{}).
		route("/vagas/real-role-42/candidatar", real). // would double-count if the pattern matched sub-actions
		route("/vagas/real-role-42", real).
		route("/vagas/empty-99", empty).
		route("acme.enlizt.me/", listing)

	jobs, err := NewEnlizt(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Provider: "enlizt", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 (sub-action excluded, bodyless page dropped)", len(jobs))
	}
	if jobs[0].ExternalID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Errorf("ExternalID = %q", jobs[0].ExternalID)
	}
	if !strings.HasSuffix(jobs[0].URL, "/vagas/real-role-42") {
		t.Errorf("URL = %q, want canonical posting (not the sub-action)", jobs[0].URL)
	}
}
