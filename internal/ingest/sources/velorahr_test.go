package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/location"
)

// The fixtures are captures of api.velorahr.com (2026-10-27): Wetaca's site configuration and
// its whole five-offer listing, and real offers cut from three larger listings — two of Grupo
// Batta's Mexican openings that show their pay, two of Serunion's (one remote, one in a town
// with a bilingual name), and two entries of Serunion's MASTER site, which re-lists offers its
// child sites own.
func velorahrFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/velorahr/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// velorahrFake answers a GET by the URL's path from canned bodies, a path with no body as 404,
// and records every URL it was asked for.
type velorahrFake struct {
	bodies map[string]string
	urls   []string
}

func (f *velorahrFake) GetJSON(_ context.Context, url string, v any) error {
	f.urls = append(f.urls, url)
	path, _, _ := strings.Cut(strings.TrimPrefix(url, "https://api.velorahr.com"), "?")
	body, ok := f.bodies[path]
	if !ok {
		return &StatusError{Method: http.MethodGet, Code: http.StatusNotFound, URL: url}
	}
	return json.Unmarshal([]byte(body), v)
}

func newVelorahrFake(t *testing.T, site string, listing string) *velorahrFake {
	return &velorahrFake{bodies: map[string]string{
		"/api/public/career-site/" + site:             velorahrFixture(t, "site_158.json"),
		"/api/public/career-site/" + site + "/offers": listing,
	}}
}

// newTestVelorahr is the adapter without its request pacing.
func newTestVelorahr(c JSONGetter) velorahr {
	s := NewVelorahr(c).(velorahr)
	s.limiter = rate.NewLimiter(rate.Inf, 1)
	return s
}

func velorahrFetch(t *testing.T, fake *velorahrFake, board, company string) []Job {
	t.Helper()
	jobs, err := newTestVelorahr(fake).Fetch(context.Background(), CompanyEntry{Company: company, Provider: "velorahr", Board: board})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	return jobs
}

func velorahrByID(jobs []Job) map[string]Job {
	out := map[string]Job{}
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestVelorahrProvider(t *testing.T) {
	s, ok := All(nil)["velorahr"]
	if !ok || s.Provider() != "velorahr" {
		t.Fatal("All() must register velorahr")
	}
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("velorahr must be a fullBoardListing: one request lists the whole site and a short answer proves its end")
	}
	if _, ok := s.(HydratingSource); ok {
		t.Error("velorahr must not hydrate: the listing already carries every body")
	}
}

func TestVelorahrListsTheWholeSiteInOneRequest(t *testing.T) {
	fake := newVelorahrFake(t, "158", velorahrFixture(t, "offers_158.json"))
	jobs := velorahrFetch(t, fake, "158", "Wetaca")
	if len(jobs) != 5 {
		t.Fatalf("got %d jobs, want the listing's 5", len(jobs))
	}
	want := []string{
		"https://api.velorahr.com/api/public/career-site/158",
		`https://api.velorahr.com/api/public/career-site/158/offers?filter=%7B%7D&range=%5B0%2C999%5D&sort=%5B%22id%22%2C%22DESC%22%5D`,
	}
	if !slices.Equal(fake.urls, want) {
		t.Errorf("requests = %q, want the site then its whole listing %q", fake.urls, want)
	}
}

