package sources

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// netempregosFake serves the real fixtures (fetched 2026-10-27, still in the site's
// ISO-8859-1/Windows-1252 bytes): the sitemap trimmed to six postings, the RSS trimmed to four
// items, three posting pages trimmed to their JobPosting and the "no longer active" page. Any
// other posting answers 404.
type netempregosFake struct {
	rssErr  error
	pageErr error

	mu    sync.Mutex
	pages []string
}

func (f *netempregosFake) GetText(_ context.Context, u string) (string, error) {
	file := ""
	switch u {
	case netempregosSitemap:
		file = "sitemap.txt"
	case netempregosRSS:
		if f.rssErr != nil {
			return "", f.rssErr
		}
		file = "rss.xml"
	default:
		f.mu.Lock()
		f.pages = append(f.pages, u)
		f.mu.Unlock()
		if f.pageErr != nil {
			return "", f.pageErr
		}
		pages := map[string]string{
			"/16179591/": "posting.html",
			"/16179323/": "abroad.html",
			"/16178989/": "remote.html",
			"/16150000/": "gone.html",
		}
		for id, name := range pages {
			if strings.Contains(u, id) {
				file = name
			}
		}
		if file == "" {
			return "", &StatusError{Method: "GET", Code: 404, URL: u}
		}
	}
	raw, err := os.ReadFile("testdata/netempregos/" + file)
	return string(raw), err
}

func newTestNetEmpregos(f *netempregosFake) netempregos {
	s := NewNetEmpregos(f).(netempregos)
	s.limiter = noWait{}
	return s
}

func TestNetEmpregosHydratesNewPostingsNewestFirst(t *testing.T) {
	f := &netempregosFake{}
	gate := DetailGate{
		Covered: func(cs []string) map[string]bool {
			return map[string]bool{"Gi Group": slices.Contains(cs, "Gi Group")}
		},
		Rejected: func(Job) bool { return false },
	}
	jobs, err := newTestNetEmpregos(f).FetchNewGated(context.Background(), CompanyEntry{}, func(id string) bool { return id == "16140000" }, gate)
	if err != nil {
		t.Fatal(err)
	}
	// The anonymous posting and the covered one cost no request; the rest are read newest first.
	want := []string{
		netempregosBaseURL + "/16179591/senior-backend-java-developer/",
		netempregosBaseURL + "/16179323/marketing-digital-content-specialist-angola/",
		netempregosBaseURL + "/16178989/operador-a-de-call-center-m-f-trabalho-remoto/",
		netempregosBaseURL + "/16150000/consultor-comercial-b2b/",
	}
	got := slices.Clone(f.pages)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("pages read = %v", f.pages)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if _, ok := byID["16179992"]; ok {
		t.Error("the anonymous posting was yielded")
	}
	if _, ok := byID["16150000"]; ok {
		t.Error("the no-longer-active posting was yielded")
	}
	if c := byID["16179986"]; c.Company != "Gi Group" || c.Description != "" || c.SeenRefresh {
		t.Errorf("covered posting = %+v, want it yielded body-less", c)
	}
	if !byID["16140000"].SeenRefresh {
		t.Error("the stored posting was not refreshed")
	}
	if len(jobs) != 5 {
		t.Errorf("got %d jobs, want 5", len(jobs))
	}

	j := byID["16179591"]
	if j.Title != "Senior Backend Java Developer" || j.Company != "IRIUM" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.Location != "Lisboa" || !slices.Equal(j.FallbackCountries, []string{"pt"}) || !j.IsTechHint || j.EmploymentType != "full_time" {
		t.Errorf("location %q fallback %v tech %v employment %q", j.Location, j.FallbackCountries, j.IsTechHint, j.EmploymentType)
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Format(time.RFC3339) != "2026-10-27T17:30:42Z" {
		t.Errorf("posted %v, want the RSS pubDate", j.PostedAt)
	}
	if !strings.Contains(j.Description, "Na IRIUM queremos") || strings.Contains(j.Description, "net-empregos.com/16179591") {
		t.Errorf("description = %.200q", j.Description)
	}
	if strings.ContainsRune(j.Description, '�') || strings.ContainsRune(j.Title+j.Description, 'Ã') {
		t.Errorf("description was not decoded from Windows-1252: %.200q", j.Description)
	}

	a := byID["16179323"]
	if a.Location != "Angola" || a.FallbackCountries != nil || a.Company != "Be.Great Consulting/Be.Great Pharma" {
		t.Errorf("abroad posting: location %q fallback %v company %q", a.Location, a.FallbackCountries, a.Company)
	}
	if a.PostedAt != nil {
		t.Errorf("a sitemap-only posting got a date: %v", a.PostedAt)
	}

	r := byID["16178989"]
	if r.WorkMode != "remote" || !r.Remote || r.Location != "Portugal" || r.Title != "Operador(a) de Call Center (M/F) – Trabalho Remoto" {
		t.Errorf("remote posting: work mode %q remote %v location %q title %q", r.WorkMode, r.Remote, r.Location, r.Title)
	}
}

// The RSS only feeds the gate: when it fails, the sitemap alone is the listing.
func TestNetEmpregosRSSFailureKeepsTheCrawl(t *testing.T) {
	f := &netempregosFake{rssErr: errors.New("connection reset")}
	jobs, err := newTestNetEmpregos(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(f.pages) != 6 || len(jobs) != 3 {
		t.Errorf("read %d pages and kept %d jobs, want 6 and 3", len(f.pages), len(jobs))
	}
}

// A run that reached no posting page at all is broken, not quiet.
func TestNetEmpregosFailsWhenNoPageAnswers(t *testing.T) {
	f := &netempregosFake{pageErr: errors.New("connection reset")}
	if _, err := newTestNetEmpregos(f).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false }); err == nil {
		t.Fatal("want an error when every posting page failed")
	}
}

// Near the run's deadline hydration stops and the run returns what it has, so the pipeline still
// writes it.
func TestNetEmpregosStopsHydratingNearTheDeadline(t *testing.T) {
	f := &netempregosFake{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	jobs, err := newTestNetEmpregos(f).FetchNew(ctx, CompanyEntry{}, func(id string) bool { return id == "16140000" })
	if err != nil {
		t.Fatal(err)
	}
	if len(f.pages) != 0 || len(jobs) != 1 || !jobs[0].SeenRefresh {
		t.Errorf("read %d pages, jobs %+v; want only the refresh", len(f.pages), jobs)
	}
}

func TestNetEmpregosPlace(t *testing.T) {
	for _, c := range []struct {
		zone, loc string
		pt        bool
	}{
		{"Porto", "Porto", true},
		{"( Todas as Zonas )", "Portugal", true},
		{"Estrangeiro - Angola", "Angola", false},
		{"Outros Locais - Estrangeiro", "", false},
	} {
		loc, fb := netempregosPlace(c.zone)
		if loc != c.loc || (fb != nil) != c.pt {
			t.Errorf("netempregosPlace(%q) = %q, %v", c.zone, loc, fb)
		}
	}
}
