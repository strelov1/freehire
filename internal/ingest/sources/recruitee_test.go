package sources

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestRecruiteeProvider(t *testing.T) {
	if got := NewRecruitee(nil).Provider(); got != "recruitee" {
		t.Errorf("Provider() = %q, want %q", got, "recruitee")
	}
}

// Recruitee earns fullBoardListing because Fetch is a single unpaginated request returning
// the board's whole offers array — no loop that could stop early, so any listing failure
// aborts the whole Fetch.
func TestRecruiteeMarkers(t *testing.T) {
	s := NewRecruitee(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("recruitee should implement the fullBoardListing marker")
	}
}

func TestRecruiteeRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["recruitee"] {
		t.Error("FullBoardListingProviders(All(nil)) should include recruitee")
	}
}

// A listing fetch failure must abort the whole Fetch, never return a partial result as
// success — the property TestRecruiteeMarkers' fullBoardListing claim rests on.
func TestRecruiteeFetchPropagatesAListingError(t *testing.T) {
	fake := &fakeHTTP{err: errors.New("boom")}
	if _, err := NewRecruitee(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded despite a listing error")
	}
}

func TestRecruiteeFetch(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"offers": [
			{
				"id": 42,
				"title": "Game Director",
				"careers_url": "https://acme.recruitee.com/o/game-director",
				"location": "Warsaw, Poland",
				"created_at": "2024-04-24 10:13:38 UTC",
				"remote": true,
				"description": "<h4>The role</h4><p>Lead the team.</p>",
				"requirements": "<h4>Requirements</h4><ul><li>7+ years</li></ul>"
			},
			{
				"id": 43,
				"title": "Artist",
				"careers_url": "https://acme.recruitee.com/o/artist",
				"location": "Remote",
				"created_at": "2024-04-24 10:13:38 UTC",
				"remote": true,
				"description": "<p>Make art.</p>",
				"requirements": ""
			}
		]
	}`}

	jobs, err := NewRecruitee(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "recruitee", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(fake.gotURL, "acme.recruitee.com") || !strings.Contains(fake.gotURL, "/offers") {
		t.Errorf("requested URL %q should target the board offers endpoint", fake.gotURL)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "42" {
		t.Errorf("ExternalID = %q, want the id", j.ExternalID)
	}
	if j.URL != "https://acme.recruitee.com/o/game-director" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Location != "Warsaw, Poland" {
		t.Errorf("Location = %q", j.Location)
	}
	if !j.Remote {
		t.Error("Remote = false, want true from the remote flag")
	}
	// Description combines description + requirements, sanitized.
	for _, want := range []string{"Lead the team.", "Requirements", "7+ years"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q, got %q", want, j.Description)
		}
	}
	if j.PostedAt == nil || j.PostedAt.UTC().Year() != 2024 {
		t.Errorf("PostedAt = %v, want parsed created_at (2024)", j.PostedAt)
	}

	// Empty requirements must not break assembly.
	if !strings.Contains(jobs[1].Description, "Make art.") {
		t.Errorf("second job description = %q", jobs[1].Description)
	}
}

// Recruitee exposes remote and hybrid as separate booleans, and they are mutually
// exclusive: a hybrid offer carries remote=false, so reading the remote flag alone drops
// the hybrid signal entirely.
func TestRecruiteeFetchHybridOffer(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"offers": [
			{"id": 1, "title": "Designer", "careers_url": "https://acme.recruitee.com/o/designer",
			 "location": "Warszawa", "created_at": "2024-03-01 10:00:00 UTC",
			 "remote": false, "hybrid": true, "description": "<p>Design.</p>", "requirements": ""}
		]
	}`}

	jobs, err := NewRecruitee(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "recruitee", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}

	if jobs[0].WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid from the hybrid flag", jobs[0].WorkMode)
	}
	if jobs[0].Remote {
		t.Error("Remote = true, want false: a hybrid offer is not remote")
	}
}

