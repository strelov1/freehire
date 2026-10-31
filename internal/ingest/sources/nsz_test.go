package sources

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	xhtml "golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// nszFixture reads what nsz.gov.rs served on 2026-10-29: search pages trimmed to a few cards
// (page 2 repeating one card of page 1, page 3 past the end), and vacancy pages cut to the
// vacancy block, with the note's contact person, phone and e-mail, and any phone or e-mail in
// the body, replaced by placeholders.
func nszFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/nsz/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// nszFake serves pages by URL. A vacancy id it holds no page for redirects home, which is how
// the site answers for a vacancy it no longer shows.
type nszFake struct {
	mu       sync.Mutex
	pages    map[string][]byte
	errs     map[string]error
	home     []byte
	detCalls []string
}

func (f *nszFake) GetHTMLResolved(_ context.Context, u string) (*xhtml.Node, string, error) {
	isDetail := strings.HasPrefix(u, nszBaseURL+"/employee/jobs/preview/")
	if isDetail {
		f.mu.Lock()
		f.detCalls = append(f.detCalls, strings.TrimPrefix(u, nszBaseURL+"/employee/jobs/preview/"))
		f.mu.Unlock()
	}
	if err := f.errs[u]; err != nil {
		return nil, "", err
	}
	b, ok := f.pages[u]
	final := u
	if !ok {
		if !isDetail {
			return nil, "", fmt.Errorf("unexpected GET %s", u)
		}
		b, final = f.home, nszBaseURL+"/"
	}
	root, err := xhtml.Parse(bytes.NewReader(b))
	return root, final, err
}

func nszListPage(page int) string    { return fmt.Sprintf(nszListURL, page) }
func nszDetailPage(id string) string { return fmt.Sprintf(nszPostingURL, id) }

func newTestNSZ(t *testing.T) (nsz, *nszFake) {
	fake := &nszFake{
		pages: map[string][]byte{
			nszListPage(1): nszFixture(t, "listing_1.html"),
			nszListPage(2): nszFixture(t, "listing_2.html"),
			nszListPage(3): nszFixture(t, "listing_3.html"),
		},
		errs: map[string]error{},
		home: nszFixture(t, "home.html"),
	}
	for _, id := range []string{"104454", "104453", "104146", "104097", "104202"} {
		fake.pages[nszDetailPage(id)] = nszFixture(t, "detail_"+id+".html")
	}
	s := NewNSZ(fake).(nsz)
	s.limiter = rate.NewLimiter(rate.Inf, 1)
	return s, fake
}

func TestNSZProvider(t *testing.T) {
	s, ok := All(nil)["nsz"]
	if !ok || s.Provider() != "nsz" {
		t.Fatal("All() must register nsz")
	}
	if _, ok := s.(CoverageGated); !ok {
		t.Error("nsz must be CoverageGated: the body costs a request and the card names the employer")
	}
	if _, ok := s.(boardless); !ok {
		t.Error("nsz must be boardless: one national search names every vacancy")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("nsz must be an aggregator: the employer is read per vacancy")
	}
}

