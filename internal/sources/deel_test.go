package sources

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// deelPage wraps a raw flight stream into a minimal board page: one
// self.__next_f.push([1,"<json-quoted flight>"]) script, the shape decodeNextFlight reads.
func deelPage(flight string) string {
	q, _ := json.Marshal(flight)
	return `<html><body><script>self.__next_f.push([1,` + string(q) + `])</script></body></html>`
}

func deelFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func deelParse(t *testing.T, name string) *html.Node {
	t.Helper()
	root, err := html.Parse(strings.NewReader(deelFixture(t, name)))
	if err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return root
}

func TestDeelProvider(t *testing.T) {
	if got := NewDeel(nil).Provider(); got != "deel" {
		t.Errorf("Provider() = %q, want %q", got, "deel")
	}
}

func TestDeelIsBoardBased(t *testing.T) {
	if _, ok := NewDeel(nil).(boardless); ok {
		t.Error("deel adapter must be board-based (per-tenant slug), not boardless")
	}
}

// TestDeelDecodeFlight pins that the embedded Next.js flight chunks concatenate and
// decode into one stream carrying the postings payload, with multibyte text decoded as
// correct UTF-8 (no mojibake).
func TestDeelDecodeFlight(t *testing.T) {
	flight, err := decodeNextFlight(deelParse(t, "deel_klarna.html"))
	if err != nil {
		t.Fatalf("decodeNextFlight: %v", err)
	}
	if !strings.Contains(flight, `"jobPostings":`) {
		t.Error("flight stream missing jobPostings payload")
	}
	if !strings.Contains(flight, "world’s favorite") {
		t.Error("flight stream lost clean UTF-8 (expected the curly apostrophe in \"world’s favorite\")")
	}
}

// TestDeelTextRows pins reference resolution: a posting's "$N" reference resolves to its
// length-delimited text row's HTML.
func TestDeelTextRows(t *testing.T) {
	flight, err := decodeNextFlight(deelParse(t, "deel_klarna.html"))
	if err != nil {
		t.Fatalf("decodeNextFlight: %v", err)
	}
	rows := nextFlightTextRows(flight)
	html23, ok := rows["23"]
	if !ok {
		t.Fatal("text row 23 not resolved")
	}
	if !strings.HasPrefix(html23, "<p>") || !strings.Contains(html23, "world’s favorite") {
		t.Errorf("row 23 HTML unexpected: %.60q…", html23)
	}
	// RSC row ids are hex: a "$2a" reference must resolve as readily as a "$23" one (a
	// decimal-only matcher silently drops every a–f row — ~40% of a real board).
	if html2a, ok := rows["2a"]; !ok || strings.TrimSpace(html2a) == "" {
		t.Error("hex text row 2a not resolved (decimal-only row matcher regression)")
	}
}

