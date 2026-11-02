package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// paradoxFake serves the real JovieofNewportBeach site captured 2026-10-20: its one-page
// listing (3 postings) and each posting's detail resource, as served. Anything else is the
// platform's real answer for a job_id the site does not know.
type paradoxFake struct {
	mu sync.Mutex
	// listing overrides the page served for a page number; nil serves the fixture on page 1
	// and the platform's real past-the-end answer after it.
	listing func(page int) (map[string]any, error)
	// detail overrides the fixture file served for a job_id ("" = the fixture of that id).
	detail map[string]string
	// mutateDetail edits a detail body before it is served.
	mutateDetail func(body map[string]any)
	asked        []string
	missingXHR   bool
	// refuse answers every detail request with the 403 a spent request budget gets.
	refuse bool
}

func (f *paradoxFake) GetJSONWithHeaders(_ context.Context, raw string, headers map[string]string, v any) error {
	f.mu.Lock()
	f.asked = append(f.asked, raw)
	if headers["X-Requested-With"] != "XMLHttpRequest" {
		f.missingXHR = true
	}
	f.mu.Unlock()
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	var body map[string]any
	switch u.Path {
	case "/api/job-list-site-ui/search-job":
		page, _ := strconv.Atoi(u.Query().Get("page"))
		if f.listing != nil {
			if body, err = f.listing(page); err != nil {
				return err
			}
			break
		}
		if page != 1 {
			body = paradoxPage(nil, 0)
			break
		}
		body = paradoxFixture("listing.json")
	case "/api/job-posting/get_widget_extra_settings":
		if f.refuse {
			return &StatusError{Method: "GET", Code: 403, URL: raw}
		}
		id := u.Query().Get("job_id")
		file := "detail_" + id + ".json"
		if o, ok := f.detail[id]; ok {
			file = o
		}
		if _, err := os.Stat("testdata/paradox/" + file); err != nil {
			file = "detail_unknown.json"
		}
		body = paradoxFixture(file)
		if f.mutateDetail != nil {
			f.mutateDetail(body)
		}
	default:
		return fmt.Errorf("HTTP 404 for %s", raw)
	}
	out, _ := json.Marshal(body)
	return json.Unmarshal(out, v)
}

func paradoxFixture(name string) map[string]any {
	b, err := os.ReadFile("testdata/paradox/" + name)
	if err != nil {
		panic(err)
	}
	var body map[string]any
	if err := json.Unmarshal(b, &body); err != nil {
		panic(err)
	}
	return body
}

// paradoxPage builds a listing page holding rows and stating total, in the platform's shape.
func paradoxPage(rows []any, total int) map[string]any {
	if rows == nil {
		rows = []any{}
	}
	return map[string]any{"jobs": rows, "total_jobs": total, "success": true}
}

// paradoxRow builds a listing row for posting id on the Jovie site.
func paradoxRow(id string) map[string]any {
	return map[string]any{
		"job_title":          "Nanny " + id,
		"original_apply_url": "https://olivia.paradox.ai/co/JovieofNewportBeach/Job?job_id=" + id,
		"location_name":      []any{"Long Beach CA 90806, United States"},
		"employment_types":   "",
	}
}

const paradoxBoardID = "olivia.paradox.ai/JovieofNewportBeach"

func paradoxFetch(f *paradoxFake) ([]Job, error) {
	return NewParadox(f).Fetch(context.Background(), CompanyEntry{Company: "Jovie of Newport Beach", Board: paradoxBoardID})
}

// A slow detail request must not spend the time the pipeline needs to persist the
// board. Without a hydration reserve, FetchNew returns after its caller expires
// and even successfully fetched postings cannot be written.
func TestParadoxLeavesTimeForPersistence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	f := &paradoxDeadlineGetter{paradoxFake: &paradoxFake{}}
	jobs, err := NewParadox(f).(HydratingSource).FetchNew(ctx,
		CompanyEntry{Company: "Acme", Board: paradoxBoardID},
		func(id string) bool { return id == "P1-6681374-1" })
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("FetchNew exhausted the persistence context: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d postings, want one seen refresh and two unreadable markers", len(jobs))
	}
	for _, j := range jobs {
		if j.ExternalID == "P1-6681374-1" {
			if !j.SeenRefresh {
				t.Error("a seen posting must still refresh without a detail request")
			}
		} else if !j.Unreadable {
			t.Errorf("deferred posting %q must remain visible to the sweep guard", j.ExternalID)
		}
	}
	if f.details != 0 {
		t.Errorf("sent %d detail requests with no hydration budget remaining", f.details)
	}
}

