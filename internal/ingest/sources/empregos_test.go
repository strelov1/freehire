package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func empregosFixture(t *testing.T, name string) empregosResponse {
	t.Helper()
	b, err := os.ReadFile("testdata/empregos/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var r empregosResponse
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// empregosFake answers each search by its filters, first page only, the way the API does: at
// most `size` postings, with the slice's real total.
type empregosFake struct {
	mu      sync.Mutex
	answer  func(filters []empregosFilter) empregosResponse
	asked   []string
	sizes   []int
	failFor string
}

func (f *empregosFake) PostJSON(_ context.Context, u string, body, v any) error {
	if u != empregosSearchURL {
		return fmt.Errorf("unexpected url %s", u)
	}
	req := body.(empregosRequest)
	key := empregosDescribe(req.Filters)
	f.mu.Lock()
	f.asked = append(f.asked, key)
	f.sizes = append(f.sizes, req.Size)
	f.mu.Unlock()
	if key == f.failFor {
		return &StatusError{Code: 502}
	}
	resp := f.answer(req.Filters)
	if len(resp.Postings) > req.Size {
		resp.Postings = resp.Postings[:req.Size]
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func newEmpregosTest(f *empregosFake, leafMax, maxNew int) empregos {
	return empregos{http: f, limiter: noWait{}, leafMax: leafMax, maxNewPerRun: maxNew}
}

func empregosPostings(ids ...int64) []empregosPosting {
	out := make([]empregosPosting, len(ids))
	for i, id := range ids {
		n := strconv.FormatInt(id, 10)
		out[i] = empregosPosting{ReferenceID: id, Title: "Vaga " + n, Company: &empregosCompany{Name: "Empresa " + n}}
	}
	return out
}

func empregosFacets(facet int, counts map[string]int) []empregosFacet {
	f := empregosFacet{FacetItem: facet}
	for v, c := range counts {
		f.Items = append(f.Items, empregosFacetItem{v, c})
	}
	return []empregosFacet{f}
}

func TestEmpregosMapsPostings(t *testing.T) {
	leaf := empregosFixture(t, "leaf.json")
	f := &empregosFake{answer: func([]empregosFilter) empregosResponse { return leaf }}
	jobs, err := newEmpregosTest(f, 100, 100).Fetch(context.Background(), CompanyEntry{Provider: "empregos"})
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]Job)
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 15 {
		t.Fatalf("got %d jobs, want the 16 postings minus the hidden employer", len(jobs))
	}
	if _, ok := byID["11919284"]; ok {
		t.Error("the \"Empresa confidencial\" posting was yielded")
	}

	j := byID["11946733"]
	if j.URL != "https://www.empregos.com.br/vaga/11946733" || j.Company != "Angellira" || j.Title == "" {
		t.Errorf("identity = %q %q %q", j.URL, j.Company, j.Title)
	}
	if j.Location != "Chapecó, SC" || len(j.Countries) != 1 || j.Countries[0] != "br" {
		t.Errorf("place = %q %v", j.Location, j.Countries)
	}
	if j.WorkMode != "onsite" || j.Remote || j.EmploymentType != "" {
		t.Errorf("work mode %q remote %v type %q", j.WorkMode, j.Remote, j.EmploymentType)
	}
	want := time.Date(2026, 10, 22, 13, 5, 10, 0, time.UTC)
	if j.PostedAt == nil || j.PostedAt.Truncate(time.Second).UTC() != want {
		t.Errorf("posted %v, want %v (Brasília time)", j.PostedAt, want)
	}

	if j := byID["11859225"]; j.WorkMode != "remote" || !j.Remote {
		t.Errorf("Remoto posting: %q %v", j.WorkMode, j.Remote)
	}
	if j := byID["11893195"]; j.WorkMode != "hybrid" {
		t.Errorf("Híbrido posting: %q", j.WorkMode)
	}
	if j := byID["11813418"]; j.Company != "Alto Calçados" {
		t.Errorf("company not trimmed: %q", j.Company)
	}
	if j := byID["11953808"]; !strings.Contains(j.Description, "<") || j.Description == "" {
		t.Errorf("HTML body lost: %.200s", j.Description)
	}
}

// TestEmpregosPlainTextBody: a body with no markup still carries HTML entities ("&nbsp;"), which
// are decoded before the text is escaped into paragraphs rather than escaped a second time.
func TestEmpregosPlainTextBody(t *testing.T) {
	r := empregosFixture(t, "plaintext.json")
	j := r.Postings[0].job()
	if j.ExternalID != "11946328" {
		t.Fatalf("fixture order changed: %s", j.ExternalID)
	}
	if !strings.HasPrefix(j.Description, "<p>") || strings.Contains(j.Description, "&amp;nbsp;") || strings.Contains(j.Description, "&amp;") {
		t.Errorf("plain-text body not rebuilt: %.300s", j.Description)
	}
	if strings.Contains(j.Description, "Requisitos</h3>") {
		t.Errorf("a blank requiredQualification became a section: %.300s", j.Description)
	}
}

// TestEmpregosRealFacetPartitions pins the property the first cut relies on: the live level
// facet's counts add up to the catalogue's total.
func TestEmpregosRealFacetPartitions(t *testing.T) {
	root := empregosFixture(t, "root.json")
	subs, err := empregosFacetCut(empregosFacetLevel)(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 21 {
		t.Errorf("got %d levels, want 21", len(subs))
	}
	if _, err := empregosFacetCut(16)(root); err == nil {
		t.Error("the industry facet (top 25 of more) was accepted as a partition")
	}
}

// TestEmpregosCutsOversizedSlices walks the three cuts: level B is too large for one answer
// and is cut by state; its "sp" slice is still too large and is cut by workplace type.
func TestEmpregosCutsOversizedSlices(t *testing.T) {
	f := &empregosFake{answer: func(fs []empregosFilter) empregosResponse {
		var r empregosResponse
		switch empregosDescribe(fs) {
		case "the catalogue":
			r.Total = 26
			r.Postings = empregosPostings(1)
			r.Facets = empregosFacets(empregosFacetLevel, map[string]int{"A": 4, "B": 22})
		case `slice 10="A"`:
			r.Total, r.Postings = 4, empregosPostings(1, 2, 3, 4)
		case `slice 10="B"`:
			r.Total, r.Postings = 22, empregosPostings(10)
		case `slice 10="B" 1="sp"`:
			r.Total, r.Postings = 15, empregosPostings(10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20)
			r.Facets = empregosFacets(empregosFacetWorkplace, map[string]int{"Presencial": 9, "Remoto": 6})
		case `slice 10="B" 1="sp" 17="Presencial"`:
			r.Total, r.Postings = 9, empregosPostings(10, 11, 12, 13, 14, 15, 16, 17, 18)
		case `slice 10="B" 1="sp" 17="Remoto"`:
			r.Total, r.Postings = 6, empregosPostings(19, 20, 21, 22, 23, 24)
		case `slice 10="B" 1="pr"`:
			// 24 is listed in two states; 30 has no location and is in none.
			r.Total, r.Postings = 3, empregosPostings(24, 25, 26)
		}
		return r
	}}
	jobs, err := newEmpregosTest(f, 10, 100).Fetch(context.Background(), CompanyEntry{Provider: "empregos"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 21 {
		t.Errorf("got %d jobs, want 4 + 15 + 2 distinct", len(jobs))
	}
	if f.sizes[0] != 1 {
		t.Errorf("the catalogue was asked for %d postings, want a count-only request", f.sizes[0])
	}
	for i, k := range f.asked {
		if k == `slice 10="B"` && f.sizes[i] != 1 {
			t.Errorf("level B (known 22 > 10) was asked for %d postings", f.sizes[i])
		}
	}
	if n := len(f.asked); n != 1+2+27+2 {
		t.Errorf("made %d requests, want root + 2 levels + 27 states + 2 workplace types", n)
	}
}

func TestEmpregosFailsWhatItCannotRead(t *testing.T) {
	cases := map[string]func([]empregosFilter) empregosResponse{
		// Level A is served whole; the 26 postings under levels the facet cut off are not.
		"a truncated facet": func(fs []empregosFilter) empregosResponse {
			var r empregosResponse
			if len(fs) == 1 {
				r.Total, r.Postings = 4, empregosPostings(1, 2, 3, 4)
				return r
			}
			r.Total, r.Postings = 30, empregosPostings(1)
			r.Facets = empregosFacets(empregosFacetLevel, map[string]int{"A": 4})
			return r
		},
		"a slice no cut can shrink": func(fs []empregosFilter) empregosResponse {
			var r empregosResponse
			r.Total, r.Postings = 50, empregosPostings(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11)
			r.Facets = append(empregosFacets(empregosFacetLevel, map[string]int{"A": 50}), empregosFacets(empregosFacetWorkplace, map[string]int{"Presencial": 50})...)
			return r
		},
		"a short single answer": func(fs []empregosFilter) empregosResponse {
			var r empregosResponse
			r.Total, r.Postings = 5, empregosPostings(1, 2)
			return r
		},
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			f := &empregosFake{answer: answer}
			if _, err := newEmpregosTest(f, 10, 100).Fetch(context.Background(), CompanyEntry{}); err == nil {
				t.Fatal("crawl succeeded")
			}
		})
	}
	t.Run("a failed request", func(t *testing.T) {
		leaf := empregosFixture(t, "leaf.json")
		f := &empregosFake{answer: func([]empregosFilter) empregosResponse { return leaf }, failFor: "the catalogue"}
		if _, err := newEmpregosTest(f, 100, 100).Fetch(context.Background(), CompanyEntry{}); err == nil {
			t.Fatal("crawl succeeded")
		}
	})
}

func TestEmpregosRefreshesSeenAndBoundsNew(t *testing.T) {
	leaf := empregosFixture(t, "leaf.json")
	f := &empregosFake{answer: func([]empregosFilter) empregosResponse { return leaf }}
	seen := func(id string) bool { return id == "11946733" }
	jobs, err := newEmpregosTest(f, 100, 3).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatal(err)
	}
	var fresh, refreshed []Job
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshed = append(refreshed, j)
		} else {
			fresh = append(fresh, j)
		}
	}
	if len(refreshed) != 1 || refreshed[0].ExternalID != "11946733" || refreshed[0].Description != "" {
		t.Errorf("refreshed = %+v", refreshed)
	}
	if len(fresh) != 3 {
		t.Fatalf("wrote %d new postings, want the bound of 3", len(fresh))
	}
	// The newest three by posting time.
	for _, j := range fresh {
		if j.ExternalID != "11953808" && j.ExternalID != "11939282" && j.ExternalID != "11938347" {
			t.Errorf("wrote %s, which is not among the newest", j.ExternalID)
		}
	}
}
