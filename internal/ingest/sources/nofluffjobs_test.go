package sources

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
)

// nofluffjobsFake serves the streamed listing and routes detail calls by the slug in the URL.
type nofluffjobsFake struct {
	listing     string
	detailByURL map[string]string
	detailErr   map[string]bool
	// The adapter fans its detail fetches out across a worker pool, so this recorder is
	// written from several goroutines at once and needs the lock. The assertions read it
	// after Fetch has joined the pool, which is already ordered.
	mu          sync.Mutex
	detailSlugs []string
}

func (f *nofluffjobsFake) GetStream(_ context.Context, _ string, _ string, fn func(io.Reader) error) error {
	return fn(strings.NewReader(f.listing))
}

func (f *nofluffjobsFake) GetJSON(_ context.Context, url string, v any) error {
	slug := url[strings.LastIndex(url, "/")+1:]
	f.mu.Lock()
	f.detailSlugs = append(f.detailSlugs, slug)
	f.mu.Unlock()
	if f.detailErr[slug] {
		return errors.New("detail boom")
	}
	return json.Unmarshal([]byte(f.detailByURL[slug]), v)
}

const nofluffjobsListingJSON = `{"postings":[
  {"id":"gcp-verita-Kraków","url":"gcp-verita-krakow","name":"Verita HR","title":"GCP Data ETL","technology":"Python","category":"data","seniority":["Mid"],"fullyRemote":false,"posted":1784034271774,"location":{"places":[{"city":"Kraków","country":{"code":"POL","name":"Poland"}}],"fullyRemote":false}},
  {"id":"remote-dev","url":"remote-dev","name":"Acme","title":"Backend Dev","technology":"Java","seniority":["Senior"],"fullyRemote":false,"posted":1784034271774,"location":{"places":[{"city":"Remote"}],"fullyRemote":true}},
  {"id":"remote-in-poland","url":"remote-in-poland","name":"Acme","title":"AI Engineer","technology":"Python","seniority":["Senior"],"fullyRemote":false,"posted":1784034271774,"location":{"places":[
    {"city":"Remote"},
    {"city":"Warsaw","street":"Puławska 2","country":{"code":"POL","name":"Poland"}},
    {"province":"lower-silesian","provinceOnly":true,"country":{"code":"POL","name":"Poland"}},
    {"city":"Budapest","country":{"code":"HUN","name":"Hungary"}}
  ],"fullyRemote":true}},
  {"id":"no-company","url":"no-company","name":"","title":"X","seniority":["Junior"],"posted":1784034271774}
]}`

