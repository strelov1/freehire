package sources

import (
	"context"
	"strings"
	"testing"
	"time"
)

// jobviteListingHTML is a Jobvite careersite listing page (jobs.jobvite.com/<board>/jobs). It
// carries the given job codes as /<board>/job/<code> anchors, plus the /<board>/jobs and
// /<board>/jobAlerts navigation anchors that must NOT be treated as postings — exercising the
// job-link predicate.
func jobviteListingHTML(board string, codes ...string) string {
	var b strings.Builder
	b.WriteString(`<html><body><div class="jv-page-body"><ul class="jv-job-list">`)
	for _, c := range codes {
		b.WriteString(`<li class="jv-job-list-item">`)
		b.WriteString(`<a class="jv-job-list-name" href="/` + board + `/job/` + c + `">A Role</a>`)
		b.WriteString(`<span class="jv-job-list-location">New York</span>`)
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul>`)
	b.WriteString(`<a href="/` + board + `/jobs">All jobs</a>`)
	b.WriteString(`<a href="/` + board + `/jobAlerts">Job alerts</a>`)
	b.WriteString(`</div></body></html>`)
	return b.String()
}

// jobviteDetailHTML is a Jobvite job page: server-rendered HTML whose payload we read is the
// schema.org JobPosting ld+json. jobLocation is an ARRAY (the shape Jobvite emits), the
// description is RAW HTML (not entity-escaped) embedding a <script> that sanitizeHTML must
// strip, and datePosted is date-only.
func jobviteDetailHTML(title, code string) string {
	return `<html><head></head><body>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
"title":"` + title + `",
"identifier":"` + code + `",
"description":"<p>Build books.</p><script>alert(1)<\/script>",
"datePosted":"2026-06-30",
"employmentType":"Full-Time",
"hiringOrganization":{"@type":"Organization","name":"Hachette (schema.org)"},
"jobLocation":[{"@type":"Place","address":{"@type":"PostalAddress",
"addressLocality":"New York","addressRegion":"New York","addressCountry":"United States"}}]}
</script>
</body></html>`
}

// jobviteRemoteSingleLocationHTML is a job page whose jobLocation is a SINGLE object (not an
// array) with a Remote locality — proving the flexible single-or-array decode and the
// location-derived remote signal (Jobvite gives no structured jobLocationType).
func jobviteRemoteSingleLocationHTML(title, code string) string {
	return `<html><body>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
"title":"` + title + `","identifier":"` + code + `",
"description":"<p>Work anywhere.</p>","datePosted":"2026-07-01",
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress",
"addressLocality":"Remote","addressCountry":"United States"}}}
</script></body></html>`
}

func TestJobviteProvider(t *testing.T) {
	if got := NewJobvite(nil).Provider(); got != "jobvite" {
		t.Errorf("Provider() = %q, want %q", got, "jobvite")
	}
}

func TestJobviteJobID(t *testing.T) {
	cases := map[string]string{
		"https://jobs.jobvite.com/hbg/job/ojJpAfwL":          "ojJpAfwL",
		"https://jobs.jobvite.com/hbg/job/ode8zfwS?nl=1":     "ode8zfwS",
		"https://jobs.jobvite.com/hbg/jobs":                  "",
		"https://jobs.jobvite.com/hbg/jobAlerts":             "",
		"https://jobs.jobvite.com/hbg/job/":                  "",
		"https://www.jobvite.com/support/job-seeker-support": "",
	}
	for in, want := range cases {
		if got := jobviteJobID(in); got != want {
			t.Errorf("jobviteJobID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJobviteFetchListingThenDetailAndMaps(t *testing.T) {
	board := "hbg"
	fake := (&routedHTTP{}).
		route("/"+board+"/jobs", jobviteListingHTML(board, "ojJpAfwL")).
		route("/"+board+"/job/ojJpAfwL", jobviteDetailHTML("Editorial Assistant", "ojJpAfwL"))

	jobs, err := NewJobvite(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Hachette Book Group", Provider: "jobvite", Board: board,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "ojJpAfwL" {
		t.Errorf("ExternalID = %q, want %q", j.ExternalID, "ojJpAfwL")
	}
	if j.URL != "https://jobs.jobvite.com/"+board+"/job/ojJpAfwL" {
		t.Errorf("URL = %q, want canonical detail URL", j.URL)
	}
	if j.Title != "Editorial Assistant" {
		t.Errorf("Title = %q", j.Title)
	}
	// The configured company is canonical (the board is that employer's site); the JSON-LD
	// hiringOrganization is ignored so a board never mislabels its own postings.
	if j.Company != "Hachette Book Group" {
		t.Errorf("Company = %q, want configured company", j.Company)
	}
	if j.Location != "New York, United States" {
		t.Errorf("Location = %q, want %q", j.Location, "New York, United States")
	}
	if strings.Contains(j.Description, "<script>") || strings.Contains(j.Description, "alert(1)") {
		t.Errorf("Description not sanitized: %q", j.Description)
	}
	if !strings.Contains(j.Description, "Build books") {
		t.Errorf("Description lost real content: %q", j.Description)
	}
	if j.PostedAt == nil || !j.PostedAt.Equal(time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PostedAt = %v, want 2026-06-30", j.PostedAt)
	}
	if j.Remote {
		t.Errorf("Remote = true, want false for a New York location")
	}
}

func TestJobviteFetchSingleObjectLocationAndRemote(t *testing.T) {
	board := "acme"
	fake := (&routedHTTP{}).
		route("/"+board+"/jobs", jobviteListingHTML(board, "zzRemote1")).
		route("/"+board+"/job/zzRemote1", jobviteRemoteSingleLocationHTML("Staff Engineer", "zzRemote1"))

	jobs, err := NewJobvite(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "jobvite", Board: board,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (single-object jobLocation must decode)", len(jobs))
	}
	j := jobs[0]
	if j.Location != "Remote, United States" {
		t.Errorf("Location = %q, want %q", j.Location, "Remote, United States")
	}
	if !j.Remote {
		t.Errorf("Remote = false, want true for a Remote location")
	}
}

func TestJobviteListingErrorIsBoardLevel(t *testing.T) {
	// An empty router returns an error for the listing fetch, which must surface as a
	// board-level error (not a silent empty result).
	_, err := NewJobvite(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "jobvite", Board: "acme",
	})
	if err == nil {
		t.Fatal("Fetch: want board-level error on listing failure, got nil")
	}
}
