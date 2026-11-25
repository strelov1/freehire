package aggregators

import (
	"github.com/strelov1/freehire/internal/ingest/sources/kit"

	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

const joobleTestKey = "sekrit-key-123"

// joobleHTTP is a paging-aware test JSONPoster: it serves pages[page-1] for the body's page
// number and records every URL and body. Like the real client, its errors carry the URL.
type joobleHTTP struct {
	pages    []string
	failPage int // 0 = never fail
	gotURLs  []string
	gotBody  []joobleRequest
}

func (f *joobleHTTP) PostJSON(_ context.Context, u string, body, v any) error {
	f.gotURLs = append(f.gotURLs, u)
	req := body.(joobleRequest)
	f.gotBody = append(f.gotBody, req)
	if req.Page == f.failPage {
		return fmt.Errorf("sources: POST %s failed after 1 attempts: unexpected status 403", u)
	}
	if req.Page < 1 || req.Page > len(f.pages) {
		return fmt.Errorf("joobleHTTP: no page %d", req.Page)
	}
	return json.Unmarshal([]byte(f.pages[req.Page-1]), v)
}

func newTestJooble(h *joobleHTTP) jooble {
	return jooble{http: h, key: joobleTestKey, now: func() time.Time { return time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC) }}
}

var joobleEntry = kit.CompanyEntry{Board: "golang developer", Region: "Kyiv"}

func joobleFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/jooble_search.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// joobleFullPage is a page of n postings under a feed that claims total matches.
func joobleFullPage(total, n int) string {
	jobs := make([]string, n)
	for i := range jobs {
		jobs[i] = fmt.Sprintf(`{"id":%d,"title":"T","link":"https://jooble.org/jdp/%d"}`, i+1, i+1)
	}
	return fmt.Sprintf(`{"totalCount":%d,"jobs":[%s]}`, total, strings.Join(jobs, ","))
}

func TestJoobleFetchMapsFields(t *testing.T) {
	// The fixture claims 3 matches but lists 2, so a second (empty) page is requested.
	h := &joobleHTTP{pages: []string{joobleFixture(t), `{"totalCount":3,"jobs":[]}`}}

	jobs, err := newTestJooble(h).Fetch(context.Background(), joobleEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("want 2 jobs, got %d", len(jobs))
	}
	got := jobs[0]
	if got.ExternalID != "1234567890" || got.Title != "Backend Engineer" || got.Company != "Acme" || got.Location != "Kyiv" {
		t.Fatalf("identity mismatch: %+v", got)
	}
	if got.URL != "https://ua.jooble.org/jdp/1234567890" {
		t.Fatalf("URL: %q", got.URL)
	}
	if !strings.HasPrefix(got.Description, "Join our platform team") {
		t.Fatalf("Description: %q", got.Description)
	}
	want := time.Date(2026, 11, 24, 12, 55, 35, 387000000, time.UTC)
	if got.PostedAt == nil || !got.PostedAt.Equal(want) {
		t.Fatalf("PostedAt = %v, want %v", got.PostedAt, want)
	}
	// A string id and a blank date decode too.
	if jobs[1].ExternalID != "9876543210" || jobs[1].PostedAt != nil {
		t.Fatalf("second job: %+v", jobs[1])
	}
}

// The key is the URL path, and the board's keywords and location plus the freshness window reach
// the body.
func TestJoobleFetchRequest(t *testing.T) {
	h := &joobleHTTP{pages: []string{`{"totalCount":0,"jobs":[]}`}}
	if _, err := newTestJooble(h).Fetch(context.Background(), joobleEntry); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if h.gotURLs[0] != joobleBaseURL+joobleTestKey {
		t.Fatalf("URL = %q", h.gotURLs[0])
	}
	b := h.gotBody[0]
	if b.Keywords != "golang developer" || b.Location != "Kyiv" || b.Page != 1 || b.ResultOnPage != joobleResultOnPage {
		t.Fatalf("body = %+v", b)
	}
	if b.DateCreatedFrom != "2026-11-23" { // now minus joobleMaxDaysOld
		t.Fatalf("datecreatedfrom = %q", b.DateCreatedFrom)
	}
}

