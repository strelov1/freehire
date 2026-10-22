package sources

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// lestaFake serves fixture bodies by exact URL; an unmapped URL fails like a refused request.
// The detail fan-out runs on a worker pool, so the recorder is locked.
type lestaFake struct {
	json  map[string]string
	pages map[string]string
	mu    sync.Mutex
	got   []string
}

func (f *lestaFake) record(url string) {
	f.mu.Lock()
	f.got = append(f.got, url)
	f.mu.Unlock()
}

func (f *lestaFake) GetJSON(_ context.Context, url string, v any) error {
	f.record(url)
	body, ok := f.json[url]
	if !ok {
		return errors.New("lestaFake: unmapped " + url)
	}
	return json.Unmarshal([]byte(body), v)
}

func (f *lestaFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.record(url)
	body, ok := f.pages[url]
	if !ok {
		return nil, errors.New("lestaFake: unmapped " + url)
	}
	return html.Parse(strings.NewReader(body))
}

func lestaFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// lestaListing is the real listing captured 2026-10-15: 38 vacancies over offsets 0, 15 and 30.
func lestaListing(t *testing.T) map[string]string {
	return map[string]string{
		"https://join.lesta.team/filters?offset=0":  lestaFixture(t, "lesta_filters_0.json"),
		"https://join.lesta.team/filters?offset=15": lestaFixture(t, "lesta_filters_15.json"),
		"https://join.lesta.team/filters?offset=30": lestaFixture(t, "lesta_filters_30.json"),
	}
}

// Only vacancy 575's page is mapped, so every other new posting's detail fails and is dropped
// rather than stored body-less.
func TestLestaFetchFromFixtures(t *testing.T) {
	const url575 = "https://join.lesta.team/vacancy/senior-backend-engineer/575"
	f := &lestaFake{json: lestaListing(t), pages: map[string]string{url575: lestaFixture(t, "lesta_vacancy.html")}}

	jobs, err := NewLesta(f).Fetch(context.Background(), CompanyEntry{Company: "Lesta Games"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(f.got) != 3+38 {
		t.Fatalf("want 3 listing + 38 detail requests, got %d", len(f.got))
	}
	if len(jobs) != 1 {
		t.Fatalf("want only the hydrated posting, got %d", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "575" || j.URL != url575 || j.Title != "Senior Backend Engineer" || j.Company != "Lesta Games" {
		t.Fatalf("identity: %+v", j)
	}
	if j.Location != "Минск" || j.Remote {
		t.Fatalf("location: %q remote=%v", j.Location, j.Remote)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 3 {
		t.Fatalf("experience: %v", j.ExperienceYearsMin)
	}
	if strings.Join(j.Skills, ",") != "cpp,python" {
		t.Fatalf("skills: %v", j.Skills)
	}
	for _, want := range []string{"Леста Игры", "Чем предстоит заниматься", "Уверенное знание Python", "Что мы предлагаем"} {
		if !strings.Contains(j.Description, want) {
			t.Fatalf("description missing %q: %q", want, j.Description)
		}
	}
	if strings.Contains(j.Description, "Навыки") {
		t.Fatalf("the skills block duplicates the structured skills and must be left out: %q", j.Description)
	}
	if strings.Contains(j.Description, "Расскажите нам о себе") {
		t.Fatalf("the apply form leaked into the description: %q", j.Description)
	}
}

// A stored posting costs no detail request and keeps its listing identity.
func TestLestaFetchNewSkipsSeenDetail(t *testing.T) {
	f := &lestaFake{json: lestaListing(t)}

	jobs, err := NewLesta(f).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Lesta Games"}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(f.got) != 3 || len(jobs) != 38 {
		t.Fatalf("want 3 requests and 38 refreshes, got %d and %d", len(f.got), len(jobs))
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.ExternalID == "" || j.Title == "" || !strings.HasPrefix(j.URL, "https://join.lesta.team/vacancy/") {
			t.Fatalf("bad refresh: %+v", j)
		}
	}
}

// An empty page before the reported total is a truncated catalogue, not its end.
func TestLestaListingRefusesAShortCatalogue(t *testing.T) {
	listing := lestaListing(t)
	listing["https://join.lesta.team/filters?offset=30"] = `{"total": 38, "vacancies": []}`
	f := &lestaFake{json: listing}

	if _, err := NewLesta(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error for a listing that stops short of its total")
	}
}

func TestLestaExperienceMin(t *testing.T) {
	cases := []struct {
		bands []string
		want  int // -1: unknown
	}{
		{nil, -1},
		{[]string{"От 1 года до 3 лет", "От 3 до 6 лет"}, 1},
		{[]string{"От 3 до 6 лет", "Более 6 лет"}, 3},
		{[]string{"Более 6 лет"}, 6},
		{[]string{"Без опыта"}, 0},
		{[]string{"Не важно"}, -1},
	}
	for _, c := range cases {
		got := lestaExperienceMin(c.bands)
		if (got == nil) != (c.want < 0) || (got != nil && *got != c.want) {
			t.Fatalf("%v: got %v, want %d", c.bands, got, c.want)
		}
	}
}

// Listing new postings and reading none of their pages is a broken crawl, not a quiet one.
func TestLestaFailsWhenNoNewPostingIsRead(t *testing.T) {
	f := &lestaFake{json: lestaListing(t)}
	if _, err := NewLesta(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error when no vacancy page could be read")
	}
}

// join.lesta.team answers its listing with a 307 back to the same URL that sets a session
// cookie on the way; a client without a jar never presents it and follows the redirect until
// it gives up ("stopped after 10 redirects", every run from 2026-10-20). With a jar it is one
// redirect and a 200.
func TestLestaCrawlsWithACookieJar(t *testing.T) {
	src, ok := All(NewClient())["lesta"].(lesta)
	if !ok {
		t.Fatal("lesta is not registered as a lesta adapter")
	}
	c, ok := src.http.(*Client)
	if !ok || c.httpClient.Jar == nil {
		t.Error("lesta crawls without a cookie jar; its listing redirects to itself forever")
	}
}