func TestVelorahrMapsAnOffer(t *testing.T) {
	jobs := velorahrByID(velorahrFetch(t, newVelorahrFake(t, "158", velorahrFixture(t, "offers_158.json")), "158", "Wetaca"))
	j, ok := jobs["7468"]
	if !ok {
		t.Fatalf("offer 7468 missing from %d jobs", len(jobs))
	}
	if j.Title != "Gestor de ATC - Interinidad" {
		t.Errorf("Title = %q, want the clean title without the [WTCTC] reference", j.Title)
	}
	if j.URL != "https://wetaca.career.velorahr.com/wetaca-158/offers/7468" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Company != "Wetaca" {
		t.Errorf("Company = %q, want Wetaca", j.Company)
	}
	if j.Location != "Madrid, ES" {
		t.Errorf("Location = %q, want the city once and the country code", j.Location)
	}
	if !slices.Equal(j.Countries, []string{"es"}) {
		t.Errorf("Countries = %v, want [es]", j.Countries)
	}
	if j.WorkMode != "hybrid" || j.Remote {
		t.Errorf("WorkMode/Remote = %q/%v, want hybrid/false", j.WorkMode, j.Remote)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time from Jornada completa", j.EmploymentType)
	}
	if want := time.Date(2026, 6, 9, 15, 40, 59, 0, time.UTC); j.PostedAt == nil || !j.PostedAt.Equal(want) {
		t.Errorf("PostedAt = %v, want %v", j.PostedAt, want)
	}
	if strings.TrimSpace(j.Description) == "" {
		t.Error("Description is empty: the listing carries the body")
	}
	// A hidden salary is not published, whatever the from/to fields hold.
	if j.SalaryMin != nil || j.SalaryMax != nil || j.SalaryCurrency != "" {
		t.Errorf("salary = %v-%v %q, want none: the site hides it", j.SalaryMin, j.SalaryMax, j.SalaryCurrency)
	}
	if r := jobs["8337"]; r.WorkMode != "remote" || !r.Remote {
		t.Errorf("8337 WorkMode/Remote = %q/%v, want remote/true", r.WorkMode, r.Remote)
	}
	if o := jobs["14979"]; o.WorkMode != "onsite" {
		t.Errorf("14979 WorkMode = %q, want onsite", o.WorkMode)
	}
	// "MADRID" in capitals is the same city.
	if g := location.Parse(jobs["7705"].Location); !slices.Contains(g.Cities, "Madrid") {
		t.Errorf("7705 Location %q resolves to cities %v, want Madrid", jobs["7705"].Location, g.Cities)
	}
}

func TestVelorahrRequirementsJoinTheBody(t *testing.T) {
	jobs := velorahrByID(velorahrFetch(t, newVelorahrFake(t, "158", velorahrFixture(t, "offers_158.json")), "158", "Wetaca"))
	j := jobs["14979"]
	if !strings.Contains(j.Description, "Quiénes somos") || !strings.Contains(j.Description, "Quién eres") {
		t.Errorf("Description must carry the offer and its requirements: %.200q", j.Description)
	}
}

