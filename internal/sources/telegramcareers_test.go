package sources

import (
	"context"
	"strings"
	"testing"
)

// telegramJobsHTML builds a telegram.org/jobs-shaped page: a #dev_page_content wrapper
// whose each role is an <h3> title followed by its description markup until the next <h3>.
func telegramJobsHTML(titlesAndBodies ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><div id="dev_page_content"><h1>Jobs</h1><p>intro</p>`)
	for i := 0; i+1 < len(titlesAndBodies); i += 2 {
		b.WriteString(`<h3>` + titlesAndBodies[i] + `</h3>` + titlesAndBodies[i+1])
	}
	b.WriteString(`</div></body></html>`)
	return b.String()
}

func TestTelegramCareersFetchParsesJobs(t *testing.T) {
	page := telegramJobsHTML(
		"Content Moderator", `<p>Responsibilities: <b>sort</b> content.</p><ul><li>care</li></ul>`,
		"C/C++ Software Engineer", `<p>Work on storage engines.</p><script>x</script>`,
	)
	fake := (&routedHTTP{}).route("telegram.org/jobs", page)

	jobs, err := NewTelegramCareers(fake).Fetch(context.Background(), CompanyEntry{Company: "Telegram", Provider: "telegramcareers"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	j := jobs[0]
	if j.Title != "Content Moderator" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.ExternalID != "content-moderator" {
		t.Errorf("ExternalID = %q, want content-moderator", j.ExternalID)
	}
	if j.Company != "Telegram" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.URL != "https://telegram.org/jobs" {
		t.Errorf("URL = %q", j.URL)
	}
	if !strings.Contains(j.Description, "Responsibilities") || !strings.Contains(j.Description, "<b>sort</b>") {
		t.Errorf("Description missing body: %q", j.Description)
	}
	// The next role's content must not bleed into this one.
	if strings.Contains(j.Description, "storage engines") {
		t.Errorf("Description bled into the next role: %q", j.Description)
	}
	// Second role's description is sanitized (script stripped).
	if strings.Contains(jobs[1].Description, "<script>") {
		t.Errorf("Description not sanitized: %q", jobs[1].Description)
	}
	if jobs[1].ExternalID != "c-c-software-engineer" {
		t.Errorf("ExternalID = %q, want c-c-software-engineer", jobs[1].ExternalID)
	}
}

func TestTelegramCareersProvider(t *testing.T) {
	if got := NewTelegramCareers(nil).Provider(); got != "telegramcareers" {
		t.Errorf("Provider() = %q, want telegramcareers", got)
	}
}

func TestTelegramCareersRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["telegramcareers"]
	if !ok {
		t.Fatal("All() missing provider telegramcareers")
	}
	if s.Provider() != "telegramcareers" {
		t.Errorf("All()[telegramcareers].Provider() = %q", s.Provider())
	}
}