func TestNoFluffJobsFetchMapsAndDrops(t *testing.T) {
	jobs, err := NewNoFluffJobs(&nofluffjobsFake{listing: nofluffjobsListingJSON}).
		Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3 (no-company dropped)", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	j := byID["gcp-verita-Kraków"]
	if j.URL != "https://nofluffjobs.com/job/gcp-verita-krakow" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "GCP Data ETL" || j.Company != "Verita HR" {
		t.Errorf("title/company: %q / %q", j.Title, j.Company)
	}
	if j.Location != "Kraków, Poland" {
		t.Errorf("Location = %q, want 'Kraków, Poland'", j.Location)
	}
	if j.Seniority != "middle" {
		t.Errorf("Seniority = %q, want middle (Mid->middle)", j.Seniority)
	}
	if len(j.Skills) == 0 {
		t.Errorf("Skills empty, want the canonicalized technology")
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2026 {
		t.Errorf("PostedAt = %v, want 2026 (epoch ms)", j.PostedAt)
	}
	if j.Remote {
		t.Error("Remote = true for a Kraków job, want false")
	}
	if !slices.Equal(j.Countries, []string{"pl"}) {
		t.Errorf("Countries = %v, want [pl]", j.Countries)
	}
	// The remote flag lives in location.fullyRemote; the posting-level "fullyRemote" beside it
	// is false on every posting the feed serves, so reading that one loses remoteness entirely.
	r := byID["remote-dev"]
	if !r.Remote || r.WorkMode != "remote" || r.Seniority != "senior" {
		t.Errorf("remote job: Remote=%v WorkMode=%q Seniority=%q", r.Remote, r.WorkMode, r.Seniority)
	}
	if r.Location != "Remote" {
		t.Errorf("Location = %q, want 'Remote' (the marker is the only place)", r.Location)
	}
	if len(r.Countries) != 0 {
		t.Errorf("Countries = %v, want none (no place states a country)", r.Countries)
	}
	// A remote posting lists the "Remote" pseudo-place AHEAD of its real offices. It states
	// remoteness, not geography — reading it as the location erases the country and drops the
	// job into the Global/Worldwide bucket.
	p := byID["remote-in-poland"]
	if p.Location != "Warsaw, Poland" {
		t.Errorf("Location = %q, want 'Warsaw, Poland' (the Remote marker is skipped)", p.Location)
	}
	if !p.Remote || p.WorkMode != "remote" {
		t.Errorf("remote-in-poland: Remote=%v WorkMode=%q, want remote", p.Remote, p.WorkMode)
	}
	if !slices.Equal(p.Countries, []string{"pl", "hu"}) {
		t.Errorf("Countries = %v, want [pl hu] (every place, first-seen order)", p.Countries)
	}
}

func TestNoFluffJobsFetchNewHydratesNewOnly(t *testing.T) {
	const listing = `{"postings":[
	  {"id":"seen-1","url":"seen-1","name":"Co","title":"A","seniority":["Mid"],"posted":1784034271774},
	  {"id":"new-2","url":"new-2","name":"Co","title":"B","seniority":["Mid"],"posted":1784034271774},
	  {"id":"err-3","url":"err-3","name":"Co","title":"C","seniority":["Mid"],"posted":1784034271774}
	]}`
	fake := &nofluffjobsFake{
		listing: listing,
		detailByURL: map[string]string{
			"new-2": `{"details":{"description":"<span>Great offer</span>"},"requirements":{"description":"<ul><li>Java skills</li></ul>"}}`,
		},
		detailErr: map[string]bool{"err-3": true},
	}
	seen := func(id string) bool { return id == "seen-1" }
	jobs, err := NewNoFluffJobs(fake).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3", len(jobs))
	}
	// Detail is fetched only for the unseen postings (new-2, err-3), never for seen-1.
	slices.Sort(fake.detailSlugs)
	if !slices.Equal(fake.detailSlugs, []string{"err-3", "new-2"}) {
		t.Errorf("detail slugs = %v, want [err-3 new-2] (seen-1 not fetched)", fake.detailSlugs)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if !byID["seen-1"].SeenRefresh || byID["seen-1"].Description != "" {
		t.Errorf("seen-1: SeenRefresh=%v desc=%q, want refresh-only with empty desc", byID["seen-1"].SeenRefresh, byID["seen-1"].Description)
	}
	if d := byID["new-2"].Description; !strings.Contains(d, "Great offer") || !strings.Contains(d, "Java skills") {
		t.Errorf("new-2 description = %q, want details+requirements assembled", d)
	}
	if byID["err-3"].SeenRefresh || byID["err-3"].Description != "" {
		t.Errorf("err-3 should fall back to list-only (empty desc, not a refresh), got desc=%q refresh=%v", byID["err-3"].Description, byID["err-3"].SeenRefresh)
	}
}

func TestNoFluffJobsSeniority(t *testing.T) {
	cases := map[string]string{
		"Mid":     "middle",
		"Trainee": "intern",
		"Expert":  "principal",
		"Junior":  "junior",
		"Senior":  "senior",
		"Manager": "", // not a freehire seniority — dropped, not guessed
		"":        "",
	}
	for in, want := range cases {
		var levels []string
		if in != "" {
			levels = []string{in}
		}
		if got := nofluffjobsSeniority(levels); got != want {
			t.Errorf("nofluffjobsSeniority(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNoFluffJobsProviderRegistered(t *testing.T) {
	s := NewNoFluffJobs(nil)
	if s.Provider() != "nofluffjobs" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	if _, ok := s.(boardless); !ok {
		t.Error("nofluffjobs should be boardless")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("nofluffjobs should be an aggregator")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("nofluffjobs should implement HydratingSource")
	}
	if _, ok := All(nil)["nofluffjobs"]; !ok {
		t.Error("All() should register nofluffjobs")
	}
	if !slices.Contains(FilterableProviders(), "nofluffjobs") {
		t.Error("FilterableProviders() should include nofluffjobs")
	}
}
