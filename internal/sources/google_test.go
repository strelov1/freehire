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

func googleFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func TestGoogleProvider(t *testing.T) {
	if got := NewGoogle(nil).Provider(); got != "google" {
		t.Errorf("Provider() = %q, want %q", got, "google")
	}
}

func TestGoogleIsBoardless(t *testing.T) {
	if _, ok := NewGoogle(nil).(boardless); !ok {
		t.Error("google adapter should be boardless (single-company, no board id)")
	}
}

// TestGoogleExtractDS1 pins the ds:1 blob extraction: the records list and the total
// count are read out of the embedded AF_initDataCallback payload.
func TestGoogleExtractDS1(t *testing.T) {
	root, err := html.Parse(strings.NewReader(googleFixture(t, "google_list.html")))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	records, total, err := extractGoogleDS1(root)
	if err != nil {
		t.Fatalf("extractGoogleDS1: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if total != 3640 {
		t.Errorf("total = %d, want 3640", total)
	}
}

// TestGoogleToJob pins the brittle positional mapping of one real record.
func TestGoogleToJob(t *testing.T) {
	root, _ := html.Parse(strings.NewReader(googleFixture(t, "google_list.html")))
	records, _, err := extractGoogleDS1(root)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	j, ok := google{}.toJob(CompanyEntry{Company: "Google"}, records[0])
	if !ok {
		t.Fatal("toJob returned ok=false for a valid record")
	}
	if j.ExternalID != "132507027548054214" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.Title != "Staff Developer Experience Engineer, DeepMind" {
		t.Errorf("Title = %q", j.Title)
	}
	// Company is the posting's own hiring brand, not the configured umbrella name.
	if j.Company != "DeepMind" {
		t.Errorf("Company = %q, want DeepMind", j.Company)
	}
	wantURL := "https://www.google.com/about/careers/applications/jobs/results/132507027548054214"
	if j.URL != wantURL {
		t.Errorf("URL = %q, want %q", j.URL, wantURL)
	}
	if j.Location != "Bengaluru, Karnataka, India" {
		t.Errorf("Location = %q", j.Location)
	}
	// Description is sanitized HTML assembling about-the-job + responsibilities + qualifications.
	for _, want := range []string{
		"DeepMind Developer Experience works at the intersection",
		"Define and lead the technical go-to-market",
		"Minimum qualifications:",
	} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q\ngot: %s", want, j.Description)
		}
	}
	if strings.Contains(j.Description, "<script") {
		t.Errorf("Description not sanitized: %s", j.Description)
	}
	wantDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if j.PostedAt == nil || !j.PostedAt.Equal(wantDate) {
		t.Errorf("PostedAt = %v, want %v", j.PostedAt, wantDate)
	}
}

// TestGoogleToJobGuards pins the degrade-gracefully guards the paging loop relies on: a
// record with no id is dropped (it would collide on the dedup key), and a record missing the
// timestamp field maps to a nil posted_at rather than panicking on the out-of-range index.
func TestGoogleToJobGuards(t *testing.T) {
	if _, ok := (google{}).toJob(CompanyEntry{}, json.RawMessage(`["","No ID"]`)); ok {
		t.Error("record with empty id should be dropped (ok=false)")
	}
	j, ok := google{}.toJob(CompanyEntry{Company: "Google"}, json.RawMessage(`["42","Short Record"]`))
	if !ok {
		t.Fatal("record with an id but missing later fields should still map")
	}
	if j.PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil when the timestamp field is absent", j.PostedAt)
	}
	if j.Company != "Google" {
		t.Errorf("Company = %q, want the configured fallback when brand field is absent", j.Company)
	}
}

// TestGoogleFetchPagesUntilEmpty: the loop pages until a page yields no records.
func TestGoogleFetchPagesUntilEmpty(t *testing.T) {
	fake := (&routedHTTP{}).
		route("page=1", googleFixture(t, "google_list.html")).
		route("page=2", googleFixture(t, "google_empty.html"))
	jobs, err := NewGoogle(fake).Fetch(context.Background(), CompanyEntry{Company: "Google"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	if jobs[0].ExternalID != "132507027548054214" || jobs[1].ExternalID != "139289021426606790" {
		t.Errorf("unexpected jobs: %q, %q", jobs[0].ExternalID, jobs[1].ExternalID)
	}
}

// TestGoogleFetchStopsAtTotal: once the running count reaches the payload total, the loop
// stops without requesting the next page (google_full.html has total=2). A page-2 request
// would hit the unrouted fake and error, so a clean return proves the early stop.
func TestGoogleFetchStopsAtTotal(t *testing.T) {
	fake := (&routedHTTP{}).route("page=1", googleFixture(t, "google_full.html"))
	jobs, err := NewGoogle(fake).Fetch(context.Background(), CompanyEntry{Company: "Google"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
}
