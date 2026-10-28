package sources

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// servicepublicFake serves fixtures cut from the real site (fetched 2026-10-28): a listing page
// trimmed to three cards (its title rewritten to a 3-page list) for every listing page, and for
// the cards' postings: the first answers a real posting page (a Réseau Canopé developer role in
// the "Numérique" domain — another posting's page, standing in for the first card's), the second
// the "Offre dépubliée" page and the third 404.
type servicepublicFake struct {
	listErr error
	// laterErr fails every listing page after the first.
	laterErr error
	noBody   bool

	mu    sync.Mutex
	lists []string
	pages []string
}

func (f *servicepublicFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	if strings.Contains(u, "/nos-offres/") {
		f.mu.Lock()
		f.lists = append(f.lists, u)
		f.mu.Unlock()
		if f.listErr != nil {
			return nil, f.listErr
		}
		if u != servicepublicListURL && f.laterErr != nil {
			return nil, f.laterErr
		}
		raw, err := os.ReadFile("testdata/servicepublic/list_page2.html")
		if err != nil {
			return nil, err
		}
		return html.Parse(strings.NewReader(strings.Replace(string(raw), "sur 2160", "sur 3", 1)))
	}
	f.mu.Lock()
	f.pages = append(f.pages, u)
	f.mu.Unlock()
	var path string
	switch {
	case strings.Contains(u, "-reference-2026-2453798/"):
		path = "testdata/servicepublic/posting.html"
		if f.noBody {
			return html.Parse(strings.NewReader("<html><head><title>Un instant</title></head><body></body></html>"))
		}
	case strings.Contains(u, "-reference-2026-2431308/"):
		path = "testdata/servicepublic/unpublished.html"
	default:
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return html.Parse(strings.NewReader(string(raw)))
}

func newTestServicePublic(f *servicepublicFake) servicepublic {
	return servicepublic{
		http:           f,
		limiter:        noWait{},
		headPages:      servicepublicHeadPages,
		windowPages:    servicepublicWindowPages,
		maxNewPerRun:   servicepublicMaxNewPerRun,
		deadlineMargin: servicepublicDeadlineMargin,
		// The window's offset is zero at the epoch, so a short list's window starts at page 1.
		now: func() time.Time { return time.Unix(0, 0) },
	}
}

func TestServicePublicReadsCardsAndHydratesNewPostings(t *testing.T) {
	f := &servicepublicFake{}
	jobs, err := newTestServicePublic(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want one (one posting unpublished, one 404): %+v", len(jobs), jobs)
	}
	j := jobs[0]
	if j.ExternalID != "2026-2453798" || j.URL != servicepublicBaseURL+"/offre-emploi/technicienne-immobilier-au-tribunal-judiciaire-de-versailles-reference-2026-2453798/" {
		t.Errorf("identity = %q %q", j.ExternalID, j.URL)
	}
	if j.Title != "Concepteur.trice développeur.se d'applications" || j.Company != "Réseau Canopé" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if !j.IsTechHint {
		t.Error("a posting in the Numérique domain carries no tech hint")
	}
	if !strings.Contains(j.Description, "Vos missions") || strings.Contains(j.Description, "Afficher la suite") {
		t.Errorf("body = %.300q", j.Description)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "fr" || j.Location != "Yvelines (78)" {
		t.Errorf("location = %q countries = %v", j.Location, j.Countries)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-28" {
		t.Errorf("posted at = %v", j.PostedAt)
	}
	if j.WorkMode != "" {
		t.Errorf("work mode = %q from a posting whose telework line says nothing", j.WorkMode)
	}
	// A list of three pages is read whole: page 1 for the head, pages 2 and 3 as the window.
	if len(f.lists) != 3 {
		t.Errorf("listing requests = %v", f.lists)
	}
}

func TestServicePublicHeadStopsAtAKnownPage(t *testing.T) {
	f := &servicepublicFake{}
	s := newTestServicePublic(f)
	s.windowPages = 0
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(f.lists) != 1 || len(f.pages) != 0 {
		t.Errorf("listing %v and %d posting requests for a page of known postings", f.lists, len(f.pages))
	}
	for _, j := range jobs {
		if !j.SeenRefresh {
			t.Errorf("known posting yielded as %+v", j)
		}
	}
}

func TestServicePublicGateSkipsCoveredEmployers(t *testing.T) {
	f := &servicepublicFake{}
	gate := DetailGate{Covered: func(cs []string) map[string]bool {
		m := map[string]bool{}
		for _, c := range cs {
			m[c] = true
		}
		return m
	}}
	if _, err := newTestServicePublic(f).FetchNewGated(context.Background(), CompanyEntry{}, func(string) bool { return false }, gate); err != nil {
		t.Fatal(err)
	}
	if len(f.pages) != 0 {
		t.Errorf("bought %d pages of covered postings", len(f.pages))
	}
}

func TestServicePublicListingFailures(t *testing.T) {
	if _, err := newTestServicePublic(&servicepublicFake{listErr: errors.New("503")}).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Error("a failed first listing page did not fail the crawl")
	}
	if _, err := newTestServicePublic(&servicepublicFake{laterErr: errors.New("503")}).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Error("a failed head page did not fail the crawl")
	}
	// The window only refreshes what is stored: its failing ends the window, not the crawl.
	f := &servicepublicFake{laterErr: errors.New("503")}
	if _, err := newTestServicePublic(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return true }); err != nil {
		t.Errorf("a failed window page failed the crawl: %v", err)
	}
}

