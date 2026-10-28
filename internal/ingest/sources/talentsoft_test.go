package sources

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// talentsoftRoute is one URL the fake answers: the fixture it serves (or literal HTML), the final
// URL it reports after redirects, and an error to fail with instead.
type talentsoftRoute struct {
	fixture, body, final string
	err                  error
}

type talentsoftFake struct {
	t      *testing.T
	routes map[string]talentsoftRoute
	mu     sync.Mutex
	gets   []string
}

func (f *talentsoftFake) GetHTMLResolved(_ context.Context, u string) (*html.Node, string, error) {
	f.mu.Lock()
	f.gets = append(f.gets, u)
	f.mu.Unlock()
	r, ok := f.routes[u]
	if !ok {
		return nil, "", &StatusError{Code: 404}
	}
	if r.err != nil {
		return nil, "", r.err
	}
	body := r.body
	if r.fixture != "" {
		raw, err := os.ReadFile(filepath.Join("testdata", "talentsoft", r.fixture))
		if err != nil {
			f.t.Fatalf("fixture %s: %v", r.fixture, err)
		}
		body = string(raw)
	}
	root, err := html.Parse(strings.NewReader(body))
	return root, firstNonEmpty(r.final, u), err
}

const (
	tsHost    = "adsn-career.talent-soft.com"
	tsList    = "https://" + tsHost + "/offre-de-emploi/liste-toutes-offres.aspx?all=1&mode=list&page="
	tsPosting = "https://" + tsHost + "/offre-de-emploi/emploi-lead-developpeur-java-h-f_237.aspx"
)

func newTalentsoftTest(f *talentsoftFake) talentsoft {
	return talentsoft{http: f, maxNewPerRun: 100, deadlineMargin: time.Minute}
}

func adsnFake(t *testing.T) *talentsoftFake {
	return &talentsoftFake{t: t, routes: map[string]talentsoftRoute{
		tsList + "1": {fixture: "listing_p1.html"},
		tsList + "2": {fixture: "listing_p2.html"},
		tsPosting:    {fixture: "posting_237.html"},
	}}
}

func TestTalentsoftListingWalksEveryPage(t *testing.T) {
	f := adsnFake(t)
	got, err := newTalentsoftTest(f).list(context.Background(), talentsoftPaced{s: newTalentsoftTest(f)}, tsHost)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// The site states 11 postings, 10 on page one, so the walk reads exactly two pages.
	if len(got) != 11 {
		t.Fatalf("got %d postings, want 11", len(got))
	}
	byID := map[string]talentsoftPosting{}
	for _, p := range got {
		byID[p.id] = p
	}
	java, ok := byID["237"]
	if !ok {
		t.Fatalf("posting 237 missing: %+v", got)
	}
	if java.url != tsPosting || java.title != "Lead développeur Java H/F" {
		t.Errorf("posting 237 = %+v", java)
	}
	if java.posted == nil {
		t.Errorf("posting 237 has no date; row items %q", java.rowItems)
	}
	if _, ok := byID["215"]; !ok {
		t.Errorf("page two's posting 215 missing")
	}
	if len(f.gets) != 2 {
		t.Errorf("requests = %q, want the two listing pages", f.gets)
	}
}

func TestTalentsoftFetchNewHydratesOnlyNewPostings(t *testing.T) {
	f := adsnFake(t)
	s := newTalentsoftTest(f)
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Board: tsHost, Company: "ADSN"},
		func(id string) bool { return id != "237" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 11 {
		t.Fatalf("got %d jobs, want 11", len(jobs))
	}
	var java *Job
	refresh := 0
	for i := range jobs {
		if jobs[i].SeenRefresh {
			refresh++
			continue
		}
		java = &jobs[i]
	}
	if refresh != 10 || java == nil {
		t.Fatalf("refresh=%d java=%v", refresh, java)
	}
	if java.ExternalID != "237" || java.URL != tsPosting || java.Company != "ADSN" {
		t.Errorf("identity = %+v", java)
	}
	if !strings.Contains(java.Description, "Rejoignez un portefeuille de projets") {
		t.Errorf("description lacks the mission: %.200q", java.Description)
	}
	if !strings.Contains(java.Location, "Venelles") || !strings.Contains(java.Location, "France") {
		t.Errorf("location = %q", java.Location)
	}
	if java.EmploymentType != "full_time" {
		t.Errorf("employment type = %q, want full_time from the CDI row label", java.EmploymentType)
	}
	if java.PostedAt == nil {
		t.Errorf("no PostedAt")
	}
	if len(f.gets) != 3 {
		t.Errorf("requests = %q, want two listing pages and one posting", f.gets)
	}
}

