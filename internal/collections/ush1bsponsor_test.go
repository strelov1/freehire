package collections

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// indexPageFixture mirrors the real markup of
// https://www.uscis.gov/archive/h-1b-employer-data-hub-files, trimmed to a handful
// of years plus one unrelated link, verified against the live page during design.
const indexPageFixture = `
<p>Some intro text and an unrelated link.</p>
<a href="/tools/reports-and-studies/h-1b-employer-data-hub">Live query tool</a>
<a href="/sites/default/files/document/data/h1b_datahubexport-2023.csv" data-entity-type="media">FY 2023 H-1B Employer Data</a>
<a href="/sites/default/files/document/data/h1b_datahubexport-2022.csv" data-entity-type="media">FY 2022 H-1B Employer Data</a>
<a href="/sites/default/files/document/data/h1b_datahubexport-2021.csv" data-entity-type="media">FY 2021 H-1B Employer Data</a>
<a href="/sites/default/files/document/data/h1b_datahubexport-2020.csv" data-entity-type="media">FY 2020 H-1B Employer Data</a>
<a href="/sites/default/files/document/data/h1b_datahubexport-2019.csv" data-entity-type="media">FY 2019 H-1B Employer Data</a>
<a href="/sites/default/files/document/data/h1b_datahubexport-2018.csv" data-entity-type="media">FY 2018 H-1B Employer Data</a>
`

