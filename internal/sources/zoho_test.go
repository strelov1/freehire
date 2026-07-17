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
		// The richtext field is escaped twice, so markup arrives double-escaped: a
		// single pass would leave a stray backslash. Decoding to a fixpoint resolves it.
		`<\\\/span>`:    "</span>",    // double-escaped closing tag
		`<\\\/p>`:       "</p>",       // double-escaped closing tag
		`\\u2022`:       "•",          // double-escaped unicode bullet
		`\\\x22q\\\x22`: `"q"`,        // double-escaped quotes
		`margin\-top`:   "margin-top", // stray backslash-hyphen folds to hyphen
	}
	for in, want := range cases {
		if got := zohoUnescape(in); got != want {
			t.Errorf("zohoUnescape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestZohoJobsInputExtraction(t *testing.T) {
	// The listing JSON is read off the hidden <input id="jobs"> via the shared firstByID+attr.
	root := parseHTML(t, `<html><body><input id="other" value="x"><input id="jobs" value='[{"id":"1"}]'></body></html>`)
	if n := firstByID(root, "jobs"); n == nil || attr(n, "value") != `[{"id":"1"}]` {
		t.Error("firstByID(jobs) did not yield the #jobs array value")
	}
	if n := firstByID(root, "missing"); n != nil {
		t.Error("firstByID(missing) = non-nil, want nil")
	}
}

// zohoDetailHTML builds a detail page whose script embeds the record with JS-escaped quotes
// (\x22) and a Job_Description value, matching what the live site serves.
func zohoDetailHTML(description string) string {
	return `<html><body><script>var rec = "{\x22id\x22:\x221\x22,\x22Job_Description\x22:\x22` +
		description + `\x22,\x22Country\x22:null}";</script></body></html>`
}

// TestZohoDescriptionDoubleEscaped guards the real-world case where the detail record
// escapes the richtext value twice: closing tags arrive as <\\\/p> and bullets as \\u2022.
// Before the fixpoint decode these reached storage as visible &lt;\/p&gt; / • text.
func TestZohoDescriptionDoubleEscaped(t *testing.T) {
	desc := `<p style=\\\x22margin\-top:0px\\\x22>Duties\\u2022audit files.<br\/><\\\/p>`
	http := (&routedHTTP{}).route("/jobs/Careers/1", zohoDetailHTML(desc))

	got, ok := zoho{http: http}.description(context.Background(), "https://acme.zohorecruit.com/jobs/Careers/1")
	if !ok {
		t.Fatal("description: not found")
	}
	// A leftover backslash (e.g. •, <\/p>) or an entity-escaped tag (&lt;) means the
	// nested escaping was not fully peeled.
	if strings.Contains(got, "&lt;") || strings.Contains(got, `\`) {
		t.Errorf("description leaks escaped markup: %q", got)
	}
	for _, want := range []string{"</p>", "•", "Duties", "audit files"} {
		if !strings.Contains(got, want) {
			t.Errorf("description = %q, missing %q", got, want)
		}
	}
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
