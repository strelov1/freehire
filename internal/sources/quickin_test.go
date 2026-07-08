package sources

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestQuickinPaginatesWhenPagesFieldMissing(t *testing.T) {
	// A response that omits (or zeroes) `pages` must not truncate a full board to its
	// first page: a full page (len == limit) means "there may be more", so the walk
	// continues until a short page. Page 1 is full (100 docs, pages:0); page 2 is short.
	var b strings.Builder
	b.WriteString(`{"page":1,"pages":0,"docs":[`)
	for i := 0; i < quickinPageSize; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"_id":"p1-%d","publicate":"published","title":"Role %d","workplace_type":"remote"}`, i, i)
	}
	b.WriteString(`]}`)
	page1 := b.String()
	page2 := `{"page":2,"pages":0,"docs":[{"_id":"p2-0","publicate":"published","title":"Last","workplace_type":"remote"}]}`

	fake := (&routedHTTP{}).
		route("/public/accounts/acme", `{"_id":"acc","name":"Acme"}`).
		route("page=1", page1).
		route("page=2", page2)

	jobs, err := NewQuickin(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Provider: "quickin", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// 100 from the full page 1 + 1 from page 2 = 101; a pages:0 early-stop would yield 100.
	if len(jobs) != quickinPageSize+1 {
		t.Fatalf("len(jobs) = %d, want %d (missing pages field must not truncate)", len(jobs), quickinPageSize+1)
	}
}

func TestQuickinProvider(t *testing.T) {
	if got := NewQuickin(nil).Provider(); got != "quickin" {
		t.Errorf("Provider() = %q, want %q", got, "quickin")
	}
}

func TestQuickinFetchResolvesAccountMapsFieldsAndPaginates(t *testing.T) {
	// The board slug "botcity" resolves to an opaque account id, which keys the paginated
	// jobs listing. Page 1 declares pages=2, so the walk continues to page 2. The first job
	// exercises every mapping; the unpublished job (publicate != "published") is dropped.
	account := `{"_id":"acc123","name":"BotCity","slug":"botcity"}`

	page1 := `{"total":3,"page":1,"pages":2,"limit":100,"docs":[
		{"_id":"job1","publicate":"published","title":"  Python Developer  ",
		 "description":"<p>Build bots</p><script>evil()</script>","requirements":"<p>Python, 3y</p>",
		 "city":"Global","region":"SP","country":"BR","workplace_type":"remote",
		 "career_url":"https://jobs.quickin.io/botcity/jobs/job1","created_at":"2026-07-03T21:54:21.543Z"},
		{"_id":"job2","publicate":"draft","title":"Hidden Role","description":"<p>secret</p>",
		 "city":"São Paulo","region":"SP","country":"BR","workplace_type":"remote",
		 "created_at":"2026-07-02T10:00:00.000Z"}
	]}`

	page2 := `{"total":3,"page":2,"pages":2,"limit":100,"docs":[
		{"_id":"job3","publicate":"published","title":"Backend Engineer","description":"<p>APIs</p>",
		 "requirements":"","city":"","region":"São Paulo","country":"BR","workplace_type":"hybrid",
		 "career_url":"","created_at":"2026-06-30T12:10:41.833Z"}
	]}`

	fake := (&routedHTTP{}).
		route("/public/accounts/botcity", account).
		route("page=1", page1).
		route("page=2", page2)

	jobs, err := NewQuickin(fake).Fetch(context.Background(), CompanyEntry{
		Company: "BotCity", Provider: "quickin", Board: "botcity",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// 1 published from page 1 (draft dropped) + 1 published from page 2 = 2.
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (one unpublished job dropped across two pages)", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	py, ok := byID["job1"]
	if !ok {
		t.Fatalf("job1 missing; got %v", byID)
	}
	if py.Title != "Python Developer" {
		t.Errorf("Title = %q, want trimmed %q", py.Title, "Python Developer")
	}
	if py.Company != "BotCity" {
		t.Errorf("Company = %q, want %q", py.Company, "BotCity")
	}
	if py.URL != "https://jobs.quickin.io/botcity/jobs/job1" {
		t.Errorf("URL = %q, want the career_url", py.URL)
	}
	if py.Location != "Global, SP, BR" {
		t.Errorf("Location = %q, want %q", py.Location, "Global, SP, BR")
	}
	if !py.Remote || py.WorkMode != "remote" {
		t.Errorf("Remote=%v WorkMode=%q, want remote", py.Remote, py.WorkMode)
	}
	// sanitizeHTML keeps safe structural HTML but strips active content (script).
	if strings.Contains(py.Description, "evil") || strings.Contains(py.Description, "<script") {
		t.Errorf("Description not sanitized: %q", py.Description)
	}
	if !strings.Contains(py.Description, "Build bots") || !strings.Contains(py.Description, "Python, 3y") {
		t.Errorf("Description missing description+requirements: %q", py.Description)
	}
	if py.PostedAt == nil || py.PostedAt.Year() != 2026 {
		t.Errorf("PostedAt = %v, want parsed 2026 date", py.PostedAt)
	}

	be, ok := byID["job3"]
	if !ok {
		t.Fatalf("job3 missing")
	}
	if be.WorkMode != "hybrid" || be.Remote {
		t.Errorf("job3 WorkMode=%q Remote=%v, want hybrid/false", be.WorkMode, be.Remote)
	}
	// Empty career_url falls back to the constructed jobs URL.
	if be.URL != "https://jobs.quickin.io/botcity/jobs/job3" {
		t.Errorf("job3 URL = %q, want constructed fallback", be.URL)
	}
	// region-only location (city empty) skips the blank.
	if be.Location != "São Paulo, BR" {
		t.Errorf("job3 Location = %q, want %q", be.Location, "São Paulo, BR")
	}
}
