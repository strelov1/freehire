package sources

import (
	"context"
	"os"
	"strings"
	"testing"
)

// viterbitFixture reads a page captured from cear.viterbit.site through the Scrapling tier
// (2026-10-27: 20 open jobs); scripts, styles and inline SVG are stripped, the JobPosting kept.
func viterbitFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/viterbit/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func viterbitCearHTTP(t *testing.T) *routedHTTP {
	r := &routedHTTP{}
	for _, id := range []string{"NvlVV9pBIZOQ", "eSr1YZ8Giy6v"} {
		r.route("-"+id+"/", viterbitFixture(t, "job_"+id+".html"))
	}
	return r.route("cear.viterbit.site/", viterbitFixture(t, "list_cear.html"))
}

var viterbitCearEntry = CompanyEntry{Company: "CEAR", Provider: "viterbit", Board: "cear"}

func TestViterbitProvider(t *testing.T) {
	s, ok := All(nil)["viterbit"]
	if !ok || s.Provider() != "viterbit" {
		t.Fatal("All() must register viterbit")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("viterbit must be a HydratingSource: a body costs a page request")
	}
}

func TestViterbitListsTheSiteAndRefreshesSeenJobs(t *testing.T) {
	fake := viterbitCearHTTP(t)
	jobs, err := NewViterbit(fake).(HydratingSource).FetchNew(context.Background(), viterbitCearEntry,
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	// 25 cards on the page carry 20 distinct jobs (a featured strip repeats some).
	if len(jobs) != 20 || fake.calls != 1 {
		t.Fatalf("jobs/requests = %d/%d, want the page's 20 jobs from one request", len(jobs), fake.calls)
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.Title == "" || len(j.ExternalID) != 12 || !strings.HasSuffix(j.URL, "-"+j.ExternalID+"/") {
			t.Errorf("job = %+v, want an identity-only SeenRefresh keyed by the 12-character id", j)
		}
	}
}

func TestViterbitMapsAJobPosting(t *testing.T) {
	fake := viterbitCearHTTP(t)
	jobs, err := NewViterbit(fake).(HydratingSource).FetchNew(context.Background(), viterbitCearEntry,
		func(id string) bool { return id != "NvlVV9pBIZOQ" && id != "eSr1YZ8Giy6v" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	a := byID["NvlVV9pBIZOQ"]
	if a.SeenRefresh || a.Unreadable || a.Title != "Abogado/a Emergencia - Temporal - CEUTA" {
		t.Fatalf("job = %+v, want it hydrated", a)
	}
	if a.Location != "Ceuta, ES" || len(a.Countries) != 1 || a.Countries[0] != "es" || a.WorkMode != "" {
		t.Errorf("Location/Countries/WorkMode = %q/%v/%q", a.Location, a.Countries, a.WorkMode)
	}
	if a.PostedAt == nil || a.PostedAt.Format("2006-01-02") != "2026-09-24" || !strings.Contains(a.Description, "Objetivo del Puesto") {
		t.Errorf("PostedAt = %v, Description = %.100q", a.PostedAt, a.Description)
	}
	if r := byID["eSr1YZ8Giy6v"]; r.WorkMode != "remote" || !r.Remote {
		t.Errorf("TELECOMMUTE job WorkMode = %q, want remote", r.WorkMode)
	}
}

func TestViterbitDefersPastTheBudget(t *testing.T) {
	fake := viterbitCearHTTP(t)
	s := viterbit{http: fake, maxNewPerRun: 1}
	jobs, err := s.FetchNew(context.Background(), viterbitCearEntry,
		func(id string) bool { return id != "NvlVV9pBIZOQ" && id != "eSr1YZ8Giy6v" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 19 || fake.calls != 2 {
		t.Fatalf("jobs/requests = %d/%d, want 18 refreshed + 1 hydrated from two requests", len(jobs), fake.calls)
	}
}

func TestViterbitRefusesAForeignBoard(t *testing.T) {
	if _, err := NewViterbit(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: "evil.com/x"}); err == nil {
		t.Fatal("Fetch: want an error for a board that is not a site label")
	}
}