func TestDiscoverRecentFYFiles_ReturnsTheNMostRecentYearsDescending(t *testing.T) {
	got, err := discoverRecentFYFiles([]byte(indexPageFixture), 5)
	if err != nil {
		t.Fatalf("discoverRecentFYFiles: %v", err)
	}
	want := []fyFile{
		{Year: 2023, URL: "/sites/default/files/document/data/h1b_datahubexport-2023.csv"},
		{Year: 2022, URL: "/sites/default/files/document/data/h1b_datahubexport-2022.csv"},
		{Year: 2021, URL: "/sites/default/files/document/data/h1b_datahubexport-2021.csv"},
		{Year: 2020, URL: "/sites/default/files/document/data/h1b_datahubexport-2020.csv"},
		{Year: 2019, URL: "/sites/default/files/document/data/h1b_datahubexport-2019.csv"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("discoverRecentFYFiles =\n%+v\nwant\n%+v", got, want)
	}
}

func TestDiscoverRecentFYFiles_IgnoresUnrelatedLinks(t *testing.T) {
	got, err := discoverRecentFYFiles([]byte(indexPageFixture), 6)
	if err != nil {
		t.Fatalf("discoverRecentFYFiles: %v", err)
	}
	for _, f := range got {
		if !strings.Contains(f.URL, "h1b_datahubexport-") {
			t.Errorf("discoverRecentFYFiles returned a non-data-hub link: %+v", f)
		}
	}
}

func TestDiscoverRecentFYFiles_ErrorsWhenFewerThanNYearsAreListed(t *testing.T) {
	if _, err := discoverRecentFYFiles([]byte(indexPageFixture), 10); err == nil {
		t.Error("discoverRecentFYFiles accepted an index with fewer than n years listed")
	}
}

func TestDiscoverRecentFYFiles_ErrorsOnAPageWithNoYearLinks(t *testing.T) {
	if _, err := discoverRecentFYFiles([]byte("<html><body>nothing here</body></html>"), 5); err == nil {
		t.Error("discoverRecentFYFiles accepted a page with no fiscal-year links")
	}
}

func csvFY2023Fixture() string {
	return "\"Fiscal Year\",Employer,\"Initial Approval\",\"Initial Denial\",\"Continuing Approval\",\"Continuing Denial\",NAICS,\"Tax ID\",State,City,ZIP\n" +
		"2023,,1,0,0,0,51,8070,DE,WILMINGTON,19801\n" +
		`2023,"1 800 CONTACTS INC",0,0,1,0,42,1643,UT,DRAPER,84020` + "\n" +
		`2023,"101 SMOKE SHOP 3 LLC",1,0,0,0,45,3465,FL,GAINESVILLE,32606` + "\n" +
		`2023,"REJECTED PETITIONER LLC",0,3,0,0,54,9999,NY,"NEW YORK",10016` + "\n"
}

func TestParseUSH1BSponsorsCSV_KeepsEmployersWithAtLeastOneApproval(t *testing.T) {
	got, err := ParseUSH1BSponsorsCSV([]byte(csvFY2023Fixture()))
	if err != nil {
		t.Fatalf("ParseUSH1BSponsorsCSV: %v", err)
	}
	want := []Record{
		{Name: "1 800 CONTACTS INC", Meta: map[string]string{"tin4": "1643"}},
		{Name: "101 SMOKE SHOP 3 LLC", Meta: map[string]string{"tin4": "3465"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseUSH1BSponsorsCSV =\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseUSH1BSponsorsCSV_SkipsABlankEmployerRow(t *testing.T) {
	got, err := ParseUSH1BSponsorsCSV([]byte(csvFY2023Fixture()))
	if err != nil {
		t.Fatalf("ParseUSH1BSponsorsCSV: %v", err)
	}
	for _, r := range got {
		if r.Name == "" {
			t.Errorf("ParseUSH1BSponsorsCSV kept a blank-employer row: %+v", got)
		}
	}
}

func TestParseUSH1BSponsorsCSV_DropsADenialOnlyEmployer(t *testing.T) {
	got, err := ParseUSH1BSponsorsCSV([]byte(csvFY2023Fixture()))
	if err != nil {
		t.Fatalf("ParseUSH1BSponsorsCSV: %v", err)
	}
	for _, r := range got {
		if r.Name == "REJECTED PETITIONER LLC" {
			t.Errorf("ParseUSH1BSponsorsCSV kept a denial-only employer: %+v", r)
		}
	}
}

func TestParseUSH1BSponsorsCSV_RejectsAnEmptyParse(t *testing.T) {
	header := "\"Fiscal Year\",Employer,\"Initial Approval\",\"Initial Denial\",\"Continuing Approval\",\"Continuing Denial\",NAICS,\"Tax ID\",State,City,ZIP\n"
	if _, err := ParseUSH1BSponsorsCSV([]byte(header)); err == nil {
		t.Error("ParseUSH1BSponsorsCSV accepted a file with no approved rows")
	}
}

func TestParseUSH1BSponsorsCSV_RejectsAMissingColumn(t *testing.T) {
	if _, err := ParseUSH1BSponsorsCSV([]byte("Employer,NAICS\nAcme,51")); err == nil {
		t.Error("ParseUSH1BSponsorsCSV accepted a CSV with no approval columns")
	}
}

// olderFYHeaderFixture mirrors the real FY2019 file's header, which pluralizes the
// approval/denial columns ("Initial Approvals" vs FY2023's "Initial Approval") —
// discovered by running the parser against the live file during the task 5.2
// measurement, not assumed from the FY2023 sample alone. The blank Tax ID is copied
// verbatim from that live row, not a placeholder — the reason USCIS leaves it blank
// for some rows is not established here.
func TestParseUSH1BSponsorsCSV_AcceptsTheOlderPluralColumnNames(t *testing.T) {
	csv := `"Fiscal Year","Employer","Initial Approvals","Initial Denials","Continuing Approvals","Continuing Denials","NAICS","Tax ID","State","City","ZIP"` + "\n" +
		`"2019","GESTAMP ALABAMA LLC","1","0","0","0","33","","AL","MC CALLA","35111"`
	got, err := ParseUSH1BSponsorsCSV([]byte(csv))
	if err != nil {
		t.Fatalf("ParseUSH1BSponsorsCSV: %v", err)
	}
	want := []Record{{Name: "GESTAMP ALABAMA LLC", Meta: map[string]string{"tin4": ""}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseUSH1BSponsorsCSV =\n%+v\nwant\n%+v", got, want)
	}
}

func TestFetchUSH1BSponsors_CombinesAllFiveYears(t *testing.T) {
	years := []string{"2023", "2022", "2021", "2020", "2019"}
	mux := http.NewServeMux()
	mux.HandleFunc("/archive/h-1b-employer-data-hub-files", func(w http.ResponseWriter, r *http.Request) {
		var b strings.Builder
		for _, y := range years {
			b.WriteString(`<a href="/sites/default/files/document/data/h1b_datahubexport-` + y + `.csv">FY ` + y + ` H-1B Employer Data</a>`)
		}
		_, _ = w.Write([]byte(b.String()))
	})
	for _, y := range years {
		y := y
		mux.HandleFunc("/sites/default/files/document/data/h1b_datahubexport-"+y+".csv", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("\"Fiscal Year\",Employer,\"Initial Approval\",\"Initial Denial\",\"Continuing Approval\",\"Continuing Denial\",NAICS,\"Tax ID\",State,City,ZIP\n" +
				y + `,"ACME ROBOTICS INC",1,0,0,0,54,1234,CA,SAN JOSE,95110` + "\n"))
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got, err := fetchUSH1BSponsors(context.Background(), srv.Client(), srv.URL+"/archive/h-1b-employer-data-hub-files")
	if err != nil {
		t.Fatalf("fetchUSH1BSponsors: %v", err)
	}
	if len(got) != len(years) {
		t.Fatalf("fetchUSH1BSponsors returned %d records, want %d", len(got), len(years))
	}
	for _, r := range got {
		if r.Name != "ACME ROBOTICS INC" || r.Meta["tin4"] != "1234" {
			t.Errorf("unexpected record: %+v", r)
		}
	}
}

func TestFetchUSH1BSponsors_SendsABrowserUserAgent(t *testing.T) {
	// uscis.gov's bot filter 403s a request with no User-Agent (confirmed against
	// the live site, and against the production import-collections run on
	// 2026-08-05) but 200s an identical request carrying a browser-like one. This
	// test pins that every request fetchUSH1BSponsors makes carries one.
	years := []string{"2023", "2022", "2021", "2020", "2019"}
	requireUA := func(w http.ResponseWriter, r *http.Request, body func()) {
		// Go's http.Client sends "Go-http-client/1.1" when the caller never sets
		// the header — exactly the fingerprint uscis.gov's bot filter 403s. Only a
		// caller-set, non-default value proves the fix, so this rejects that
		// default the same way the live site does.
		if ua := r.Header.Get("User-Agent"); ua == "" || strings.HasPrefix(ua, "Go-http-client") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		body()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/archive/h-1b-employer-data-hub-files", func(w http.ResponseWriter, r *http.Request) {
		requireUA(w, r, func() {
			var b strings.Builder
			for _, y := range years {
				b.WriteString(`<a href="/sites/default/files/document/data/h1b_datahubexport-` + y + `.csv">FY ` + y + ` H-1B Employer Data</a>`)
			}
			_, _ = w.Write([]byte(b.String()))
		})
	})
	for _, y := range years {
		y := y
		mux.HandleFunc("/sites/default/files/document/data/h1b_datahubexport-"+y+".csv", func(w http.ResponseWriter, r *http.Request) {
			requireUA(w, r, func() {
				_, _ = w.Write([]byte("\"Fiscal Year\",Employer,\"Initial Approval\",\"Initial Denial\",\"Continuing Approval\",\"Continuing Denial\",NAICS,\"Tax ID\",State,City,ZIP\n" +
					y + `,"ACME ROBOTICS INC",1,0,0,0,54,1234,CA,SAN JOSE,95110` + "\n"))
			})
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if _, err := fetchUSH1BSponsors(context.Background(), srv.Client(), srv.URL+"/archive/h-1b-employer-data-hub-files"); err != nil {
		t.Fatalf("fetchUSH1BSponsors: %v (expected a User-Agent to be sent, avoiding the 403)", err)
	}
}

func TestFetchUSH1BSponsors_FailsWholeCallWhenOneYearIsUnreachable(t *testing.T) {
	years := []string{"2023", "2022", "2021", "2020", "2019"}
	mux := http.NewServeMux()
	mux.HandleFunc("/archive/h-1b-employer-data-hub-files", func(w http.ResponseWriter, r *http.Request) {
		var b strings.Builder
		for _, y := range years {
			b.WriteString(`<a href="/sites/default/files/document/data/h1b_datahubexport-` + y + `.csv">FY ` + y + ` H-1B Employer Data</a>`)
		}
		_, _ = w.Write([]byte(b.String()))
	})
	for _, y := range years {
		y := y
		mux.HandleFunc("/sites/default/files/document/data/h1b_datahubexport-"+y+".csv", func(w http.ResponseWriter, r *http.Request) {
			if y == "2021" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte("\"Fiscal Year\",Employer,\"Initial Approval\",\"Initial Denial\",\"Continuing Approval\",\"Continuing Denial\",NAICS,\"Tax ID\",State,City,ZIP\n" +
				y + `,"ACME ROBOTICS INC",1,0,0,0,54,1234,CA,SAN JOSE,95110` + "\n"))
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if _, err := fetchUSH1BSponsors(context.Background(), srv.Client(), srv.URL+"/archive/h-1b-employer-data-hub-files"); err == nil {
		t.Error("fetchUSH1BSponsors returned no error when one of five years failed to fetch")
	}
}

func TestFetchUSH1BSponsors_FailsWholeCallWhenFewerThanFiveYearsAreListed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/archive/h-1b-employer-data-hub-files", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="/sites/default/files/document/data/h1b_datahubexport-2023.csv">FY 2023 H-1B Employer Data</a>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if _, err := fetchUSH1BSponsors(context.Background(), srv.Client(), srv.URL+"/archive/h-1b-employer-data-hub-files"); err == nil {
		t.Error("fetchUSH1BSponsors returned no error when the index listed only one year")
	}
}