func TestTalentsoftEmptySiteIsNoPostings(t *testing.T) {
	f := &talentsoftFake{t: t, routes: map[string]talentsoftRoute{
		tsList + "1": {fixture: "listing_empty.html"},
	}}
	jobs, err := newTalentsoftTest(f).Fetch(context.Background(), CompanyEntry{Board: tsHost, Company: "ADSN"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs=%d err=%v, want an empty board", len(jobs), err)
	}
}

func TestTalentsoftListingFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		routes map[string]talentsoftRoute
	}{
		{"redirected off the listing", map[string]talentsoftRoute{
			tsList + "1": {fixture: "listing_p1.html", final: "https://" + tsHost + "/accueil.aspx"},
		}},
		{"page that is not a listing", map[string]talentsoftRoute{
			tsList + "1": {body: "<html><title>Maintenance</title><body>back soon</body></html>"},
		}},
		{"later page fails", map[string]talentsoftRoute{
			tsList + "1": {fixture: "listing_p1.html"},
			tsList + "2": {err: &StatusError{Code: 503}},
		}},
		{"later page lists nothing", map[string]talentsoftRoute{
			tsList + "1": {fixture: "listing_p1.html"},
			tsList + "2": {fixture: "listing_empty.html"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &talentsoftFake{t: t, routes: tc.routes}
			if _, err := newTalentsoftTest(f).Fetch(context.Background(), CompanyEntry{Board: tsHost}); err == nil {
				t.Fatal("want the crawl to fail")
			}
		})
	}
}

func TestTalentsoftWithdrawnPostingIsDropped(t *testing.T) {
	f := adsnFake(t)
	f.routes[tsPosting] = talentsoftRoute{fixture: "listing_empty.html", final: "https://" + tsHost + "/accueil.aspx?LCID=1036"}
	jobs, err := newTalentsoftTest(f).FetchNew(context.Background(), CompanyEntry{Board: tsHost},
		func(id string) bool { return id != "237" })
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.ExternalID == "237" {
			t.Fatalf("a posting that redirects home must be dropped, got %+v", j)
		}
	}
}

func TestTalentsoftFailedPostingIsUnreadable(t *testing.T) {
	f := adsnFake(t)
	f.routes[tsPosting] = talentsoftRoute{err: errors.New("timeout")}
	jobs, err := newTalentsoftTest(f).FetchNew(context.Background(), CompanyEntry{Board: tsHost},
		func(id string) bool { return id != "237" })
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.ExternalID == "237" && !j.Unreadable {
			t.Fatalf("want an unreadable marker, got %+v", j)
		}
	}
}

func TestTalentsoftBudgetDefersNewPostings(t *testing.T) {
	f := adsnFake(t)
	s := newTalentsoftTest(f)
	s.maxNewPerRun = 0
	jobs, err := s.Fetch(context.Background(), CompanyEntry{Board: tsHost})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 || len(f.gets) != 2 {
		t.Fatalf("jobs=%d requests=%q: with no budget only the listing is read", len(jobs), f.gets)
	}
}

func TestTalentsoftRejectsANonHostBoard(t *testing.T) {
	f := &talentsoftFake{t: t}
	for _, b := range []string{"", "https://x.talent-soft.com", "x.talent-soft.com/path", "nodot"} {
		if _, err := newTalentsoftTest(f).Fetch(context.Background(), CompanyEntry{Board: b}); err == nil {
			t.Errorf("board %q accepted", b)
		}
	}
	if len(f.gets) != 0 {
		t.Errorf("requests sent: %q", f.gets)
	}
}

func TestTalentsoftEmploymentType(t *testing.T) {
	for labels, want := range map[string]string{
		"CDI":                          "full_time",
		"CDD-Temps plein":              "full_time",
		"Stage":                        "internship",
		"Part time permanent":          "part_time",
		"Intérim":                      "contract",
		"International mobility":       "",
		"Durée indéterminée":           "full_time",
		"Emploi ouvert aux titulaires": "",
	} {
		if got := talentsoftEmploymentType([]string{labels}); got != want {
			t.Errorf("%q = %q, want %q", labels, got, want)
		}
	}
}

func TestTalentsoftDate(t *testing.T) {
	if d := talentsoftDate("27/10/2025"); d == nil || d.Format("2006-01-02") != "2025-10-27" {
		t.Errorf("27/10/2025 = %v", d)
	}
	if d := talentsoftDate("10.27.2025"); d != nil {
		t.Errorf("a month-first date must not be read: %v", d)
	}
	if d := talentsoftDate("Réf. : 2026-187048"); d != nil {
		t.Errorf("a reference is not a date: %v", d)
	}
}

func TestTalentsoftListedPlace(t *testing.T) {
	for items, want := range map[string]string{
		"Réf. : 2026-54226|28/10/2026|CDD|Joué-lès-Tours": "Joué-lès-Tours",
		"28/10/2026|Réf. : 2026-1":                        "",
		"Réf. : 2026-1|28/10/2026":                        "",
		"":                                                "",
	} {
		var p talentsoftPosting
		if items != "" {
			p.rowItems = strings.Split(items, "|")
		}
		if got := p.listedPlace(); got != want {
			t.Errorf("%q = %q, want %q", items, got, want)
		}
	}
}