// Recruitee's board listing already carries the application form, so the adapter yields it
// with the job and the capture costs no request beyond the one the crawl was making anyway.
func TestRecruiteeFetchYieldsApplyForm(t *testing.T) {
	fake := &fakeHTTP{body: `{
		"offers": [
			{
				"id": 42,
				"title": "Game Director",
				"careers_url": "https://acme.recruitee.com/o/game-director",
				"created_at": "2024-04-24 10:13:38 UTC",
				"description": "<p>Lead.</p>",
				"options_cv": "required",
				"options_phone": "optional",
				"options_cover_letter": "off",
				"open_questions": [
					{"id": 7, "position": 1, "required": true, "kind": "single_choice",
					 "body": "Contract type?",
					 "open_question_options": [{"id": 91, "position": 0, "body": "B2B"}]}
				]
			}
		]
	}`}

	jobs, err := NewRecruitee(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "acme"})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("Fetch() = %d jobs, err=%v, want 1", len(jobs), err)
	}
	form := jobs[0].ApplyForm
	if form == nil {
		t.Fatal("ApplyForm = nil, want the form the listing already carried")
	}
	if form.Provider != "recruitee" {
		t.Errorf("provider = %q, want %q", form.Provider, "recruitee")
	}

	// One request, and only one: a Recruitee capture must not become a per-posting fetch.
	if !strings.Contains(fake.gotURL, "/api/offers/") {
		t.Errorf("requested %q, want only the board listing", fake.gotURL)
	}

	var ids []string
	for _, f := range form.Fields {
		ids = append(ids, f.ID)
	}
	for _, want := range []string{"name", "email", "cv", "phone", "7"} {
		if !slices.Contains(ids, want) {
			t.Errorf("captured %v, want it to include %q", ids, want)
		}
	}
	if slices.Contains(ids, "cover_letter") {
		t.Error("captured cover_letter, want it omitted when the employer switched it off")
	}
}

// An offer describing no form at all still yields one: the standard fields the platform
// always demands are themselves the answer to what applying costs.
func TestRecruiteeFetchAlwaysYieldsAForm(t *testing.T) {
	fake := &fakeHTTP{body: `{"offers":[{"id":1,"title":"Dev","careers_url":"https://a.recruitee.com/o/dev"}]}`}

	jobs, err := NewRecruitee(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "acme"})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("Fetch() = %d jobs, err=%v", len(jobs), err)
	}
	if jobs[0].ApplyForm == nil {
		t.Fatal("ApplyForm = nil, want the standard fields Recruitee always demands")
	}
}

// Live-verified (2026-08-14): a real Recruitee posting carries exactly this shape.
func TestRecruiteeFetchReadsSalary(t *testing.T) {
	fake := &fakeHTTP{body: `{"offers":[{"id":1,"title":"Marketing Manager",
		"salary":{"min":"50000","max":"70000","period":"year","currency":"EUR"}}]}`}

	jobs, err := NewRecruitee(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	j := jobs[0]
	if j.SalaryMin == nil || *j.SalaryMin != 50000 {
		t.Errorf("SalaryMin = %v, want 50000", j.SalaryMin)
	}
	if j.SalaryMax == nil || *j.SalaryMax != 70000 {
		t.Errorf("SalaryMax = %v, want 70000", j.SalaryMax)
	}
	if j.SalaryCurrency != "EUR" || j.SalaryPeriod != "year" {
		t.Errorf("SalaryCurrency/SalaryPeriod = %q/%q, want EUR/year", j.SalaryCurrency, j.SalaryPeriod)
	}
}

// The common case: every field present but null. Confirmed live — most Recruitee
// offers carry this shape, not an absent "salary" key.
func TestRecruiteeFetchAllNullSalaryLeavesEmpty(t *testing.T) {
	fake := &fakeHTTP{body: `{"offers":[{"id":1,"title":"Dev",
		"salary":{"min":null,"max":null,"period":null,"currency":null}}]}`}

	jobs, err := NewRecruitee(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if j := jobs[0]; j.SalaryMin != nil || j.SalaryMax != nil {
		t.Errorf("SalaryMin/Max = %v/%v, want both nil", j.SalaryMin, j.SalaryMax)
	}
}

func TestRecruiteeFetchOneSidedSalaryStillCounts(t *testing.T) {
	// A stated ceiling with no floor ("up to €70k") is still a real, usable salary.
	fake := &fakeHTTP{body: `{"offers":[{"id":1,"title":"Dev",
		"salary":{"min":null,"max":"70000","period":"year","currency":"EUR"}}]}`}

	jobs, err := NewRecruitee(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	j := jobs[0]
	if j.SalaryMin != nil {
		t.Errorf("SalaryMin = %v, want nil", j.SalaryMin)
	}
	if j.SalaryMax == nil || *j.SalaryMax != 70000 {
		t.Errorf("SalaryMax = %v, want 70000", j.SalaryMax)
	}
}
