package sources

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// epreselecFixture reads a page captured from tierraastur.epreselec.com (2026-10-27: eight open
// offers) through the Scrapling tier; the ASP.NET view state is blanked.
func epreselecFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/epreselec/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

const epreselecTierraAstur = "tierraastur.epreselec.com/Ofertas/Ofertas.aspx"

// epreselecTierraAsturHTTP routes the two captured offers by id and the list for every other URL
// of the site, which is what the platform answers for an id_oferta it holds no detail page for.
func epreselecTierraAsturHTTP(t *testing.T) *routedHTTP {
	r := &routedHTTP{}
	for _, id := range []string{"3374595", "3372603"} {
		r.route(epreselecTierraAstur+"?id_oferta="+id, epreselecFixture(t, "job_tierraastur_"+id+".html"))
	}
	return r.route(epreselecTierraAstur, epreselecFixture(t, "list_tierraastur.html"))
}

var epreselecTierraAsturEntry = CompanyEntry{Company: "Tierra Astur", Provider: "epreselec", Board: "tierraastur"}

func testEPreselec(c HTMLGetter, budget int) epreselec {
	return epreselec{http: c, maxNewPerRun: budget}
}

func TestEPreselecProvider(t *testing.T) {
	s, ok := All(nil)["epreselec"]
	if !ok || s.Provider() != "epreselec" {
		t.Fatal("All() must register epreselec")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("epreselec must be a HydratingSource: a body costs a page carrying the whole list")
	}
	if !FullBoardListingProviders(All(nil))["epreselec"] {
		t.Error("epreselec's list is one page checked against its stated count: it must be fullBoardListing")
	}
}

func TestEPreselecListRefreshesSeenOffers(t *testing.T) {
	fake := epreselecTierraAsturHTTP(t)
	jobs, err := testEPreselec(fake, 10).FetchNew(context.Background(), epreselecTierraAsturEntry,
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 8 || fake.calls != 1 {
		t.Fatalf("jobs/requests = %d/%d, want the list's 8 offers from one request", len(jobs), fake.calls)
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.Title == "" || j.URL != "https://"+epreselecTierraAstur+"?id_oferta="+j.ExternalID {
			t.Errorf("job = %+v, want an identity-only SeenRefresh", j)
		}
	}
}

func TestEPreselecMapsAnOffer(t *testing.T) {
	fake := epreselecTierraAsturHTTP(t)
	jobs, err := testEPreselec(fake, 10).FetchNew(context.Background(), epreselecTierraAsturEntry,
		func(id string) bool { return id != "3374595" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	var g Job
	for _, j := range jobs {
		if j.ExternalID == "3374595" {
			g = j
		}
	}
	if g.SeenRefresh || g.Unreadable || g.Title != "PARRILLERO/A" {
		t.Fatalf("job = %+v, want the offer hydrated", g)
	}
	if g.Location != "OVIEDO - GIJON - AVILES, Asturias, España" || len(g.Countries) != 1 || g.Countries[0] != "es" {
		t.Errorf("Location/Countries = %q/%v", g.Location, g.Countries)
	}
	for _, want := range []string{"<h3>Funciones</h3>", "Asado de todo tipo de carnes", "<h3>Requisitos</h3>", "<h3>Se ofrece</h3>"} {
		if !strings.Contains(g.Description, want) {
			t.Errorf("Description lacks %q: %.300q", want, g.Description)
		}
	}
	if strings.Contains(g.Description, "Inscribirme") || strings.Contains(g.Description, "Denunciar") {
		t.Errorf("Description carries the page's buttons: %.300q", g.Description)
	}
	if g.PostedAt == nil {
		t.Error("PostedAt = nil, want the list's date")
	}
}

func TestEPreselecDefersPastTheBudget(t *testing.T) {
	fake := epreselecTierraAsturHTTP(t)
	jobs, err := testEPreselec(fake, 1).FetchNew(context.Background(), epreselecTierraAsturEntry,
		func(id string) bool { return id != "3374595" && id != "3372603" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	// Six seen offers refreshed, one new offer hydrated, the other deferred (not emitted).
	if len(jobs) != 7 || fake.calls != 2 {
		t.Fatalf("jobs/requests = %d/%d, want 7 jobs from the list and one body", len(jobs), fake.calls)
	}
}

func TestEPreselecRefusesAPageWithoutTheList(t *testing.T) {
	fake := (&routedHTTP{}).route("aecc.epreselec.com", `<html><head><title>Page not found</title></head>
<body><form action="../404.aspx"><h1>404 - Page not found</h1></form></body></html>`)
	if _, err := testEPreselec(fake, 10).Fetch(context.Background(), CompanyEntry{Board: "aecc"}); err == nil {
		t.Fatal("Fetch: want an error for the platform's 404 page, which must not read as an empty site")
	}
}

func TestEPreselecRefusesAShortList(t *testing.T) {
	list := strings.Replace(epreselecFixture(t, "list_tierraastur.html"), "Total ofertas: 8", "Total ofertas: 9", 1)
	if _, err := testEPreselec((&routedHTTP{}).route("tierraastur", list), 10).Fetch(context.Background(), epreselecTierraAsturEntry); err == nil {
		t.Fatal("Fetch: want an error when the list is shorter than the count the page states")
	}
}

// epreselecRefusingGetter answers 405 to the first `refusals` requests, then delegates.
type epreselecRefusingGetter struct {
	*routedHTTP
	refusals int
}

func (g *epreselecRefusingGetter) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	if g.refusals > 0 {
		g.refusals--
		return nil, &StatusError{Method: http.MethodGet, Code: http.StatusMethodNotAllowed, URL: url}
	}
	return g.routedHTTP.GetHTML(ctx, url)
}

func TestEPreselecResendsARefusedRequest(t *testing.T) {
	g := &epreselecRefusingGetter{routedHTTP: epreselecTierraAsturHTTP(t), refusals: 2}
	s := epreselec{http: g, maxNewPerRun: 10, retryWait: time.Millisecond}
	jobs, err := s.FetchNew(context.Background(), epreselecTierraAsturEntry, func(string) bool { return true })
	if err != nil || len(jobs) != 8 {
		t.Fatalf("FetchNew = %d jobs, %v; want the list after two refusals", len(jobs), err)
	}
}

func TestEPreselecDate(t *testing.T) {
	if d := epreselecDate("27 de octubre, 2026"); d == nil || d.Format("2006-01-02") != "2026-10-27" {
		t.Errorf("epreselecDate = %v", d)
	}
	if d := epreselecDate("ayer"); d != nil {
		t.Errorf("epreselecDate(ayer) = %v, want nil", d)
	}
}
