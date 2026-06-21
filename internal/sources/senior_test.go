package sources

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSeniorProvider(t *testing.T) {
	if got := NewSenior(nil).Provider(); got != "senior" {
		t.Errorf("Provider() = %q, want %q", got, "senior")
	}
}

// seniorHTTP is a body-aware test JSONPoster for the Senior bridge API: the resolve,
// search, and detail endpoints share one host and differ by path, and the paged search
// carries its page number in the POST body (not the URL). It routes on the path suffix
// and, for the search endpoint, on the requested page parsed from the body.
type seniorHTTP struct {
	profileID   string // resolve response; empty -> resolve fails (non-tenant)
	pages       map[int]string
	details     map[string]string // vacancy id -> findVacancyById response
	failDetails map[string]bool   // vacancy id -> detail request errors
}

func (f *seniorHTTP) PostJSON(_ context.Context, url string, body, v any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	switch {
	case strings.Contains(url, "getProfileIdBySubdomain"):
		if f.profileID == "" {
			return errors.New("seniorHTTP: subdomain not found")
		}
		return json.Unmarshal([]byte(`{"profileId":"`+f.profileID+`"}`), v)
	case strings.Contains(url, "searchVacancies"):
		var req struct {
			Page int `json:"page"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			return err
		}
		page, ok := f.pages[req.Page]
		if !ok {
			page = `{"totalPages":0,"totalElements":0,"contents":[]}`
		}
		return json.Unmarshal([]byte(page), v)
	case strings.Contains(url, "findVacancyById"):
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			return err
		}
		if f.failDetails[req.ID] {
			return errors.New("seniorHTTP: detail boom")
		}
		d, ok := f.details[req.ID]
		if !ok {
			return errors.New("seniorHTTP: no detail for " + req.ID)
		}
		return json.Unmarshal([]byte(d), v)
	}
	return errors.New("seniorHTTP: no route for " + url)
}

func TestSeniorFetchResolvesSearchesAndMaps(t *testing.T) {
	fake := &seniorHTTP{
		profileID: "GUID-1",
		pages: map[int]string{
			0: `{"totalPages":2,"totalElements":3,"contents":[
				{"vacancy":{
					"id":"v1","title":"Backend Engineer",
					"localization":{"city":"Florianópolis","province":"SC","country":"Brasil"},
					"jobModel":["IN_PERSON"],
					"publication":{"startDate":"2026-06-10"}
				},"company":{"tenant":"intelbras","name":"Intelbras"}},
				{"vacancy":{
					"id":"v2","title":"Remote Data Engineer",
					"localization":{"city":"","province":"","country":"Brasil"},
					"jobModel":["REMOTE"],
					"publication":{"startDate":"2026-06-11"}
				},"company":{"tenant":"intelbras","name":"Intelbras"}}
			]}`,
			1: `{"totalPages":2,"totalElements":3,"contents":[
				{"vacancy":{
					"id":"v3","title":"Hybrid Product Manager",
					"localization":{"city":"São Paulo","province":"SP","country":"Brasil"},
					"jobModel":["HYBRID"],
					"publication":{"startDate":"2026-06-12"}
				},"company":{"tenant":"intelbras","name":"Intelbras"}}
			]}`,
		},
		details: map[string]string{
			"v1": `{"vacancy":{"about":{"description":"<p>Build the backend.</p><script>alert(1)</script>"}}}`,
			"v2": `{"vacancy":{"about":{"description":"<p>Crunch data remotely.</p>"}}}`,
			"v3": `{"vacancy":{"about":{"description":"<p>Own the roadmap.</p>"}}}`,
		},
	}

	jobs, err := NewSenior(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Intelbras", Provider: "senior", Board: "intelbras",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3 (both pages)", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	v1, ok := byID["v1"]
	if !ok {
		t.Fatal("vacancy v1 missing")
	}
	if v1.Title != "Backend Engineer" {
		t.Errorf("Title = %q", v1.Title)
	}
	if v1.Company != "Intelbras" {
		t.Errorf("Company = %q", v1.Company)
	}
	if v1.Location != "Florianópolis, SC, Brasil" {
		t.Errorf("Location = %q, want joined city/province/country", v1.Location)
	}
	if v1.WorkMode != "onsite" {
		t.Errorf("WorkMode = %q, want onsite from IN_PERSON", v1.WorkMode)
	}
	if v1.Remote {
		t.Error("Remote = true, want false for IN_PERSON")
	}
	if v1.URL != "https://intelbras.portaldetalentos.senior.com.br/vacancy/v1" {
		t.Errorf("URL = %q", v1.URL)
	}
	if v1.PostedAt == nil || v1.PostedAt.UTC().Year() != 2026 {
		t.Errorf("PostedAt = %v, want parsed startDate (2026)", v1.PostedAt)
	}
	if !strings.Contains(v1.Description, "Build the backend.") {
		t.Errorf("Description = %q, want sanitized about.description", v1.Description)
	}
	if strings.Contains(v1.Description, "<script>") {
		t.Errorf("Description = %q, want script stripped", v1.Description)
	}

	v2 := byID["v2"]
	if v2.Location != "Brasil" {
		t.Errorf("v2 Location = %q, want empty city/province skipped", v2.Location)
	}
	if v2.WorkMode != "remote" {
		t.Errorf("v2 WorkMode = %q, want remote", v2.WorkMode)
	}
	if !v2.Remote {
		t.Error("v2 Remote = false, want true from REMOTE jobModel")
	}

	v3 := byID["v3"]
	if v3.WorkMode != "hybrid" {
		t.Errorf("v3 WorkMode = %q, want hybrid", v3.WorkMode)
	}
	if v3.Remote {
		t.Error("v3 Remote = true, want false for HYBRID")
	}
}

func TestSeniorFetchSkipsFailedDetail(t *testing.T) {
	// v2's detail errors -> it is skipped, v1 still comes through.
	fake := &seniorHTTP{
		profileID: "GUID-1",
		pages: map[int]string{
			0: `{"totalPages":1,"totalElements":2,"contents":[
				{"vacancy":{"id":"v1","title":"Engineer","localization":{"city":"Blumenau","province":"SC","country":"Brasil"},"jobModel":["IN_PERSON"],"publication":{"startDate":"2026-06-10"}},"company":{"tenant":"intelbras","name":"Intelbras"}},
				{"vacancy":{"id":"v2","title":"Broken","localization":{"city":"NYC"},"jobModel":[],"publication":{"startDate":""}},"company":{"tenant":"intelbras","name":"Intelbras"}}
			]}`,
		},
		details:     map[string]string{"v1": `{"vacancy":{"about":{"description":"<p>ok</p>"}}}`},
		failDetails: map[string]bool{"v2": true},
	}

	jobs, err := NewSenior(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Intelbras", Provider: "senior", Board: "intelbras",
	})
	if err != nil {
		t.Fatalf("Fetch should not abort the board on one failed detail: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "v1" {
		t.Fatalf("want only v1 to survive, got %d jobs", len(jobs))
	}
}

func TestSeniorFetchFailsWhenSubdomainUnresolved(t *testing.T) {
	// A non-tenant subdomain fails resolve -> Fetch returns an error (the board is bad,
	// not an empty board).
	fake := &seniorHTTP{}
	if _, err := NewSenior(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Nope", Provider: "senior", Board: "not-a-tenant",
	}); err == nil {
		t.Fatal("Fetch = nil error, want error when resolve fails")
	}
}