type paradoxDeadlineGetter struct {
	*paradoxFake
	details int
	readyID string
}

func (f *paradoxDeadlineGetter) GetJSONWithHeaders(ctx context.Context, raw string, headers map[string]string, v any) error {
	if strings.Contains(raw, "/api/job-posting/") {
		f.mu.Lock()
		f.details++
		f.mu.Unlock()
		u, _ := url.Parse(raw)
		if u.Query().Get("job_id") == f.readyID {
			return f.paradoxFake.GetJSONWithHeaders(ctx, raw, headers, v)
		}
		// Bound the regression itself if the adapter stops reserving time.
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > time.Second {
			return errors.New("detail request has no bounded hydration deadline")
		}
		<-ctx.Done()
		return ctx.Err()
	}
	return f.paradoxFake.GetJSONWithHeaders(ctx, raw, headers, v)
}

func TestParadoxReturnsCompletedDetailsBeforePersistenceDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute+200*time.Millisecond)
	defer cancel()
	f := &paradoxDeadlineGetter{paradoxFake: &paradoxFake{}, readyID: "P1-6616376-1"}
	jobs, err := NewParadox(f).Fetch(ctx, CompanyEntry{Company: "Acme", Board: paradoxBoardID})
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("the pipeline cannot persist the completed detail: %v", err)
	}
	var completed, unreadable int
	for _, j := range jobs {
		if j.Unreadable {
			unreadable++
		} else if j.ExternalID == "P1-6616376-1" && j.Description != "" {
			completed++
		}
	}
	if completed != 1 || unreadable != 2 {
		t.Fatalf("got %d completed details and %d unreadable markers, want 1 and 2", completed, unreadable)
	}
}

func TestParadoxRetainsPostingsWhenPacerRefusesBeforeDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	// The listing consumes the only token. The next token is beyond even the
	// parent deadline, so Wait refuses while both contexts are still alive.
	f := &paradoxFake{}
	s := NewParadox(rateLimitedHeaderJSONGetter{
		inner:   f,
		limiter: rate.NewLimiter(rate.Every(10*time.Minute), 1),
	}).(HydratingSource)
	jobs, err := s.FetchNew(ctx, CompanyEntry{Company: "Acme", Board: paradoxBoardID},
		func(id string) bool { return id == "P1-6681374-1" })
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("the persistence context must remain live: %v", err)
	}
	var refreshed, unreadable int
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshed++
		}
		if j.Unreadable {
			unreadable++
		}
	}
	if refreshed != 1 || unreadable != 2 {
		t.Fatalf("got %d refreshes and %d unreadable markers, want 1 and 2", refreshed, unreadable)
	}
}

func TestParadoxFetch(t *testing.T) {
	f := &paradoxFake{}
	jobs, err := paradoxFetch(f)
	if err != nil {
		t.Fatal(err)
	}
	if f.missingXHR {
		t.Error("a request went out without X-Requested-With; the API answers 404 to it")
	}
	if !strings.Contains(f.asked[0], "compsite_slug=jovieofnewportbeach") || !strings.Contains(f.asked[0], "limit=100") {
		t.Errorf("listing request %q: want the lower-cased site and a 100-row page", f.asked[0])
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want the site's 3", len(jobs))
	}
	var j Job
	for _, x := range jobs {
		if x.ExternalID == "P1-6376329-2" {
			j = x
		}
	}
	if j.ExternalID == "" {
		t.Fatalf("posting P1-6376329-2 missing: %+v", jobs)
	}
	if j.URL != "https://olivia.paradox.ai/co/JovieofNewportBeach/Job?job_id=P1-6376329-2" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "On-Call Nanny in OC/LA (ALL AGES)" || j.Company != "Jovie of Newport Beach" {
		t.Errorf("title/company = %q / %q", j.Title, j.Company)
	}
	if j.Location != "Long Beach CA 90806, United States" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-09-23 from the JobPosting's \"26-09-23\"", j.PostedAt)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "us" {
		t.Errorf("Countries = %v, want [us] from addressCountry", j.Countries)
	}
	// The body is the detail's full HTML, not the listing's 250-character preview, and it is
	// sanitized: the editor's contenteditable attributes do not survive.
	if len(j.Description) < 1000 || !strings.Contains(j.Description, "<") {
		t.Errorf("Description is not the full HTML body (%d bytes): %.200q", len(j.Description), j.Description)
	}
	if strings.Contains(j.Description, "contenteditable") || strings.Contains(j.Description, "<script") {
		t.Errorf("Description is not sanitized: %.300q", j.Description)
	}
	if j.SeenRefresh || j.Unreadable {
		t.Errorf("a hydrated posting must be a full job: %+v", j)
	}
}