func TestNSZHydratesUnseenVacancies(t *testing.T) {
	s, _ := newTestNSZ(t)
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "nsz"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 5 {
		t.Fatalf("got %d jobs, want 5 (page 2 repeats a page-1 card): %+v", len(jobs), jobs)
	}
	got := byExternalID(jobs)

	teacher := got["104454"]
	if teacher.Title != "Vaspitač u predškolskoj ustanovi" {
		t.Errorf("title = %q, want it in Latin script", teacher.Title)
	}
	if teacher.Company != "PU ''Dečiji ključ''" || teacher.Location != "Beograd" {
		t.Errorf("company/location = %q / %q, want the vacancy page's employer and place of work", teacher.Company, teacher.Location)
	}
	if teacher.URL != nszDetailPage("104454") {
		t.Errorf("url = %q", teacher.URL)
	}
	if len(teacher.Countries) != 1 || teacher.Countries[0] != "rs" {
		t.Errorf("countries = %v, want [rs] from the source", teacher.Countries)
	}
	want := time.Date(2026, 10, 29, 0, 0, 0, 0, nszBelgrade)
	if teacher.PostedAt == nil || !teacher.PostedAt.Equal(want) {
		t.Errorf("posted = %v, want the period's first day %v", teacher.PostedAt, want)
	}
	if teacher.EmploymentType != "" || teacher.IsTechHint {
		t.Errorf("employment/tech = %q / %v, want neither for a fixed-term teacher", teacher.EmploymentType, teacher.IsTechHint)
	}
	for _, want := range []string{"Potreban vaspitač", "<li>Vrsta rada: Određeno</li>", "<li>Broj radnika: 3</li>"} {
		if !strings.Contains(teacher.Description, want) {
			t.Errorf("description lacks %q: %q", want, teacher.Description)
		}
	}
	for _, unwanted := range []string{"REDACTED", "NAPOMENA: Svi izrazi", "Vremensko trajanje"} {
		if strings.Contains(teacher.Description, unwanted) {
			t.Errorf("description carries %q: %q", unwanted, teacher.Description)
		}
	}
	if strings.ContainsFunc(teacher.Title+teacher.Company+teacher.Location+teacher.Description, func(r rune) bool {
		return r >= 'А' && r <= 'я' || strings.ContainsRune("ЂЈЉЊЋЏђјљњћџ", r)
	}) {
		t.Errorf("a field kept Cyrillic: %+v", teacher)
	}

	if dev := got["104146"]; !dev.IsTechHint {
		t.Errorf("a vacancy filed under Informatika i programiranje must be marked technical: %+v", dev)
	}
	if latin := got["104453"]; latin.Title != "Viši radiološki tehničar" || latin.Company != "Dom zdravlja Niš" {
		t.Errorf("a vacancy written in Latin = %q / %q, want it unchanged", latin.Title, latin.Company)
	}
	if c := got["104097"]; c.EmploymentType != "contract" {
		t.Errorf("Ugovor o delu: employment = %q, want contract", c.EmploymentType)
	}
	if many := got["104202"]; !strings.HasPrefix(many.Location, "Beograd; Kragujevac; Kruševac; ") {
		t.Errorf("a vacancy in many places: location = %q", many.Location)
	}
}

func TestNSZRefreshesSeenAndSparesGatedVacancies(t *testing.T) {
	s, fake := newTestNSZ(t)
	seen := func(id string) bool { return id == "104454" }
	gate := DetailGate{Rejected: func(j Job) bool { return j.ExternalID == "104097" }}
	jobs, err := s.FetchNewGated(context.Background(), CompanyEntry{Provider: "nsz"}, seen, gate)
	if err != nil {
		t.Fatalf("FetchNewGated: %v", err)
	}
	got := byExternalID(jobs)
	if j := got["104454"]; !j.SeenRefresh || j.Description != "" || j.Title != "Vaspitač u predškolskoj ustanovi" {
		t.Errorf("a stored vacancy must be refreshed by identity, in Latin: %+v", j)
	}
	if got["104097"].Description != "" {
		t.Error("a rejected vacancy must be yielded body-less")
	}
	for _, id := range fake.detCalls {
		if id == "104454" || id == "104097" {
			t.Errorf("vacancy page bought for %s, which needed none", id)
		}
	}
}

func TestNSZCoveredEmployerSkipsDetail(t *testing.T) {
	s, fake := newTestNSZ(t)
	var asked []string
	gate := DetailGate{Covered: func(companies []string) map[string]bool {
		asked = companies
		return map[string]bool{"Dom zdravlja Niš": true}
	}}
	jobs, err := s.FetchNewGated(context.Background(), CompanyEntry{Provider: "nsz"}, func(string) bool { return false }, gate)
	if err != nil {
		t.Fatalf("FetchNewGated: %v", err)
	}
	if len(asked) != 5 {
		t.Errorf("Covered asked about %v, want every listed employer", asked)
	}
	if j := byExternalID(jobs)["104453"]; j.Description != "" || j.Company != "Dom zdravlja Niš" || j.Location != "Niš" {
		t.Errorf("covered vacancy = %+v, want body-less with the card's employer and town", j)
	}
	for _, id := range fake.detCalls {
		if id == "104453" {
			t.Error("vacancy page bought for a covered employer's vacancy")
		}
	}
}

func TestNSZGoneAndFailedDetails(t *testing.T) {
	s, fake := newTestNSZ(t)
	delete(fake.pages, nszDetailPage("104454"))
	fake.errs[nszDetailPage("104453")] = errors.New("unexpected status 503")
	fake.pages[nszDetailPage("104146")] = fake.home
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "nsz"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	got := byExternalID(jobs)
	if _, ok := got["104454"]; ok {
		t.Error("a vacancy that redirects home is gone and must not be yielded")
	}
	if _, ok := got["104453"]; ok {
		t.Error("a vacancy whose page failed must be deferred, not yielded")
	}
	if _, ok := got["104146"]; ok {
		t.Error("a page carrying no vacancy must be deferred, not yielded")
	}
	if len(got) != 2 {
		t.Errorf("got %d jobs, want the 2 readable ones", len(got))
	}
}

