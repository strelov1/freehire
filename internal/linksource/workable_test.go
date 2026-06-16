package linksource

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

// workableBoardJSON mirrors the public widget API (details=true): all of an account's jobs
// inline, each with its shortcode — the link-source picks the one the link names.
const workableBoardJSON = `{
 "jobs": [
   {"title": "Frontend Engineer", "shortcode": "AAAA111111", "url": "https://apply.workable.com/jobhire/j/AAAA111111", "description": "<p>Other role.</p>", "published_on": "2026-05-01", "city": "Limassol", "country": "Cyprus", "telecommuting": false},
   {"title": "Lead AI Product Manager", "shortcode": "915C6E469E", "url": "https://apply.workable.com/jobhire/j/915C6E469E", "description": "<p>Own it.</p><script>x()</script>", "published_on": "2026-06-10", "city": "", "country": "Cyprus", "telecommuting": true}
 ]
}`

func TestWorkableResolvesNamedShortcode(t *testing.T) {
	const link = "https://apply.workable.com/jobhire/j/915C6E469E"
	c := (&fakeClient{}).route("api/v1/widget/accounts/jobhire", workableBoardJSON, "")

	job, ok, err := NewWorkable(c).Resolve(context.Background(), link)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !ok {
		t.Fatal("ok=false, want the named vacancy resolved")
	}
	// external_id is the shortcode, matching what the ingest workable adapter writes, so the
	// same vacancy crawled directly dedups instead of duplicating.
	if job.ExternalID != "915C6E469E" {
		t.Errorf("ExternalID = %q, want 915C6E469E", job.ExternalID)
	}
	if job.Title != "Lead AI Product Manager" {
		t.Errorf("Title = %q (picked wrong shortcode?)", job.Title)
	}
	if job.Company != "Jobhire" {
		t.Errorf("Company = %q, want Jobhire (humanized account)", job.Company)
	}
	if job.Location != "Cyprus" {
		t.Errorf("Location = %q, want Cyprus", job.Location)
	}
	if !job.Remote {
		t.Error("Remote = false, want true (telecommuting)")
	}
	if got := job.Description; strings.Contains(got, "<script>") || strings.Contains(got, "x()") || !strings.Contains(got, "Own it.") {
		t.Errorf("Description not sanitized/assembled: %q", got)
	}
}

// A link whose shortcode is not on the board (closed/removed) is matched but unresolved.
func TestWorkableUnknownShortcodeSkips(t *testing.T) {
	const link = "https://apply.workable.com/jobhire/j/ZZZZ999999"
	c := (&fakeClient{}).route("api/v1/widget/accounts/jobhire", workableBoardJSON, "")

	_, ok, err := NewWorkable(c).Resolve(context.Background(), link)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ok {
		t.Error("ok=true, want false for a shortcode not on the board")
	}
}

func TestWorkableMatch(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"https://apply.workable.com/jobhire/j/915C6E469E", true},
		{"https://apply.workable.com/jobhire/j/915C6E469E/apply", true},
		{"https://apply.workable.com/jobhire", false}, // board, not one posting
		{"https://jobs.lever.co/x/abc", false},        // other host
	}
	for _, tc := range cases {
		u, _ := url.Parse(tc.raw)
		if got := NewWorkable(nil).Match(u); got != tc.want {
			t.Errorf("Match(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestWorkableSourceKeyMatchesIngestProvider(t *testing.T) {
	if got := NewWorkable(nil).Source(); got != "workable" {
		t.Errorf("Source() = %q, want workable", got)
	}
}