func TestServicePublicPageWithoutBodyIsNoAnswer(t *testing.T) {
	f := &servicepublicFake{noBody: true}
	_, _, err := newTestServicePublic(f).detail(context.Background(), servicepublicCard{id: "2026-2453798", url: servicepublicBaseURL + "/offre-emploi/x-reference-2026-2453798/"})
	if !errors.Is(err, errServicepublicNoPosting) {
		t.Errorf("err = %v, want errServicepublicNoPosting", err)
	}
}

// Three days of three-hourly runs re-list every page past the head even while the head and the
// page count move between runs — which they do, and which an offset taken modulo either breaks.
func TestServicePublicWindowComesRound(t *testing.T) {
	seen := map[int]bool{}
	start := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	maxHead, minPages := 0, 1<<30
	for run := 0; run < 24; run++ {
		head := 3 + run%7
		pages := 2160 + run*6
		maxHead, minPages = max(maxHead, head), min(minPages, pages)
		for _, p := range servicepublicWindow(head, pages, servicepublicWindowPages, start.Add(time.Duration(run*3)*time.Hour)) {
			if p <= head || p > pages {
				t.Fatalf("window page %d outside (%d, %d]", p, head, pages)
			}
			seen[p] = true
		}
	}
	for p := maxHead + 1; p <= minPages; p++ {
		if !seen[p] {
			t.Fatalf("page %d never re-listed in three days", p)
		}
	}
}

func TestServicePublicParseURL(t *testing.T) {
	for raw, want := range map[string]string{
		"https://choisirleservicepublic.gouv.fr/offre-emploi/adjoint-au-responsable-rh-reference-2026-2453804/":                                      "2026-2453804",
		"https://choisirleservicepublic.gouv.fr/offre-emploi/chargee-de-mission--signal-accueil-collectif-de-mineurs--reference-MENJ-30-2026-49681/": "menj-30-2026-49681",
		"https://choisirleservicepublic.gouv.fr/nos-offres/":                                                                                         "",
		"https://evil.example/offre-emploi/adjoint-au-responsable-rh-reference-2026-2453804/":                                                        "",
	} {
		id, _, ok := servicepublicParseURL(raw)
		if ok != (want != "") || id != want {
			t.Errorf("servicepublicParseURL(%q) = %q %v, want %q", raw, id, ok, want)
		}
	}
	for in, want := range map[string]string{"Oui": "hybrid", "Non": "onsite", "Non renseigné": ""} {
		if got := servicepublicWorkMode(in); got != want {
			t.Errorf("servicepublicWorkMode(%q) = %q, want %q", in, got, want)
		}
	}
}
