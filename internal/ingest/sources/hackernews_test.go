package sources

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// hackernewsSearchBody lists the whoishiring account's newest stories, newest first: the
// sibling threads must be skipped and the two hiring threads found in order.
const hackernewsSearchBody = `{"hits": [
  {"objectID": "49522896", "title": "Ask HN: Who wants to be hired? (September 2026)"},
  {"objectID": "49522897", "title": "Ask HN: Who is hiring? (September 2026)"},
  {"objectID": "49522895", "title": "Ask HN: Freelancer? Seeking freelancer? (September 2026)"},
  {"objectID": "49156683", "title": "Ask HN: Who is hiring? (August 2026)"},
  {"objectID": "48747976", "title": "Ask HN: Who is hiring? (July 2026)"}
]}`

// hackernewsThreadSept is this month's thread: a canonical post with a link and body, a
// deleted comment, a free-form post with no pipe, and a post whose header carries commitment
// and salary segments before its location. Replies under a post are not top-level and must
// not be read.
const hackernewsThreadSept = `{"id": 49522897, "title": "Ask HN: Who is hiring? (September 2026)", "children": [
  {"id": 49522903, "created_at": "2026-09-01T15:01:54.000Z", "author": "a",
   "text": "Modash.io | Senior Product Engineer | Remote (Europe) | Full-time | &#x20AC;75k&#x2013;110k | <a href=\"https:&#x2F;&#x2F;modash.io\" rel=\"nofollow\">https:&#x2F;&#x2F;modash.io</a><p>Modash helps brands find creators.<p>Apply at the link.",
   "children": [{"id": 49522999, "created_at": "2026-09-01T16:00:00.000Z", "text": "Reply | Not a job | Nowhere", "children": []}]},
  {"id": 49522904, "created_at": "2026-09-01T15:02:00.000Z", "author": null, "text": null, "children": []},
  {"id": 49522905, "created_at": "2026-09-01T15:03:00.000Z", "text": "We are hiring engineers, email me.<p>No format.", "children": []},
  {"id": 49522912, "created_at": "2026-09-01T15:02:11.000Z",
   "text": "Quill | Fullstack SWE https:&#x2F;&#x2F;quill.co&#x2F;jobs | Full-time | $150 - 210K USD + equity | Remote, PT&#x2F;ET hours preferred | <a href=\"https:&#x2F;&#x2F;quill.co&#x2F;\">https:&#x2F;&#x2F;quill.co&#x2F;</a><p>Quill is a fullstack SDK.", "children": []}
]}`

// hackernewsThreadAug is last month's thread: one minimal post with no link at all.
const hackernewsThreadAug = `{"id": 49156683, "title": "Ask HN: Who is hiring? (August 2026)", "children": [
  {"id": 49156700, "created_at": "2026-08-03T15:10:00.000Z", "text": "Acme &amp; Co | Backend Engineer", "children": []}
]}`

func hackernewsFake() *routedHTTP {
	return (&routedHTTP{}).
		route("search_by_date", hackernewsSearchBody).
		route("/items/49522897", hackernewsThreadSept).
		route("/items/49156683", hackernewsThreadAug)
}