func TestNSZNoDetailAnsweredFailsTheCrawl(t *testing.T) {
	s, fake := newTestNSZ(t)
	for _, id := range []string{"104454", "104453", "104146", "104097", "104202"} {
		fake.errs[nszDetailPage(id)] = errors.New("unexpected status 403")
	}
	if _, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "nsz"}, func(string) bool { return false }); err == nil {
		t.Fatal("want an error: a crawl that read no vacancy page is broken, not quiet")
	}
}

func TestNSZListingFailures(t *testing.T) {
	fetch := func(s nsz) error {
		_, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "nsz"}, func(string) bool { return false })
		return err
	}
	t.Run("a later page failing fails the crawl", func(t *testing.T) {
		s, fake := newTestNSZ(t)
		fake.errs[nszListPage(2)] = errors.New("unexpected status 500")
		if fetch(s) == nil {
			t.Fatal("want an error: a partial listing would let the sweep close what it did not reach")
		}
	})
	t.Run("an empty page short of the pager's last fails the crawl", func(t *testing.T) {
		s, fake := newTestNSZ(t)
		fake.pages[nszListPage(2)] = nszFixture(t, "listing_3.html")
		if fetch(s) == nil {
			t.Fatal("want an error: page 2 of 2 coming back empty is a truncated listing")
		}
	})
	t.Run("a page that is not the search fails the crawl", func(t *testing.T) {
		s, fake := newTestNSZ(t)
		fake.pages[nszListPage(3)] = nszFixture(t, "home.html")
		if fetch(s) == nil {
			t.Fatal("want an error: a page without the search form proves no end")
		}
	})
	t.Run("an empty catalogue fails the crawl", func(t *testing.T) {
		s, fake := newTestNSZ(t)
		fake.pages[nszListPage(1)] = nszFixture(t, "listing_3.html")
		if fetch(s) == nil {
			t.Fatal("want an error: a national exchange with no vacancy is a failed measurement")
		}
	})
	t.Run("a listing that never ends fails the crawl", func(t *testing.T) {
		s, fake := newTestNSZ(t)
		for p := 3; p <= nszMaxPages; p++ {
			fake.pages[nszListPage(p)] = nszFixture(t, "listing_2.html")
		}
		if fetch(s) == nil {
			t.Fatal("want an error: reaching the page ceiling proves no end")
		}
	})
}

func TestNSZHydrationBound(t *testing.T) {
	s, fake := newTestNSZ(t)
	s.maxNewPerRun = 1
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Provider: "nsz"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || len(fake.detCalls) != 1 || jobs[0].ExternalID != "104454" {
		t.Errorf("got %d jobs from %v, want only the newest hydrated", len(jobs), fake.detCalls)
	}
}

func TestNSZSplitEmployer(t *testing.T) {
	for in, want := range map[string][2]string{
		"Завод за геријатрију и палијативно збрињавање  Београд-Савски Венац": {"Завод за геријатрију и палијативно збрињавање", "Београд-Савски Венац"},
		"JELENA BABIĆ PR KESTEN 2 BEOGRAD  Београд-Раковица":                  {"JELENA BABIĆ PR KESTEN 2 BEOGRAD", "Београд-Раковица"},
		"“MINACORD MEDIA”  Београд": {"“MINACORD MEDIA”", "Београд"},
		"Dom zdravlja Niš":          {"Dom zdravlja Niš", ""},
	} {
		employer, town := nszSplitEmployer(in)
		if employer != want[0] || town != want[1] {
			t.Errorf("nszSplitEmployer(%q) = %q, %q; want %q, %q", in, employer, town, want[0], want[1])
		}
	}
}

func TestNSZLatinizeTextKeepsAttributes(t *testing.T) {
	root, err := xhtml.Parse(strings.NewReader(`<p>Пријаве на <a href="https://example.rs/пријава">сајту</a></p>`))
	if err != nil {
		t.Fatal(err)
	}
	nszLatinizeText(root)
	var b strings.Builder
	if err := xhtml.Render(&b, root); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); !strings.Contains(got, `<p>Prijave na <a href="https://example.rs/пријава">sajtu</a></p>`) {
		t.Errorf("text must be transliterated and a link's address kept as written: %q", got)
	}
}
