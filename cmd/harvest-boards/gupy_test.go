package main

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

func TestGupyRegisteredAsDiscoverer(t *testing.T) {
	p, ok := probers["gupy"]
	if !ok {
		t.Fatal(`probers["gupy"] missing`)
	}
	if _, isDiscoverer := p.(discoverer); !isDiscoverer {
		t.Error("gupy prober should implement the discoverer interface")
	}
}

func TestGupyProbe(t *testing.T) {
	g := gupyProber{}
	// Shapes trimmed from live responses (2026-10-19).
	getter := fakeGetter{
		sources.GupyJobsURL + "?subdomain=origoenergiainterno": `{"data":[{"id":12679978,"name":"Analista Sênior de Garantia da Receita","workplaceType":"hybrid"}]}`,
		gupyCareerPageURL + "origoenergiainterno":              `{"careerPage":{"id":134980,"companyId":48481,"name":"Órigo Energia Recrutamento Interno","publicationName":"Órigo Energia - Recrutamento Interno","subdomain":"origoenergiainterno"}}`,
		// postings but no career-page description reachable -> falls back to the subdomain
		sources.GupyJobsURL + "?subdomain=caju": `{"data":[{"id":12691526},{"id":12691527}]}`,
		// a live page with nothing published -> skip
		sources.GupyJobsURL + "?subdomain=datalawyer": `{"data":[]}`,
	}

	cases := []struct {
		page     string
		wantName string
		wantN    int
	}{
		{"origoenergiainterno", "Órigo Energia - Recrutamento Interno", 1},
		{"caju", "caju", 2},
		{"datalawyer", "", 0},
		{"missing", "", 0}, // Gupy's 404 "Career Page not Found" -> getter error -> skip, never fatal
	}
	for _, c := range cases {
		name, n, err := g.probe(context.Background(), getter, c.page)
		if err != nil {
			t.Errorf("probe(%s) err = %v, want nil", c.page, err)
		}
		if name != c.wantName || n != c.wantN {
			t.Errorf("probe(%s) = (%q, %d), want (%q, %d)", c.page, name, n, c.wantName, c.wantN)
		}
	}
}

func gupyPage(t *testing.T, pages ...string) string {
	t.Helper()
	body := `{"data":[`
	for i, p := range pages {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"id":%d,"jobUrl":"https://%s.gupy.io/job/eyJqb2JJZCI6MX0=?jobBoardSource=gupy_portal"}`, i, p)
	}
	return body + `],"pagination":{"total":100}}`
}

func gupyFeedPage(offset int) string {
	return gupyFeedURL + fmt.Sprintf("?limit=%d&offset=%d", gupyPageSize, offset)
}

func TestGupyDiscoverPaginatesDedupsAndStops(t *testing.T) {
	g := gupyProber{}
	getter := fakeGetter{
		gupyFeedPage(0):                gupyPage(t, "caju", "Stefanini", "caju"), // caju repeats within the page; case folds
		gupyFeedPage(gupyPageSize):     gupyPage(t, "stefanini", "origoenergia"), // stefanini repeats across pages
		gupyFeedPage(2 * gupyPageSize): `{"data":[],"pagination":{"total":100}}`, // empty page -> stop
	}

	got, err := g.discover(context.Background(), getter)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	want := []string{"caju", "stefanini", "origoenergia"} // distinct pages, first-seen order
	if !slices.Equal(got, want) {
		t.Errorf("discover() = %v, want %v (distinct, first-seen)", got, want)
	}
}

// A page that fails to fetch mid-sweep truncates the discovery to what was collected so far,
// rather than aborting the whole harvest.
func TestGupyDiscoverTruncatesOnPageError(t *testing.T) {
	g := gupyProber{}
	getter := fakeGetter{
		gupyFeedPage(0): gupyPage(t, "caju", "stefanini"),
		// the next offset is unmapped -> getter error -> stop with what was collected
	}

	got, err := g.discover(context.Background(), getter)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if want := []string{"caju", "stefanini"}; !slices.Equal(got, want) {
		t.Errorf("discover() = %v, want %v (truncated at the failed page)", got, want)
	}
}

// The portal feed answers nothing past offset 10,000, so the sweep never asks for it.
func TestGupyDiscoverStopsAtTheFeedsEnd(t *testing.T) {
	g := gupyProber{}
	getter := fakeGetter{}
	for offset := 0; offset < gupyMaxOffset; offset += gupyPageSize {
		getter[gupyFeedPage(offset)] = gupyPage(t, fmt.Sprintf("p%d", offset))
	}

	got, err := g.discover(context.Background(), getter)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(got) != gupyMaxOffset/gupyPageSize {
		t.Errorf("discover() found %d pages, want %d", len(got), gupyMaxOffset/gupyPageSize)
	}
}