// The walk ends once totalCount postings are read, without spending a request on an empty page.
func TestJoobleFetchStopsAtTotalCount(t *testing.T) {
	h := &joobleHTTP{pages: []string{joobleFullPage(3, 2), joobleFullPage(3, 1), joobleFullPage(3, 1)}}
	jobs, err := newTestJooble(h).Fetch(context.Background(), joobleEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 || len(h.gotURLs) != 2 {
		t.Fatalf("want 3 jobs in 2 requests, got %d jobs in %d requests", len(jobs), len(h.gotURLs))
	}
}

// An empty page ends the walk even when totalCount promises more.
func TestJoobleFetchStopsOnEmptyPage(t *testing.T) {
	h := &joobleHTTP{pages: []string{joobleFullPage(99, 2), `{"totalCount":99,"jobs":[]}`, joobleFullPage(99, 2)}}
	jobs, err := newTestJooble(h).Fetch(context.Background(), joobleEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 || len(h.gotURLs) != 2 {
		t.Fatalf("want 2 jobs in 2 requests, got %d jobs in %d requests", len(jobs), len(h.gotURLs))
	}
}

// A deep feed is cut at the page budget.
func TestJoobleFetchStopsAtThePageBudget(t *testing.T) {
	pages := make([]string, joobleMaxPages+3)
	for i := range pages {
		pages[i] = joobleFullPage(10000, 2)
	}
	h := &joobleHTTP{pages: pages}
	if _, err := newTestJooble(h).Fetch(context.Background(), joobleEntry); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(h.gotURLs) != joobleMaxPages {
		t.Fatalf("want %d requests, got %d", joobleMaxPages, len(h.gotURLs))
	}
}

// The page budget is one leg of the free plan's 100 requests/day per key; a silent bump here is
// spent against a ceiling nothing else in the process checks.
func TestJooblePageBudgetStaysWithinTheDailyCeiling(t *testing.T) {
	const (
		boards       = 4   // keyword boards on the one US key
		runsPerDay   = 4   // freehire-ops' provision/host2/systemd/freehire-ingest@jooble.timer
		dailyCeiling = 100 // Jooble's free plan, per key, reset at midnight UTC
	)
	if perDay := boards * joobleMaxPages * runsPerDay; perDay > dailyCeiling {
		t.Fatalf("crawl budget is %d requests/day, over Jooble's ceiling of %d", perDay, dailyCeiling)
	}
}

func TestJoobleFetchRejectsBlankBoard(t *testing.T) {
	j := newTestJooble(&joobleHTTP{})
	if _, err := j.Fetch(context.Background(), kit.CompanyEntry{Board: "go"}); err == nil {
		t.Fatal("want error on blank location")
	}
	if _, err := j.Fetch(context.Background(), kit.CompanyEntry{Region: "Kyiv"}); err == nil {
		t.Fatal("want error on blank keywords")
	}
}

// A rejected key (403) is a board-level error, and the key, a URL path segment the shared client
// echoes into its errors, must not survive into the message.
func TestJoobleFirstPageErrorHidesTheKey(t *testing.T) {
	h := &joobleHTTP{failPage: 1}
	_, err := newTestJooble(h).Fetch(context.Background(), joobleEntry)
	if err == nil {
		t.Fatal("want error on first-page failure")
	}
	if strings.Contains(err.Error(), joobleTestKey) {
		t.Fatalf("key leaked into the error: %v", err)
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("the status was lost: %v", err)
	}
}

// A later page failing ends the walk with what was gathered, and its log line hides the key too.
func TestJoobleLaterPageFailureReturnsPartialWithoutLoggingTheKey(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	h := &joobleHTTP{pages: []string{joobleFullPage(99, 2)}, failPage: 2}
	jobs, err := newTestJooble(h).Fetch(context.Background(), joobleEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("want the 2 gathered jobs, got %d", len(jobs))
	}
	if logs.Len() == 0 || strings.Contains(logs.String(), joobleTestKey) {
		t.Fatalf("log must be written and key-free: %q", logs.String())
	}
}
