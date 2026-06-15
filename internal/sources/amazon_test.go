package sources

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// amazonHTTP is an offset-aware test JSONGetter: amazon.jobs pages its search.json by an
// offset query parameter, so this fake routes a canned response on the offset parsed from
// the URL. An unrequested offset returns an empty, zero-hit page — the natural end-of-list
// response that stops pagination.
type amazonHTTP struct {
	pages   map[int]string
	fail    bool
	gotURLs []string
}

var amazonOffsetRE = regexp.MustCompile(`offset=(\d+)`)

func (f *amazonHTTP) GetJSON(_ context.Context, url string, v any) error {
	f.gotURLs = append(f.gotURLs, url)
	if f.fail {
		return errors.New("amazonHTTP: boom")
	}
	off := 0
	if m := amazonOffsetRE.FindStringSubmatch(url); m != nil {
		off, _ = strconv.Atoi(m[1])
	}
	raw, ok := f.pages[off]
	if !ok {
		raw = `{"hits":0,"jobs":[]}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func TestAmazonProvider(t *testing.T) {
	if got := NewAmazon(nil).Provider(); got != "amazon" {
		t.Errorf("Provider() = %q, want %q", got, "amazon")
	}
}

func TestAmazonFetchPaginatesAndMaps(t *testing.T) {
	fake := &amazonHTTP{pages: map[int]string{
		0: `{"hits":3,"jobs":[
			{"id_icims":"10449371","title":"Software Development Engineer III ","job_path":"/en/jobs/10449371/sde-iii","normalized_location":"Redmond, Washington, USA","description":"<p>Build it</p><script>alert(1)</script>","posted_date":"June 15, 2026"},
			{"id_icims":"10449999","title":"Data Engineer","job_path":"/en/jobs/10449999/data-engineer","normalized_location":"Vancouver, BC, Canada","description":"<p>Pipelines</p>","posted_date":""}
		]}`,
		100: `{"hits":3,"jobs":[
			{"id_icims":"10450000","title":"SRE","job_path":"/en/jobs/10450000/sre","normalized_location":"Dublin, Ireland","description":"<p>Keep it up</p>","posted_date":"June 14, 2026"}
		]}`,
	}}

	jobs, err := NewAmazon(fake).Fetch(context.Background(), CompanyEntry{Company: "Amazon", Provider: "amazon"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (two pages)", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j := byID["10449371"]
	if j.Title != "Software Development Engineer III" {
		t.Errorf("Title = %q, want trimmed", j.Title)
	}
	if j.Company != "Amazon" {
		t.Errorf("Company = %q, want Amazon", j.Company)
	}
	if want := "https://www.amazon.jobs/en/jobs/10449371/sde-iii"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if want := "Redmond, Washington, USA"; j.Location != want {
		t.Errorf("Location = %q, want %q", j.Location, want)
	}
	if !strings.Contains(j.Description, "<p>Build it</p>") || strings.Contains(j.Description, "<script>") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-15", j.PostedAt)
	}

	if byID["10449999"].PostedAt != nil {
		t.Errorf("blank posted_date should yield nil PostedAt, got %v", byID["10449999"].PostedAt)
	}
}

func TestAmazonStopsAtEmptyPage(t *testing.T) {
	// hits claims 9 but the source returns one job then an empty page: the adapter must
	// stop on the empty page rather than loop forever chasing the count.
	fake := &amazonHTTP{pages: map[int]string{
		0: `{"hits":9,"jobs":[{"id_icims":"1","title":"Only","job_path":"/en/jobs/1/only","normalized_location":"Remote","description":"x","posted_date":""}]}`,
	}}
	jobs, err := NewAmazon(fake).Fetch(context.Background(), CompanyEntry{Company: "Amazon"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (stop at empty page)", len(jobs))
	}
}

func TestAmazonTransportErrorFailsBoard(t *testing.T) {
	fake := &amazonHTTP{fail: true}
	if _, err := NewAmazon(fake).Fetch(context.Background(), CompanyEntry{Company: "Amazon"}); err == nil {
		t.Fatal("Fetch: want transport error, got nil")
	}
}

func TestAmazonRegisteredInAllAndBoardless(t *testing.T) {
	s, ok := All(nil)["amazon"]
	if !ok {
		t.Fatal(`All(nil)["amazon"] missing`)
	}
	if _, isBoardless := s.(boardless); !isBoardless {
		t.Error("amazon should be boardless (single company, no board id)")
	}
}
