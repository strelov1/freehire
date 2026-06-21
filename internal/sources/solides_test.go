package sources

import (
	"context"
	"strings"
	"testing"
)

func TestSolidesProvider(t *testing.T) {
	if got := NewSolides(nil).Provider(); got != "solides" {
		t.Errorf("Provider() = %q, want %q", got, "solides")
	}
}

func TestSolidesFetchMapsFieldsAndPaginates(t *testing.T) {
	// Page 1 declares totalPages=2, so the walk continues to page 2. The first job
	// exercises every mapping; the closed job (currentState != "em_andamento") is
	// dropped before page 2's open job.
	page1 := `{"data":{"count":3,"currentPage":1,"totalPages":2,"data":[
		{"id":2001,"title":"  Backend Engineer  ",
		 "description":"<p>Build things</p><script>evil()</script>",
		 "currentState":"em_andamento",
		 "city":{"name":"Florianópolis"},"state":{"name":"Santa Catarina","code":"SC"},
		 "homeOffice":false,"jobType":"hibrido","createdAt":"2026-06-11",
		 "redirectLink":"https://malformed/x"},
		{"id":2002,"title":"Closed Role","currentState":"encerrada",
		 "city":{"name":"São Paulo"},"state":{"name":"São Paulo","code":"SP"},
		 "homeOffice":false,"jobType":"presencial","createdAt":"2026-06-10"}
	]}}`

	page2 := `{"data":{"count":3,"currentPage":2,"totalPages":2,"data":[
		{"id":2003,"title":"Remote SRE","description":"<p>On call</p>",
		 "currentState":"em_andamento",
		 "city":{"name":""},"state":{"name":"","code":""},
		 "homeOffice":true,"jobType":"remoto","createdAt":"2026-06-09"}
	]}}`

	fake := (&routedHTTP{}).
		route("page=1", page1).
		route("page=2", page2)

	jobs, err := NewSolides(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Dynamox", Provider: "solides", Board: "dynamox",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// 1 open from page 1 (closed one dropped) + 1 open from page 2 = 2.
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (one closed job dropped across two pages)", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	be, ok := byID["2001"]
	if !ok {
		t.Fatal("posting 2001 missing")
	}
	if be.Title != "Backend Engineer" {
		t.Errorf("Title = %q, want trimmed title", be.Title)
	}
	if be.URL != "https://dynamox.vagas.solides.com.br/vaga/2001" {
		t.Errorf("URL = %q, want built vaga URL (not redirectLink)", be.URL)
	}
	if be.Company != "Dynamox" {
		t.Errorf("Company = %q, want configured company", be.Company)
	}
	if be.Location != "Florianópolis, SC" {
		t.Errorf("Location = %q, want city.name + state.code joined", be.Location)
	}
	if be.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid from jobType hibrido", be.WorkMode)
	}
	if be.Remote {
		t.Error("Remote = true, want false from homeOffice")
	}
	if !strings.Contains(be.Description, "Build things") {
		t.Errorf("Description missing body, got %q", be.Description)
	}
	if strings.Contains(be.Description, "evil") {
		t.Errorf("Description must be sanitized, got %q", be.Description)
	}
	if be.PostedAt == nil || be.PostedAt.UTC().Year() != 2026 {
		t.Errorf("PostedAt = %v, want parsed createdAt (2026)", be.PostedAt)
	}

	if _, present := byID["2002"]; present {
		t.Error("posting 2002 is not em_andamento and must be dropped")
	}

	sre, ok := byID["2003"]
	if !ok {
		t.Fatal("posting 2003 missing")
	}
	if !sre.Remote {
		t.Error("Remote = false, want true from homeOffice")
	}
	if sre.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote from jobType remoto", sre.WorkMode)
	}
	if sre.Location != "" {
		t.Errorf("Location = %q, want empty when city/state are blank", sre.Location)
	}
}

func TestSolidesFetchStopsAtLastPage(t *testing.T) {
	// A single-page board (totalPages=1) ends the walk after one request.
	fake := (&routedHTTP{}).
		route("page=1", `{"data":{"count":1,"currentPage":1,"totalPages":1,"data":[
			{"id":1,"title":"Only Role","currentState":"em_andamento",
			 "city":{"name":"Joinville"},"state":{"code":"SC"},
			 "homeOffice":false,"jobType":"presencial","createdAt":"2026-06-01"}
		]}}`)

	jobs, err := NewSolides(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Reivax", Provider: "solides", Board: "reivax",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 from the single page", len(jobs))
	}
	if jobs[0].WorkMode != "onsite" {
		t.Errorf("WorkMode = %q, want onsite from jobType presencial", jobs[0].WorkMode)
	}
	if fake.calls != 1 {
		t.Errorf("made %d HTTP calls, want 1 (totalPages=1 must stop the walk)", fake.calls)
	}
}