func TestVelorahrShownSalaryAndForeignCountry(t *testing.T) {
	fake := newVelorahrFake(t, "233", velorahrFixture(t, "offers_233.json"))
	jobs := velorahrByID(velorahrFetch(t, fake, "233", "Grupo Batta"))
	j, ok := jobs["15215"]
	if !ok {
		t.Fatalf("offer 15215 missing")
	}
	if j.SalaryMin == nil || *j.SalaryMin != 9000 || j.SalaryMax == nil || *j.SalaryMax != 9500 ||
		j.SalaryCurrency != "MXN" || j.SalaryPeriod != "month" {
		t.Errorf("salary = %v-%v %q/%q, want 9000-9500 MXN/month", j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
	}
	if !slices.Equal(j.Countries, []string{"mx"}) {
		t.Errorf("Countries = %v, want [mx]: the offer states Mexico", j.Countries)
	}
	if j.Location != "HERMOSILLO, Sonora, MX" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.Company != "Grupo Batta" {
		t.Errorf("Company = %q, want the catalogue's Grupo Batta", j.Company)
	}
}

func TestVelorahrBilingualTownAndHourlySchedule(t *testing.T) {
	jobs := velorahrByID(velorahrFetch(t, newVelorahrFake(t, "194", velorahrFixture(t, "offers_194.json")), "194", "Serunion"))
	p := jobs["14279"]
	if p.Location != "Pamplona, Navarra, ES" {
		t.Errorf("Location = %q, want the Spanish half of Pamplona/Iruña", p.Location)
	}
	if g := location.Parse(p.Location); !slices.Contains(g.Cities, "Pamplona") {
		t.Errorf("Location %q resolves to cities %v, want Pamplona", p.Location, g.Cities)
	}
	if p.EmploymentType != "" {
		t.Errorf("EmploymentType = %q, want none: Indiferente states no schedule", p.EmploymentType)
	}
	r := jobs["15043"]
	if r.EmploymentType != "part_time" || r.WorkMode != "remote" {
		t.Errorf("15043 EmploymentType/WorkMode = %q/%q, want part_time/remote", r.EmploymentType, r.WorkMode)
	}
	if g := location.Parse(r.Location); !slices.Contains(g.Cities, "Reus") {
		t.Errorf("Location %q resolves to cities %v, want Reus", r.Location, g.Cities)
	}
}

// A master site re-lists its child sites' offers, each naming the site that owns it; those are
// crawled under their own board, so the master yields none of them.
func TestVelorahrSkipsOffersAnotherSiteOwns(t *testing.T) {
	jobs := velorahrFetch(t, newVelorahrFake(t, "279", velorahrFixture(t, "offers_279.json")), "279", "Serunion")
	if len(jobs) != 0 {
		t.Errorf("got %d jobs, want none: every offer belongs to site 194", len(jobs))
	}
	// The same offers read on the site they name are kept.
	if jobs := velorahrFetch(t, newVelorahrFake(t, "194", velorahrFixture(t, "offers_279.json")), "194", "Serunion"); len(jobs) != 2 {
		t.Errorf("got %d jobs on the owning site, want 2", len(jobs))
	}
}

func TestVelorahrUnknownSiteIsGone(t *testing.T) {
	fake := &velorahrFake{bodies: map[string]string{"/api/public/career-site/99999/offers": `{"data":[],"success":true}`}}
	_, err := newTestVelorahr(fake).Fetch(context.Background(), CompanyEntry{Company: "X", Provider: "velorahr", Board: "99999"})
	if !errors.Is(err, ErrBoardGone) {
		t.Errorf("err = %v, want ErrBoardGone: the platform has no such career site", err)
	}
}

func TestVelorahrRejectsANonNumericBoard(t *testing.T) {
	for _, board := range []string{"", "wetaca", "0", "158/offers", "-1"} {
		fake := &velorahrFake{}
		if _, err := newTestVelorahr(fake).Fetch(context.Background(), CompanyEntry{Provider: "velorahr", Board: board}); err == nil {
			t.Errorf("board %q: want an error", board)
		}
		if len(fake.urls) != 0 {
			t.Errorf("board %q: issued %d requests, want none", board, len(fake.urls))
		}
	}
}

// A listing that fills the whole requested window may have more behind it, and a swept board
// must not read a truncated list as a complete one.
func TestVelorahrFullWindowFailsTheCrawl(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"success":true,"data":[`)
	for i := 0; i < velorahrWindow; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":%d,"clean_title":"T","description_html":"<p>d</p>","country":"ES","offer_url":"https://wetaca.career.velorahr.com/wetaca-158/offers/%d"}`, i+1, i+1)
	}
	b.WriteString(`]}`)
	_, err := newTestVelorahr(newVelorahrFake(t, "158", b.String())).Fetch(context.Background(), CompanyEntry{Company: "Wetaca", Provider: "velorahr", Board: "158"})
	if err == nil {
		t.Error("a listing filling the whole window must fail the crawl")
	}
}

// The posting link is only ever a link, but it must be the platform's own page.
func TestVelorahrDropsAnOfferLinkingOffThePlatform(t *testing.T) {
	listing := strings.Replace(velorahrFixture(t, "offers_158.json"),
		`https:\/\/wetaca.career.velorahr.com\/wetaca-158\/offers\/7468`, `https:\/\/evil.example\/offers\/7468`, 1)
	jobs := velorahrByID(velorahrFetch(t, newVelorahrFake(t, "158", listing), "158", "Wetaca"))
	if _, ok := jobs["7468"]; ok || len(jobs) != 4 {
		t.Errorf("got %d jobs (7468 kept: %v), want the other 4", len(jobs), ok)
	}
}
