package sources

import (
	"context"
	"strings"
	"testing"
)

func TestZohoUnescape(t *testing.T) {
	cases := map[string]string{
		`\x3Cp\x3EHello\x3C/p\x3E`: "<p>Hello</p>", // hex escapes
		`a\x22b\x22c`:              `a"b"c`,        // escaped quotes
		`line\nbreak`:              "line\nbreak",  // backslash-n
		`path\/to`:                 "path/to",      // escaped slash
		`plain`:                    "plain",        // nothing to do
	}
	for in, want := range cases {
		if got := zohoUnescape(in); got != want {
			t.Errorf("zohoUnescape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestZohoElementAttrByID(t *testing.T) {
	root := parseHTML(t, `<html><body><input id="other" value="x"><input id="jobs" value='[{"id":"1"}]'></body></html>`)
	if got := elementAttrByID(root, "input", "jobs", "value"); got != `[{"id":"1"}]` {
		t.Errorf("elementAttrByID = %q", got)
	}
	if got := elementAttrByID(root, "input", "missing", "value"); got != "" {
		t.Errorf("missing id = %q, want empty", got)
	}
}

// zohoDetailHTML builds a detail page whose script embeds the record with JS-escaped quotes
// (\x22) and a Job_Description value, matching what the live site serves.
func zohoDetailHTML(description string) string {
	return `<html><body><script>var rec = "{\x22id\x22:\x221\x22,\x22Job_Description\x22:\x22` +
		description + `\x22,\x22Country\x22:null}";</script></body></html>`
}

func TestZohoFetch(t *testing.T) {
	listing := `<html><body><input id="jobs" value='[` +
		`{"id":"100","Posting_Title":"Backend Engineer","City":"Lisbon","Country":"Portugal","Remote_Job":false,"Publish":true},` +
		`{"id":"200","Posting_Title":"Remote Designer","City":null,"Country":null,"Remote_Job":true,"Publish":true},` +
		`{"id":"300","Posting_Title":"Draft Role","Publish":false}` +
		`]'></body></html>`
	http := (&routedHTTP{}).
		route("/jobs/Careers/100", zohoDetailHTML(`\x3Cp\x3EBuild things\x3C/p\x3E`)).
		route("/jobs/Careers/200", zohoDetailHTML(`\x3Cp\x3EDesign things\x3C/p\x3E`)).
		route("/jobs/Careers", listing)

	jobs, err := zoho{http: http}.Fetch(context.Background(),
		CompanyEntry{Company: "Acme", Board: "acme.zohorecruit.com"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// The unpublished record (300) is dropped.
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "100" || j.Title != "Backend Engineer" || j.Company != "Acme" {
		t.Errorf("job0 = id:%q title:%q company:%q", j.ExternalID, j.Title, j.Company)
	}
	if j.Location != "Lisbon, Portugal" {
		t.Errorf("job0 location = %q", j.Location)
	}
	if !strings.Contains(j.Description, "Build things") {
		t.Errorf("job0 description = %q", j.Description)
	}
	if j.URL != "https://acme.zohorecruit.com/jobs/Careers/100" {
		t.Errorf("job0 url = %q", j.URL)
	}
	// Remote record: structured remote flag → WorkMode remote.
	if jobs[1].WorkMode != "remote" || !jobs[1].Remote {
		t.Errorf("job1 workmode/remote = %q/%v", jobs[1].WorkMode, jobs[1].Remote)
	}
}