// TestDeelFetch pins the single-request crawl: one GET of the board page yields every
// posting from the embedded payload, with no per-posting detail request.
func TestDeelFetch(t *testing.T) {
	fake := &fakeHTTP{body: deelFixture(t, "deel_klarna.html")}
	jobs, err := NewDeel(fake).Fetch(context.Background(), CompanyEntry{Company: "Klarna ACME", Board: "klarna"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if want := "https://jobs.deel.com/klarna"; fake.gotURL != want {
		t.Errorf("requested %q, want %q", fake.gotURL, want)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	// Both postings (a "$23" and a hex "$2a" reference) must carry a resolved description.
	for i, j := range jobs {
		if strings.TrimSpace(j.Description) == "" {
			t.Errorf("job %d (%s) has an empty description; its reference did not resolve", i, j.ExternalID)
		}
	}
}

// TestDeelFieldMapping pins the mapping of one real posting from the embedded payload.
func TestDeelFieldMapping(t *testing.T) {
	fake := &fakeHTTP{body: deelFixture(t, "deel_klarna.html")}
	jobs, err := NewDeel(fake).Fetch(context.Background(), CompanyEntry{Company: "Configured Co", Board: "klarna"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	j := jobs[0]
	const id = "5d3636f8-0712-4fe2-a1f4-84358440d272"
	if j.ExternalID != id {
		t.Errorf("ExternalID = %q, want the posting id %q", j.ExternalID, id)
	}
	if want := "https://jobs.deel.com/klarna/job-details/" + id + "/overview"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if want := "Customer Trust and Experience Consultant (Fully Remote - Sweden)"; j.Title != want {
		t.Errorf("Title = %q, want %q", j.Title, want)
	}
	if j.Company != "Klarna" { // careerPageSettings.preferredOrganizationName wins over e.Company
		t.Errorf("Company = %q, want %q (from careerPageSettings)", j.Company, "Klarna")
	}
	if j.Location != "Sweden" {
		t.Errorf("Location = %q, want %q", j.Location, "Sweden")
	}
	if !strings.Contains(j.Description, "world’s favorite") {
		t.Errorf("Description missing resolved body; got %.80q…", j.Description)
	}
	if strings.Contains(j.Description, "<script") {
		t.Error("Description not sanitized")
	}
	if !j.Remote { // "Fully Remote" in the title → isRemote
		t.Error("Remote = false, want true for a 'Fully Remote' posting")
	}
}

// TestDeelPostedAt pins that createdAt maps to PostedAt via RFC3339.
func TestDeelPostedAt(t *testing.T) {
	fake := &fakeHTTP{body: deelFixture(t, "deel_klarna.html")}
	jobs, _ := NewDeel(fake).Fetch(context.Background(), CompanyEntry{Board: "klarna"})
	got := jobs[0].PostedAt
	if got == nil {
		t.Fatal("PostedAt = nil, want the createdAt date")
	}
	if want := time.Date(2025, 11, 4, 0, 0, 0, 0, time.UTC); !got.Truncate(24 * time.Hour).Equal(want) {
		t.Errorf("PostedAt day = %v, want %v", got.Truncate(24*time.Hour), want)
	}
}

// TestDeelCompanyFallback pins that an absent careerPageSettings name falls back to the
// configured company name.
func TestDeelCompanyFallback(t *testing.T) {
	p := deelPosting{ID: "abc", Title: "Backend Engineer"}
	j, ok := deel{}.toJob(CompanyEntry{Company: "Acme", Board: "acme"}, "", nil, p)
	if !ok {
		t.Fatal("toJob returned ok=false for a valid posting")
	}
	if j.Company != "Acme" {
		t.Errorf("Company = %q, want fallback %q", j.Company, "Acme")
	}
}

// TestDeelRemoteHeuristic pins that the remote flag comes from the shared isRemote
// heuristic over the title and location (Deel exposes no structured workplace field).
func TestDeelRemoteHeuristic(t *testing.T) {
	remote, _ := deel{}.toJob(CompanyEntry{Board: "b"}, "Org", nil, deelPosting{ID: "1", Title: "Remote Data Engineer"})
	if !remote.Remote {
		t.Error("Remote = false for a 'Remote' title, want true")
	}
	onsite, _ := deel{}.toJob(CompanyEntry{Board: "b"}, "Org", nil, deelPosting{ID: "2", Title: "Office Manager"})
	if onsite.Remote {
		t.Error("Remote = true for a non-remote title, want false")
	}
}

func TestDeelEmptyBoardYieldsNoJobs(t *testing.T) {
	fake := &fakeHTTP{body: deelFixture(t, "deel_empty.html")}
	jobs, err := NewDeel(fake).Fetch(context.Background(), CompanyEntry{Board: "klarna"})
	if err != nil {
		t.Fatalf("Fetch on empty board: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("got %d jobs, want 0 for an empty board", len(jobs))
	}
}

func TestDeelNoPayloadIsError(t *testing.T) {
	// A page that has a flight chunk but no jobPostings payload must error loudly rather
	// than silently yield an empty catalogue.
	body := `<html><body><script>self.__next_f.push([1,"7:[\"$\",\"div\",null,{}]\n"])</script></body></html>`
	fake := &fakeHTTP{body: body}
	if _, err := NewDeel(fake).Fetch(context.Background(), CompanyEntry{Board: "klarna"}); err == nil {
		t.Error("Fetch with no jobPostings payload returned nil error, want an error")
	}
}

func TestDeelDropsEmptyID(t *testing.T) {
	if _, ok := (deel{}).toJob(CompanyEntry{Board: "x"}, "Org", nil, deelPosting{ID: "", Title: "x"}); ok {
		t.Error("toJob yielded a posting with empty id (would collide on the dedup key)")
	}
}

// TestDeelTitleWithBrackets pins that a tenant-controlled bracket inside a posting value
// (e.g. "[EMEA] …") does not unbalance the jobPostings array scan.
func TestDeelTitleWithBrackets(t *testing.T) {
	flight := `8:["$","$L1f",null,{"careerPageSettings":{"preferredOrganizationName":"Acme"},` +
		`"jobPostings":[{"id":"1","title":"[EMEA] Senior Engineer [remote]","richtextDescription":"$a"}]}]` +
		"\n" + `a:T9,<p>Hi</p>`
	fake := &fakeHTTP{body: deelPage(flight)}
	jobs, err := NewDeel(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (bracket in title unbalanced the scan)", len(jobs))
	}
	if jobs[0].Title != "[EMEA] Senior Engineer [remote]" {
		t.Errorf("Title = %q, want the bracketed title", jobs[0].Title)
	}
}

// TestDeelUnresolvedRefsError pins the loud-failure guard: postings that reference text
// rows none of which resolve mean the row parse broke, so the board errors rather than
// shipping every job with an empty description.
func TestDeelUnresolvedRefsError(t *testing.T) {
	flight := `8:["$","$L1f",null,{"careerPageSettings":{"preferredOrganizationName":"Acme"},` +
		`"jobPostings":[{"id":"1","title":"X","richtextDescription":"$ff"}]}]`
	fake := &fakeHTTP{body: deelPage(flight)}
	if _, err := NewDeel(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Error("Fetch with no resolvable description references returned nil error, want an error")
	}
}
