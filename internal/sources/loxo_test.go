package sources

import (
	"context"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestLoxoProvider(t *testing.T) {
	if got := NewLoxo(nil).Provider(); got != "loxo" {
		t.Errorf("Provider() = %q, want %q", got, "loxo")
	}
}

// TestLoxoMapDetailFixture parses a real Loxo detail page (fitnext) and asserts the
// mapping: title from og:title (no " | Agency" suffix), description from the embedded
// JSON blob, canonical /job/<base64> URL, decoded <agency_id>-<slug> external id, and the
// location parsed from the og:description "Location: … Salary:" run.
func TestLoxoMapDetailFixture(t *testing.T) {
	root := parseFixture(t, "testdata/loxo/detail_fitnext.html")
	loc := "https://fitnext.app.loxo.co/job/NDI0NzQtN3RzZTNpa2M2NDJ5emowdg=="

	j, ok := mapLoxoDetail(root, CompanyEntry{Company: "FitNext Co."}, loc)
	if !ok {
		t.Fatal("mapLoxoDetail returned ok=false on a real detail page")
	}
	if j.Title != "Frontend Engineer (React + TypeScript)" {
		t.Errorf("Title = %q, want the role without the agency suffix", j.Title)
	}
	if j.ExternalID != "42474-7tse3ikc642yzj0v" {
		t.Errorf("ExternalID = %q, want decoded <agency_id>-<slug>", j.ExternalID)
	}
	if j.URL != loc {
		t.Errorf("URL = %q, want canonical detail URL %q", j.URL, loc)
	}
	if !strings.Contains(j.Description, "Frontend Engineer") || j.Description == "" {
		t.Errorf("Description missing/empty: %.80q", j.Description)
	}
	if !strings.Contains(j.Location, "LATAM") {
		t.Errorf("Location = %q, want the Remote (LATAM) parsed from og:description", j.Location)
	}
	if !j.Remote {
		t.Errorf("Remote = false, want true for a Remote (LATAM) posting")
	}
}

// TestLoxoJobLinksHostResolution asserts the listing yields one link per posting and that
// each is resolved against the board host, so pods and agency subdomains all work.
func TestLoxoJobLinksHostResolution(t *testing.T) {
	root := parseFixture(t, "testdata/loxo/listing_fitnext.html")
	for _, host := range []string{"fitnext.app.loxo.co", "pod4.app.loxo.co", "app.loxo.co"} {
		base := mustURL(t, "https://"+host+"/fitnext")
		links := loxoJobLinks(base, root)
		if len(links) != 22 {
			t.Errorf("%s: got %d job links, want 22", host, len(links))
		}
		for _, l := range links {
			if !strings.HasPrefix(l, "https://"+host+"/job/") {
				t.Errorf("%s: link %q not resolved against the board host", host, l)
				break
			}
		}
	}
}

// TestLoxoFetchMapsAndIsolatesFailures drives Fetch over an inline listing: the routed
// posting maps to a Job; the unrouted posting's detail fetch fails and is dropped, not fatal.
func TestLoxoFetchMapsAndIsolatesFailures(t *testing.T) {
	const listing = `<html><body>
<a href="/job/YWJjLTE=">Role A</a>
<a href="/job/YWJjLTI=">Role B</a>
</body></html>`
	const detailA = `<html><head>
<meta property="og:title" content="Role A">
<meta property="og:description" content="Role ALocation: RemoteSalary: x">
<script type="application/json">{"description":"<p>Do A</p>"}</script>
</head></html>`

	fake := (&routedHTTP{}).
		route("/job/YWJjLTE=", detailA).
		route("acme", listing)

	jobs, err := NewLoxo(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "app.loxo.co/acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the unrouted posting must drop, not fail the crawl)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "abc-1" || j.Company != "Acme" || j.Title != "Role A" {
		t.Errorf("job = {id:%q company:%q title:%q}, want {abc-1 Acme Role A}", j.ExternalID, j.Company, j.Title)
	}
	if j.URL != "https://app.loxo.co/job/YWJjLTE=" {
		t.Errorf("URL = %q, want relative /job link resolved against board host", j.URL)
	}
}

func TestLoxoCompany(t *testing.T) {
	hub := CompanyEntry{Company: "FitNext Co.", Hub: true}
	plain := CompanyEntry{Company: "FitNext Co."}
	cases := []struct {
		name, title       string
		e                 CompanyEntry
		wantCo, wantTitle string
	}{
		{"hub em-dash client", "Senior SWE — Acme Corp", hub, "Acme Corp", "Senior SWE"},
		{"hub at client", "Backend Engineer @ Globex", hub, "Globex", "Backend Engineer"},
		{"hub no delimiter falls back", "Frontend Engineer", hub, "FitNext Co.", "Frontend Engineer"},
		{"hub ascii hyphen not a delimiter", "Full-Stack Developer - Nashville", hub, "FitNext Co.", "Full-Stack Developer - Nashville"},
		{"non-hub always agency", "Senior SWE — Acme Corp", plain, "FitNext Co.", "Senior SWE — Acme Corp"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			co, title := loxoCompany(c.title, c.e)
			if co != c.wantCo || title != c.wantTitle {
				t.Errorf("loxoCompany(%q) = (%q, %q), want (%q, %q)", c.title, co, title, c.wantCo, c.wantTitle)
			}
		})
	}
}

func TestLoxoExternalID(t *testing.T) {
	cases := map[string]string{
		"https://fitnext.app.loxo.co/job/NDI0NzQtN3RzZTNpa2M2NDJ5emowdg==": "42474-7tse3ikc642yzj0v",
		"https://app.loxo.co/job/YWJjLTE=":                                "abc-1",
		"https://app.loxo.co/job/NDI0NzQtN3RzZTNpa2M2NDJ5emowdg==?t=99":    "42474-7tse3ikc642yzj0v",
		"https://app.loxo.co/careers-in-nonprofits":                       "",
		"https://app.loxo.co/job/":                                        "",
	}
	for u, want := range cases {
		if got := loxoExternalID(u); got != want {
			t.Errorf("loxoExternalID(%q) = %q, want %q", u, got, want)
		}
	}
}

func parseFixture(t *testing.T, path string) *html.Node {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	root, err := html.Parse(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("parse fixture %s: %v", path, err)
	}
	return root
}