func TestParadoxFetchNewSkipsSeenDetails(t *testing.T) {
	f := &paradoxFake{}
	seen := func(id string) bool { return id != "P1-6616376-1" }
	jobs, err := NewParadox(f).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Jovie of Newport Beach", Board: paradoxBoardID}, seen)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}
	refreshed := 0
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshed++
			if j.URL == "" || j.Title == "" || j.Description != "" {
				t.Errorf("a seen posting must carry identity only: %+v", j)
			}
		}
	}
	if refreshed != 2 {
		t.Errorf("got %d liveness refreshes, want 2", refreshed)
	}
	details := 0
	for _, a := range f.asked {
		if strings.Contains(a, "get_widget_extra_settings") {
			details++
		}
	}
	if details != 1 {
		t.Errorf("made %d detail requests, want 1 — a seen posting costs none", details)
	}
}

// A job_id the site no longer knows answers 200 with no body; the posting is deferred, not
// stored with the listing's preview.
func TestParadoxDefersABodylessDetail(t *testing.T) {
	f := &paradoxFake{detail: map[string]string{"P1-6681374-1": "detail_unknown.json"}}
	jobs, err := paradoxFetch(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 with the bodyless one deferred", len(jobs))
	}
	for _, j := range jobs {
		if j.ExternalID == "P1-6681374-1" {
			t.Errorf("the bodyless posting was yielded: %+v", j)
		}
	}
}

// Sites that embed no JobPosting state no date and no structured country; the posting is
// still complete.
func TestParadoxWithoutJobPosting(t *testing.T) {
	f := &paradoxFake{mutateDetail: func(b map[string]any) { b["google_json_ld"] = "" }}
	jobs, err := paradoxFetch(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}
	for _, j := range jobs {
		if j.PostedAt != nil || j.Countries != nil || j.Description == "" {
			t.Errorf("want a body and no date/country without a JobPosting: %+v", j)
		}
	}
}

func TestParadoxPagesToTheEnd(t *testing.T) {
	const total = paradoxPageSize + 5
	rows := make([]any, total)
	for i := range rows {
		rows[i] = paradoxRow(fmt.Sprintf("P1-%07d-1", i))
	}
	f := &paradoxFake{listing: func(page int) (map[string]any, error) {
		switch page {
		case 1:
			return paradoxPage(rows[:paradoxPageSize], total), nil
		case 2:
			return paradoxPage(rows[paradoxPageSize:], total), nil
		}
		return nil, fmt.Errorf("page %d asked for past a short page", page)
	}}
	// Every posting is already held, so the test reads the listing walk alone.
	jobs, err := NewParadox(f).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Jovie of Newport Beach", Board: paradoxBoardID}, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != total {
		t.Errorf("got %d jobs, want %d", len(jobs), total)
	}
}

// fullBoardListing rests on these: a later page failing, a count short of the stated total,
// and a page still full at the safety ceiling all fail the crawl instead of yielding a
// truncated board.
func TestParadoxRefusesATruncatedListing(t *testing.T) {
	full := make([]any, paradoxPageSize)
	for i := range full {
		full[i] = paradoxRow(fmt.Sprintf("P1-%07d-1", i))
	}
	cases := map[string]func(page int) (map[string]any, error){
		"later page fails": func(page int) (map[string]any, error) {
			if page == 1 {
				return paradoxPage(full, 150), nil
			}
			return nil, errors.New("HTTP 502")
		},
		"fewer than the total": func(int) (map[string]any, error) {
			return paradoxPage([]any{paradoxRow("P1-1-1")}, 2), nil
		},
		"success false": func(int) (map[string]any, error) {
			p := paradoxPage([]any{paradoxRow("P1-1-1")}, 1)
			p["success"] = false
			return p, nil
		},
		"never ends": func(page int) (map[string]any, error) {
			rows := make([]any, paradoxPageSize)
			for i := range rows {
				rows[i] = paradoxRow(fmt.Sprintf("P%d-%d", page, i))
			}
			return paradoxPage(rows, 1<<30), nil
		},
		"row on another site": func(int) (map[string]any, error) {
			r := paradoxRow("P1-1-1")
			r["original_apply_url"] = "https://olivia.paradox.ai/co/SomeoneElse/Job?job_id=P1-1-1"
			return paradoxPage([]any{r}, 1), nil
		},
	}
	for name, listing := range cases {
		t.Run(name, func(t *testing.T) {
			if jobs, err := paradoxFetch(&paradoxFake{listing: listing}); err == nil {
				t.Errorf("want an error, got %d jobs", len(jobs))
			}
		})
	}
}

