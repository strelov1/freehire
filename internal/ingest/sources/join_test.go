package sources

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

// joinListJSON builds a list-API response with the given pageCount and job items.
// Each item is "id|idParam|title|createdAt|workplaceType|cityName|countryName".
func joinListJSON(pageCount int, items ...[7]string) string {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i, it := range items {
		if i > 0 {
			b.WriteString(",")
		}
		city := "null"
		if it[5] != "" || it[6] != "" {
			city = `{"cityName":"` + it[5] + `","countryName":"` + it[6] + `"}`
		}
		b.WriteString(`{"id":` + it[0] + `,"idParam":"` + it[1] + `","title":"` + it[2] +
			`","createdAt":"` + it[3] + `","workplaceType":"` + it[4] + `","city":` + city + `}`)
	}
	b.WriteString(`],"pagination":{"rowCount":` + strconv.Itoa(pageCount*len(items)) + `,"pageCount":` + strconv.Itoa(pageCount) + `}}`)
	return b.String()
}

func joinDetailJSON(domain, markdownDescription string) string {
	return `{"description":"` + markdownDescription + `","company":{"domain":"` + domain + `"}}`
}

func TestJoinProvider(t *testing.T) {
	if got := NewJoin(nil).Provider(); got != "join" {
		t.Errorf("Provider() = %q, want %q", got, "join")
	}
}

func TestJoinFetchListThenDetailAndMaps(t *testing.T) {
	list := joinListJSON(1, [7]string{
		"16291944", "16291944-cro-manager", "CRO Manager", "2026-06-11T11:51:32.030Z",
		"OFFICE", "Berlin", "Germany",
	})
	detail := joinDetailJSON("justhiringde", "Intro line.\\n\\n*   Own the funnel\\n*   Ship fast")
	fake := (&routedHTTP{}).
		route("/companies/167291/jobs", list).
		route("/jobs/16291944", detail)

	jobs, err := NewJoin(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Just Hiring", Provider: "join", Board: "167291",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "16291944" {
		t.Errorf("ExternalID = %q, want 16291944", j.ExternalID)
	}
	if j.URL != "https://join.com/companies/justhiringde/16291944-cro-manager" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "CRO Manager" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Just Hiring" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Berlin, Germany" {
		t.Errorf("Location = %q, want %q", j.Location, "Berlin, Germany")
	}
	if strings.Contains(j.Description, "*   ") ||
		!strings.Contains(j.Description, "<li>Own the funnel</li>") || !strings.Contains(j.Description, "<p>") {
		t.Errorf("Description not markdown-rendered/sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 11, 11, 51, 32, 30_000_000, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-11T11:51:32.030Z", j.PostedAt)
	}
}

