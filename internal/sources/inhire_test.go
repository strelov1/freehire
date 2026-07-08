package sources

import (
	"context"
	"strings"
	"testing"
)

func TestInhireProvider(t *testing.T) {
	if got := NewInhire(nil).Provider(); got != "inhire" {
		t.Errorf("Provider() = %q, want %q", got, "inhire")
	}
}

// inhireListBody builds a listing response with one job post.
func inhireListBody(jobID, displayName, workplaceType, location string) string {
	return `{"jobsPage":[
		{"jobId":"` + jobID + `","displayName":"` + displayName + `","workplaceType":"` + workplaceType + `","location":"` + location + `","status":"published"}
	]}`
}

// inhireDetailBody builds a detail response for one job post.
func inhireDetailBody(workplaceType, location, description, publishedAt, createdAt string) string {
	return `{"description":"` + description + `","workplaceType":"` + workplaceType + `","location":"` + location +
		`","publishedAt":"` + publishedAt + `","createdAt":"` + createdAt + `"}`
}

func TestInhireFetchMapsListAndDetail(t *testing.T) {
	const jobID = "11111111-2222-3333-4444-555555555555"
	fake := (&routedHTTP{}).
		route("/job-posts/public/pages/"+jobID,
			inhireDetailBody("Remote", "São Paulo, BR",
				"&lt;p&gt;Build &amp; ship.&lt;/p&gt;", "2026-06-15T10:00:00Z", "2026-06-10T10:00:00Z")).
		route("/job-posts/public/pages",
			inhireListBody(jobID, "  Senior Backend Engineer  ", "Remote", "São Paulo, BR"))

	jobs, err := NewInhire(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Conta Azul", Provider: "inhire", Board: "contaazul",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != jobID {
		t.Errorf("ExternalID = %q, want %q", j.ExternalID, jobID)
	}
	if j.Title != "Senior Backend Engineer" {
		t.Errorf("Title = %q, want the trimmed display name", j.Title)
	}
	if j.Company != "Conta Azul" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if j.Location != "São Paulo, BR" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.URL != "https://contaazul.inhire.app/vagas/"+jobID {
		t.Errorf("URL = %q", j.URL)
	}
	if j.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote", j.WorkMode)
	}
	if !j.Remote {
		t.Errorf("Remote = false, want true for a Remote workplace type")
	}
	if !strings.Contains(j.Description, "Build &amp; ship.") {
		t.Errorf("Description = %q, want the unescaped+sanitized HTML body", j.Description)
	}
	if strings.Contains(j.Description, "&lt;p&gt;") {
		t.Errorf("Description still HTML-entity-encoded: %q", j.Description)
	}
	if j.PostedAt == nil {
		t.Fatalf("PostedAt = nil, want the parsed publishedAt")
	}
	if got := j.PostedAt.UTC().Format("2006-01-02"); got != "2026-06-15" {
		t.Errorf("PostedAt = %s, want 2026-06-15 (publishedAt)", got)
	}
}

func TestInhireFetchMapsWorkModes(t *testing.T) {
	tests := []struct {
		workplaceType string
		wantMode      string
		wantRemote    bool
	}{
		{"Remote", "remote", true},
		{"Hybrid", "hybrid", false},
		{"On-site", "onsite", false},
	}
	for _, tt := range tests {
		t.Run(tt.workplaceType, func(t *testing.T) {
			const jobID = "aaaa"
			fake := (&routedHTTP{}).
				route("/job-posts/public/pages/"+jobID,
					inhireDetailBody(tt.workplaceType, "Florianópolis, BR", "<p>x</p>", "", "2026-06-10T10:00:00Z")).
				route("/job-posts/public/pages",
					inhireListBody(jobID, "Role", tt.workplaceType, "Florianópolis, BR"))

			jobs, err := NewInhire(fake).Fetch(context.Background(), CompanyEntry{
				Company: "Involves", Provider: "inhire", Board: "involves",
			})
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if len(jobs) != 1 {
				t.Fatalf("len(jobs) = %d, want 1", len(jobs))
			}
			if jobs[0].WorkMode != tt.wantMode {
				t.Errorf("WorkMode = %q, want %q", jobs[0].WorkMode, tt.wantMode)
			}
			if jobs[0].Remote != tt.wantRemote {
				t.Errorf("Remote = %v, want %v", jobs[0].Remote, tt.wantRemote)
			}
		})
	}
}

func TestInhireFetchFallsBackToCreatedAt(t *testing.T) {
	const jobID = "bbbb"
	fake := (&routedHTTP{}).
		route("/job-posts/public/pages/"+jobID,
			inhireDetailBody("Remote", "", "<p>x</p>", "", "2026-06-08T09:00:00Z")).
		route("/job-posts/public/pages",
			inhireListBody(jobID, "Role", "Remote", ""))

	jobs, err := NewInhire(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Neoway", Provider: "inhire", Board: "neoway",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if jobs[0].PostedAt == nil {
		t.Fatalf("PostedAt = nil, want createdAt fallback")
	}
	if got := jobs[0].PostedAt.UTC().Format("2006-01-02"); got != "2026-06-08" {
		t.Errorf("PostedAt = %s, want 2026-06-08 (createdAt fallback)", got)
	}
}

func TestInhireFetchSkipsFailedDetail(t *testing.T) {
	// The second post's detail returns invalid JSON -> its detail decode errors and it is
	// skipped, but the first still comes through.
	list := `{"jobsPage":[
		{"jobId":"good","displayName":"Good","workplaceType":"Remote","location":"BR","status":"published"},
		{"jobId":"broken","displayName":"Broken","workplaceType":"Remote","location":"BR","status":"published"}
	]}`
	fake := (&routedHTTP{}).
		route("/job-posts/public/pages/good",
			inhireDetailBody("Remote", "BR", "<p>ok</p>", "2026-06-10T10:00:00Z", "")).
		route("/job-posts/public/pages/broken", `}{ not json`).
		route("/job-posts/public/pages", list)

	jobs, err := NewInhire(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "inhire", Board: "acme",
	})
	if err != nil {
		t.Fatalf("Fetch should not abort on one failed detail: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "good" {
		t.Fatalf("want only the good post, got %d jobs", len(jobs))
	}
}