// A refusal means the IP's request budget is spent: the rest of the board's hydration is
// deferred instead of spending one more refused request per posting.
func TestParadoxStopsHydratingOnARefusal(t *testing.T) {
	const n = 40
	rows := make([]any, n)
	for i := range rows {
		rows[i] = paradoxRow(fmt.Sprintf("P1-%07d-1", i))
	}
	f := &paradoxFake{refuse: true, listing: func(page int) (map[string]any, error) {
		return paradoxPage(rows, n), nil
	}}
	jobs, err := paradoxFetch(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Errorf("got %d jobs from refused details, want none", len(jobs))
	}
	details := 0
	for _, a := range f.asked {
		if strings.Contains(a, "get_widget_extra_settings") {
			details++
		}
	}
	// Only the requests already in flight when the first refusal landed may go out.
	if details > defaultDetailWorkers {
		t.Errorf("sent %d detail requests after a refusal, want at most %d", details, defaultDetailWorkers)
	}
}

func TestParadoxIsPaced(t *testing.T) {
	src, ok := All(NewClient())["paradox"].(paradox)
	if !ok {
		t.Fatal(`All()["paradox"] is not the paradox adapter`)
	}
	if _, paced := src.http.(rateLimitedHeaderJSONGetter); !paced {
		t.Errorf("paradox's client is %T, want the paced getter — the API meters a per-IP budget", src.http)
	}
}

func TestParadoxRefusesAForeignBoard(t *testing.T) {
	for _, board := range []string{
		"JovieofNewportBeach",             // no host
		"olivia.paradox.ai",               // no site
		"olivia.paradox.ai/a/b",           // a path, not a site
		"example.com/JovieofNewportBeach", // not a paradox.ai host
		"paradox.ai.example.com/JovieofNewportBeach",
	} {
		f := &paradoxFake{}
		if _, err := NewParadox(f).Fetch(context.Background(), CompanyEntry{Company: "X", Board: board}); err == nil {
			t.Errorf("board %q: want an error", board)
		}
		if len(f.asked) != 0 {
			t.Errorf("board %q: sent %v before refusing", board, f.asked)
		}
	}
}

func TestParadoxIsFullBoardListing(t *testing.T) {
	if _, ok := NewParadox(nil).(fullBoardListing); !ok {
		t.Error("paradox must implement fullBoardListing")
	}
	if !FullBoardListingProviders(All(nil))["paradox"] {
		t.Error("FullBoardListingProviders(All(nil)) should include paradox")
	}
}

func TestParadoxDate(t *testing.T) {
	for in, want := range map[string]string{
		"26-09-23":   "2026-09-23",
		"2026-09-23": "2026-09-23",
		"":           "",
		"garbage":    "",
	} {
		got := paradoxDate(in)
		if (got == nil) != (want == "") || (got != nil && got.Format("2006-01-02") != want) {
			t.Errorf("paradoxDate(%q) = %v, want %q", in, got, want)
		}
	}
}

func TestParadoxEmploymentType(t *testing.T) {
	for _, c := range []struct {
		in   any
		want string
	}{
		{"", ""},
		{[]any{}, ""},
		{[]any{"Full Time"}, "full_time"},
		{[]any{"Part-Time"}, "part_time"},
		{[]any{"Full Time", "Part Time"}, ""},
		{[]any{"Seasonal"}, ""},
	} {
		if got := paradoxEmploymentType(c.in); got != c.want {
			t.Errorf("paradoxEmploymentType(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