func TestJoinPaginatesAllPages(t *testing.T) {
	d := joinDetailJSON("acme", "Body.")
	p1 := joinListJSON(2, [7]string{"1", "1-a", "A", "2026-06-01T00:00:00Z", "REMOTE", "", ""})
	p2 := joinListJSON(2, [7]string{"2", "2-b", "B", "2026-06-02T00:00:00Z", "REMOTE", "", ""})
	fake := (&routedHTTP{}).
		route("jobs?page=1", p1).
		route("jobs?page=2", p2).
		route("/jobs/1", d).route("/jobs/2", d)

	jobs, err := NewJoin(fake).Fetch(context.Background(), CompanyEntry{Board: "9"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (both pages)", len(jobs))
	}
}

func TestJoinSinglePageIssuesOneListRequest(t *testing.T) {
	d := joinDetailJSON("acme", "Body.")
	// Only page 1 is routed. If the adapter requested page 2, the routed fake would error and
	// the board would fail — so a green single-job result proves exactly one list request.
	fake := (&routedHTTP{}).
		route("jobs?page=1", joinListJSON(1, [7]string{"5", "5-x", "X", "2026-06-01T00:00:00Z", "REMOTE", "", ""})).
		route("/jobs/5", d)
	jobs, err := NewJoin(fake).Fetch(context.Background(), CompanyEntry{Board: "9"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
}

func TestJoinRemoteFromWorkplaceType(t *testing.T) {
	d := joinDetailJSON("acme", "Body.")
	fake := (&routedHTTP{}).
		route("/companies/9/jobs", joinListJSON(1, [7]string{"7", "7-r", "Engineer", "2026-06-01T00:00:00Z", "REMOTE", "", ""})).
		route("/jobs/7", d)
	jobs, err := NewJoin(fake).Fetch(context.Background(), CompanyEntry{Board: "9"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].Remote {
		t.Fatalf("want one remote job, got %+v", jobs)
	}
}

func TestJoinRemoteIgnoresTitleUsesWorkplaceAndLocation(t *testing.T) {
	d := joinDetailJSON("acme", "Body.")
	// workplaceType OFFICE + "Remote" only in the title must NOT flag remote (the title is
	// not a remote signal); the authoritative workplaceType and the location are.
	fake := (&routedHTTP{}).
		route("/companies/9/jobs", joinListJSON(1,
			[7]string{"1", "1-a", "Remote Sensing Engineer", "2026-06-01T00:00:00Z", "OFFICE", "Berlin", "Germany"})).
		route("/jobs/1", d)
	jobs, err := NewJoin(fake).Fetch(context.Background(), CompanyEntry{Board: "9"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Remote {
		t.Fatalf("title-only 'Remote' must not flag remote, got %+v", jobs)
	}
}

func TestJoinFailedDetailPreservesListedPosting(t *testing.T) {
	d := joinDetailJSON("acme", "Body.")
	// No route for /jobs/222: a transient detail failure must preserve its listed identity.
	fake := (&routedHTTP{}).
		route("/companies/9/jobs", joinListJSON(1,
			[7]string{"111", "111-kept", "Kept", "2026-06-01T00:00:00Z", "REMOTE", "", ""},
			[7]string{"222", "222-drop", "Dropped", "2026-06-01T00:00:00Z", "REMOTE", "", ""})).
		route("/jobs/111", d)
	jobs, err := NewJoin(fake).Fetch(context.Background(), CompanyEntry{Board: "9"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %v, want both listed postings", jobs)
	}
	for _, job := range jobs {
		if job.ExternalID == "222" && !job.Unreadable {
			t.Fatal("failed detail lost its unreadable marker")
		}
	}
}

func TestJoinEmptyBoardYieldsNoJobsNoError(t *testing.T) {
	fake := (&routedHTTP{}).route("/companies/9/jobs", `{"items":[],"pagination":{"pageCount":0,"rowCount":0}}`)
	jobs, err := NewJoin(fake).Fetch(context.Background(), CompanyEntry{Board: "9"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestJoinRegisteredInAll(t *testing.T) {
	s, ok := All(nil)["join"]
	if !ok {
		t.Fatal("All() missing provider join")
	}
	if s.Provider() != "join" {
		t.Errorf("All()[join].Provider() = %q", s.Provider())
	}
}

func TestMarkdownToHTML(t *testing.T) {
	got := markdownToHTML("Intro paragraph.\n\n*   First\n*   Second")
	if !strings.Contains(got, "<p>Intro paragraph.</p>") {
		t.Errorf("paragraph not rendered: %q", got)
	}
	if !strings.Contains(got, "<ul>") || !strings.Contains(got, "<li>First</li>") ||
		!strings.Contains(got, "<li>Second</li>") {
		t.Errorf("list not rendered: %q", got)
	}
	if s := strings.TrimSpace(markdownToHTML("")); s != "" {
		t.Errorf("empty input should yield empty output, got %q", s)
	}
}

func TestJoinRejectsIncompleteListings(t *testing.T) {
	for _, body := range []string{`{}`, `{"items":null,"pagination":{"pageCount":0,"rowCount":0}}`, `{"items":[],"pagination":{"pageCount":1,"rowCount":1}}`, `{"items":[],"pagination":{"pageCount":0}}`} {
		t.Run(body, func(t *testing.T) {
			_, err := NewJoin((&routedHTTP{}).route("/companies/9/jobs", body)).Fetch(context.Background(), CompanyEntry{Board: "9"})
			if err == nil {
				t.Fatal("malformed or incomplete listing succeeded")
			}
		})
	}
}
func TestJoinListingMarkers(t *testing.T) {
	s := NewJoin(nil)
	if _, ok := s.(fullBoardListing); !ok {
		t.Fatal("missing full listing contract")
	}
	if _, ok := s.(emptyBoardListing); !ok {
		t.Fatal("missing explicit empty contract")
	}
}

func TestJoinIncompletePaginationFailsWholeBoard(t *testing.T) {
	item := [7]string{"1", "1-a", "Engineer", "", "REMOTE", "", ""}
	for name, fake := range map[string]*routedHTTP{
		"late failure":  (&routedHTTP{}).route("jobs?page=1", joinListJSON(2, item)),
		"repeated page": (&routedHTTP{}).route("jobs?page=1", joinListJSON(2, item)).route("jobs?page=2", joinListJSON(2, item)),
		"short count":   (&routedHTTP{}).route("jobs?page=1", strings.Replace(joinListJSON(1, item), `"rowCount":1`, `"rowCount":2`, 1)),
		"changed count": (&routedHTTP{}).route("jobs?page=1", joinListJSON(2, item)).route("jobs?page=2", joinListJSON(1, [7]string{"2", "2-b", "Other", "", "REMOTE", "", ""})),
	} {
		t.Run(name, func(t *testing.T) {
			jobs, err := NewJoin(fake).Fetch(context.Background(), CompanyEntry{Board: "9"})
			if err == nil || len(jobs) != 0 {
				t.Fatalf("partial listing returned: %v, %v", jobs, err)
			}
		})
	}
}

func TestJoinDetailGoneAndUnreadableAreDistinct(t *testing.T) {
	for _, code := range []int{404, 410, 500} {
		fake := (&routedHTTP{}).route("/companies/9/jobs", joinListJSON(1, [7]string{"1", "1-a", "Engineer", "", "REMOTE", "", ""})).routeErr("/jobs/1", &StatusError{Code: code})
		jobs, err := NewJoin(fake).Fetch(context.Background(), CompanyEntry{Board: "9"})
		if err != nil {
			t.Fatal(err)
		}
		if code == 500 {
			if len(jobs) != 1 || !jobs[0].Unreadable {
				t.Fatalf("transient detail failure lost listed identity: %v", jobs)
			}
		} else if len(jobs) != 0 {
			t.Fatalf("gone detail retained: %v", jobs)
		}
	}
}
