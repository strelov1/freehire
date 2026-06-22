package sources

import (
	"context"
	"strings"
	"testing"
)

func TestFactorialLocation(t *testing.T) {
	cases := []struct {
		name     string
		rows     []string
		wantLoc  string
		wantMode string
	}{
		{
			name:     "work-mode parenthetical (IT)",
			rows:     []string{"A tempo indeterminato", "Full time", "€35,000 - €50,000", "Ibrido (Milano, Lombardia, Italia)", "S - Partnerships (PAR)"},
			wantLoc:  "Milano, Lombardia, Italia",
			wantMode: "hybrid",
		},
		{
			name:    "plain comma-separated (BR)",
			rows:    []string{"Clt", "Período integral", "R$1.937,6 Mensal", "RIO DAS OSTRAS, RJ, Brasil", "QUALIDADE"},
			wantLoc: "RIO DAS OSTRAS, RJ, Brasil",
		},
		{
			// A team row has a parenthetical but no comma inside it, and a single-region
			// location with no comma is not detected — both leave location empty.
			name: "no comma anywhere → empty",
			rows: []string{"Full time", "TRIVENETO", "S - Partnerships (PAR)"},
		},
		{
			name:     "remote label maps",
			rows:     []string{"Remoto (Madrid, España)"},
			wantLoc:  "Madrid, España",
			wantMode: "remote",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loc, mode := factorialLocation(tc.rows)
			if loc != tc.wantLoc || mode != tc.wantMode {
				t.Errorf("factorialLocation = (%q,%q), want (%q,%q)", loc, mode, tc.wantLoc, tc.wantMode)
			}
		})
	}
}

func TestFactorialID(t *testing.T) {
	cases := map[string]string{
		"https://muffin.factorial.it/job_posting/partnership-success-manager-305055": "305055",
		"https://x.factorialhr.com.br/job_posting/tecnico-de-inspecao-308827":        "308827",
		"https://x.factorial.it/job_posting/no-id-here":                              "", // no trailing -<id>
	}
	for url, want := range cases {
		m := factorialIDPattern.FindStringSubmatch(url)
		got := ""
		if m != nil {
			got = m[1]
		}
		if got != want {
			t.Errorf("id(%q) = %q, want %q", url, got, want)
		}
	}
}

// factorialDetailHTML builds a job page: an h1 title, the metadata list (each row in a
// border-gray-50 li), and the description in a div.styledText.
func factorialDetailHTML(title, salary, location, description string) string {
	return `<html><body>` +
		`<ul><li class="flex border-b border-gray-50">A tempo indeterminato</li>` +
		`<li class="flex border-b border-gray-50">` + salary + `</li>` +
		`<li class="flex border-b border-gray-50">` + location + `</li></ul>` +
		`<h1 class="font-bold">` + title + `</h1>` +
		`<div class='styledText'>` + description + `</div>` +
		`</body></html>`
}

func TestFactorialFetch(t *testing.T) {
	listing := `<html><body><ul>
		<li class='job-offer-item'><a href="/job_posting/backend-engineer-100">Backend Engineer</a></li>
		<li class='job-offer-item'><a href="/job_posting/backend-engineer-100">Apply</a></li>
		<li class='job-offer-item'><a href="/job_posting/data-analyst-200">Data Analyst</a></li>
	</ul></body></html>`
	// Detail routes are listed before the listing route: routedHTTP returns the first route
	// whose match string is a substring of the URL, and the listing root is a substring of
	// every detail URL.
	http := (&routedHTTP{}).
		route("/job_posting/backend-engineer-100", factorialDetailHTML("Backend Engineer", "€40,000", "Ibrido (Milano, Italia)", "<p>Build things</p>")).
		route("/job_posting/data-analyst-200", factorialDetailHTML("Data Analyst", "€30,000", "Remoto (Madrid, España)", "<p>Analyze data</p>")).
		route("https://acme.factorial.it/", listing)

	jobs, err := factorial{http: http}.Fetch(context.Background(),
		CompanyEntry{Company: "Acme", Board: "acme.factorial.it"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 { // the duplicate "Apply" link to job 100 is de-duped
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "100" || j.Title != "Backend Engineer" || j.Company != "Acme" {
		t.Errorf("job0 = id:%q title:%q company:%q", j.ExternalID, j.Title, j.Company)
	}
	if j.Location != "Milano, Italia" || j.WorkMode != "hybrid" {
		t.Errorf("job0 location/workmode = %q/%q", j.Location, j.WorkMode)
	}
	if !strings.Contains(j.Description, "Build things") {
		t.Errorf("job0 description = %q", j.Description)
	}
	if jobs[1].WorkMode != "remote" || !jobs[1].Remote {
		t.Errorf("job1 workmode/remote = %q/%v", jobs[1].WorkMode, jobs[1].Remote)
	}
}