func TestHackerNewsFetchReadsTheTwoNewestHiringThreads(t *testing.T) {
	fake := hackernewsFake()
	jobs, err := NewHackerNews(fake).Fetch(context.Background(), CompanyEntry{Company: "Hacker News — Who is hiring"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// The search plus the two threads; July's is outside the window.
	if fake.calls != 3 {
		t.Errorf("requests = %d, want 3", fake.calls)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3 (deleted, free-form and reply comments skipped): %+v", len(jobs), jobs)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	modash := byID["49522903"]
	if modash.Company != "Modash.io" || modash.Title != "Senior Product Engineer" || modash.Location != "Remote (Europe)" {
		t.Errorf("modash = %q / %q / %q", modash.Company, modash.Title, modash.Location)
	}
	if modash.URL != "https://modash.io" {
		t.Errorf("modash URL = %q, want the post's first link", modash.URL)
	}
	// WorkMode stays unset: it must carry only a platform-STRUCTURED signal (source.go),
	// never this free-text heuristic, which the pipeline's own location dictionary resolves
	// instead — see TestHackerNewsToJobDoesNotSetWorkModeFromFreeText.
	if !modash.Remote || modash.WorkMode != "" {
		t.Errorf("modash Remote/WorkMode = %v/%q, want true/\"\"", modash.Remote, modash.WorkMode)
	}
	if !strings.Contains(modash.Description, "Modash helps brands find creators.") || !strings.Contains(modash.Description, "Senior Product Engineer") {
		t.Errorf("modash Description = %q, want the whole post", modash.Description)
	}
	if modash.PostedAt == nil || modash.PostedAt.UTC().Format("2006-01-02") != "2026-09-01" {
		t.Errorf("modash PostedAt = %v", modash.PostedAt)
	}

	quill := byID["49522912"]
	if quill.Title != "Fullstack SWE" {
		t.Errorf("quill Title = %q, want the URL stripped from the role", quill.Title)
	}
	if quill.Location != "Remote, PT/ET hours preferred" {
		t.Errorf("quill Location = %q, want the first non-commitment, non-salary segment", quill.Location)
	}
	if quill.URL != "https://quill.co/jobs" && quill.URL != "https://quill.co/" {
		t.Errorf("quill URL = %q", quill.URL)
	}

	acme := byID["49156700"]
	if acme.Company != "Acme & Co" {
		t.Errorf("acme Company = %q, want entities decoded", acme.Company)
	}
	if acme.URL != "https://news.ycombinator.com/item?id=49156700" {
		t.Errorf("acme URL = %q, want the comment permalink fallback", acme.URL)
	}
	if acme.Location != "" || acme.Remote {
		t.Errorf("acme Location/Remote = %q/%v, want none", acme.Location, acme.Remote)
	}
}

// fullCatalog: a thread that could not be read fails the crawl instead of shrinking it.
func TestHackerNewsAThreadFailureFailsTheCrawl(t *testing.T) {
	fake := hackernewsFake().routeErr("/items/49156683", errors.New("boom"))
	_, err := NewHackerNews(fake).Fetch(context.Background(), CompanyEntry{Company: "x"})
	if err == nil || !strings.Contains(err.Error(), "thread 49156683") {
		t.Fatalf("err = %v, want the thread failure", err)
	}
}

func TestHackerNewsNoHiringThreadIsAnError(t *testing.T) {
	fake := (&routedHTTP{}).route("search_by_date", `{"hits": [{"objectID": "1", "title": "Ask HN: Who wants to be hired? (September 2026)"}]}`)
	if _, err := NewHackerNews(fake).Fetch(context.Background(), CompanyEntry{Company: "x"}); err == nil {
		t.Fatal("Fetch succeeded with no hiring thread")
	}
}

func TestHackerNewsParseHeader(t *testing.T) {
	cases := []struct {
		text string
		want hackernewsHeader
		ok   bool
	}{
		{"Acme | Engineer | NYC | ONSITE<p>body", hackernewsHeader{Company: "Acme", Title: "Engineer", Location: "NYC"}, true},
		{"Acme | Engineer | Contract | Remote<p>body", hackernewsHeader{Company: "Acme", Title: "Engineer", Location: "Remote", Remote: true}, true},
		{"Acme | Engineer | $200k | London", hackernewsHeader{Company: "Acme", Title: "Engineer", Location: "London"}, true},
		// € and £ are multi-byte UTF-8; a salary segment leading with either must be skipped
		// the same way a $ one is, not mistaken for the location.
		{"Acme | Engineer | €95k-120k | Berlin", hackernewsHeader{Company: "Acme", Title: "Engineer", Location: "Berlin"}, true},
		{"Acme | Engineer | £80k-100k | London", hackernewsHeader{Company: "Acme", Title: "Engineer", Location: "London"}, true},
		{"Acme | https://acme.example/jobs", hackernewsHeader{}, false},
		{"Just a sentence with no pipes<p>more", hackernewsHeader{}, false},
		{" | Engineer", hackernewsHeader{}, false},
		// A URL immediately followed by closing punctuation, no space in between: the match
		// must stop before the paren, not swallow it into the stripped segment.
		{"Acme | Fullstack SWE (https://acme.example/jobs) | NYC", hackernewsHeader{Company: "Acme", Title: "Fullstack SWE ()", Location: "NYC"}, true},
		// A header whose employer segment is itself a bare URL names no real employer —
		// dropped the same way an all-URL title already is.
		{"https://acme.example | Senior Engineer | NYC", hackernewsHeader{}, false},
		// A post that opens with the role, not the employer: every segment is shifted by one,
		// so filing it would write "Senior Software Engineer, Frontend" as the company and
		// "New York, NY (In-Office)" as the role. Observed live; dropped, not corrected.
		{"Senior Software Engineer, Frontend | New York, NY (In-Office) | Full-time", hackernewsHeader{}, false},
		{"Founding Engineer | Remote | Equity", hackernewsHeader{}, false},
		{"Head of Data Engineering | Berlin", hackernewsHeader{}, false},
		// The seniority word alone is not evidence — these are real employers, and dropping
		// them is the false positive the paired role noun exists to prevent.
		{"Lead Bank | Engineer | Kansas City", hackernewsHeader{Company: "Lead Bank", Title: "Engineer", Location: "Kansas City"}, true},
		{"Chief Industries | Welder | Nebraska", hackernewsHeader{Company: "Chief Industries", Title: "Welder", Location: "Nebraska"}, true},
	}
	for _, c := range cases {
		got, ok := hackernewsParseHeader(c.text)
		if ok != c.ok || got != c.want {
			t.Errorf("hackernewsParseHeader(%q) = %+v, %v; want %+v, %v", c.text, got, ok, c.want, c.ok)
		}
	}
}

func TestHackerNewsToJobDoesNotSetWorkModeFromFreeText(t *testing.T) {
	// "Remote" in the Title, not the Location — Job.WorkMode must stay unset so the
	// pipeline's own location/description dictionary resolves it, per source.go's
	// documented contract that WorkMode carries only a platform-STRUCTURED signal.
	// A set WorkMode takes precedence over that dictionary, so this Title match would
	// otherwise wrongly override a genuinely onsite Location.
	c := hackernewsItem{
		ID:        1,
		CreatedAt: "2026-09-01T00:00:00Z",
		Text:      "Acme | Remote Systems Engineer | Full-time | Austin, TX",
	}
	job, ok := c.toJob()
	if !ok {
		t.Fatal("toJob() = false, want a job")
	}
	if job.WorkMode != "" {
		t.Errorf("WorkMode = %q, want empty", job.WorkMode)
	}
}

func TestHackerNewsMarkers(t *testing.T) {
	src := NewHackerNews(nil)
	if src.Provider() != "hackernews" {
		t.Errorf("Provider() = %q", src.Provider())
	}
	if _, ok := src.(boardless); !ok {
		t.Error("hackernews is not boardless")
	}
	if _, ok := src.(aggregator); !ok {
		t.Error("hackernews is not an aggregator")
	}
	if _, ok := src.(fullCatalog); !ok {
		t.Error("hackernews is not a fullCatalog; a post from an aged-out thread would never close")
	}
}
