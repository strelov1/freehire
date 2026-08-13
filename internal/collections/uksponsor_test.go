package collections

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

const testCSVURL = "https://assets.publishing.service.gov.uk/media/abc/SP_-_Worker_and_Temporary_Worker_Web_Register_-_2026-06-30.csv"

// contentAPIPayload mirrors the shape of the real GOV.UK Content API response for
// the sponsor-register publication.
const contentAPIPayload = `{
  "details": {
    "attachments": [
      {"title": "Sponsor guidance", "url": "https://x/guide.pdf", "content_type": "application/pdf"},
      {"title": "Register of Worker and Temporary Worker licensed sponsors",
       "url": "` + testCSVURL + `", "content_type": "text/csv"}
    ]
  }
}`

func TestPickSponsorCSV_SelectsTheRegisterAttachment(t *testing.T) {
	got, err := pickSponsorCSV([]byte(contentAPIPayload))
	if err != nil {
		t.Fatalf("pickSponsorCSV: %v", err)
	}
	if got != testCSVURL {
		t.Errorf("pickSponsorCSV = %q, want %q", got, testCSVURL)
	}
}

func TestPickSponsorCSV_IgnoresUnrelatedAttachments(t *testing.T) {
	// A student-sponsor CSV sits in the same publication family and must not be
	// mistaken for the worker register.
	payload := `{"details":{"attachments":[
	  {"title":"Register of student sponsors","url":"https://x/students.csv","content_type":"text/csv"},
	  {"title":"Sponsor guidance","url":"https://x/guide.pdf","content_type":"application/pdf"}
	]}}`
	if _, err := pickSponsorCSV([]byte(payload)); err == nil {
		t.Error("pickSponsorCSV accepted an unrelated attachment")
	}
}

func TestPickSponsorCSV_RejectsUnparseablePayloads(t *testing.T) {
	for _, payload := range []string{"<html>not json</html>", "{}", `{"details":{}}`} {
		if _, err := pickSponsorCSV([]byte(payload)); err == nil {
			t.Errorf("pickSponsorCSV accepted %q", payload)
		}
	}
}

func TestScrapeSponsorCSV_FindsTheRegisterLink(t *testing.T) {
	html := `<a href="` + testCSVURL + `">Register of Worker and Temporary Worker licensed sponsors</a>`
	got, err := scrapeSponsorCSV([]byte(html))
	if err != nil {
		t.Fatalf("scrapeSponsorCSV: %v", err)
	}
	if got != testCSVURL {
		t.Errorf("scrapeSponsorCSV = %q, want %q", got, testCSVURL)
	}
}

func TestScrapeSponsorCSV_IgnoresOtherCSVLinks(t *testing.T) {
	// The publication page also links a student register and an on-site CSV preview;
	// neither is the file we want.
	html := `
	  <a href="https://assets.publishing.service.gov.uk/media/1/Student_Sponsor_Register.csv">Students</a>
	  <a href="/csv-preview/1/SP_-_Worker_and_Temporary_Worker_Web_Register.csv">View online</a>`
	if _, err := scrapeSponsorCSV([]byte(html)); err == nil {
		t.Error("scrapeSponsorCSV accepted a non-register link")
	}
}

func TestResolveUKSponsorCSV_PrefersTheContentAPI(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(contentAPIPayload))
	}))
	defer srv.Close()

	got, err := resolveUKSponsorCSV(context.Background(), srv.Client(), srv.URL+"/api", srv.URL+"/page")
	if err != nil {
		t.Fatalf("resolveUKSponsorCSV: %v", err)
	}
	if got != testCSVURL {
		t.Errorf("resolved %q, want %q", got, testCSVURL)
	}
	if len(paths) != 1 || paths[0] != "/api" {
		t.Errorf("requests = %v, want the API alone", paths)
	}
}

func TestResolveUKSponsorCSV_FallsBackToTheHTMLPage(t *testing.T) {
	cases := []struct {
		name string
		api  func(http.ResponseWriter)
	}{
		{"api errors", func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) }},
		{"api returns non-json", func(w http.ResponseWriter) { _, _ = w.Write([]byte("<html>nope</html>")) }},
		{"api omits the attachment", func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"details":{"attachments":[]}}`)) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/api") {
					tc.api(w)
					return
				}
				_, _ = w.Write([]byte(`<a href="` + testCSVURL + `">Register of Worker and Temporary Worker licensed sponsors</a>`))
			}))
			defer srv.Close()

			got, err := resolveUKSponsorCSV(context.Background(), srv.Client(), srv.URL+"/api", srv.URL+"/page")
			if err != nil {
				t.Fatalf("resolveUKSponsorCSV: %v", err)
			}
			if got != testCSVURL {
				t.Errorf("resolved %q, want %q", got, testCSVURL)
			}
		})
	}
}

func TestResolveUKSponsorCSV_ErrorsWhenNeitherYieldsAURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := resolveUKSponsorCSV(context.Background(), srv.Client(), srv.URL+"/api", srv.URL+"/page"); err == nil {
		t.Error("resolveUKSponsorCSV returned no error when both sources failed")
	}
}

func TestParseUKSponsors_ReadsTheRegisterColumns(t *testing.T) {
	csv := "Organisation Name,Town/City,County,Type & Rating,Route\n" +
		`ACME Robotics Ltd,London,Greater London,"Worker (A rating)",Skilled Worker` + "\n" +
		`ACME Robotics Ltd,London,Greater London,"Worker (A rating)",Scale-up` + "\n" +
		`Green Fields Farm Ltd,Boston,Lincolnshire,"Temporary Worker (A rating)",Temporary Worker - Seasonal Worker` + "\n" +
		`,Nowhere,,"Worker (A rating)",Skilled Worker`

	got, err := ParseUKSponsors([]byte(csv))
	if err != nil {
		t.Fatalf("ParseUKSponsors: %v", err)
	}
	want := []Record{
		{Name: "ACME Robotics Ltd", Meta: map[string]string{"town": "London", "rating": "Worker (A rating)", "route": "Skilled Worker"}},
		{Name: "ACME Robotics Ltd", Meta: map[string]string{"town": "London", "rating": "Worker (A rating)", "route": "Scale-up"}},
		{Name: "Green Fields Farm Ltd", Meta: map[string]string{"town": "Boston", "rating": "Temporary Worker (A rating)", "route": "Temporary Worker - Seasonal Worker"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseUKSponsors =\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseUKSponsors_LocatesColumnsByHeader(t *testing.T) {
	// An upstream column reorder must not silently read the wrong field.
	csv := "Route,Organisation Name,Town/City,County,Type & Rating\n" +
		"Skilled Worker,ACME Robotics Ltd,London,Greater London,Worker (A rating)"
	got, err := ParseUKSponsors([]byte(csv))
	if err != nil {
		t.Fatalf("ParseUKSponsors: %v", err)
	}
	if len(got) != 1 || got[0].Name != "ACME Robotics Ltd" || got[0].Meta["route"] != "Skilled Worker" {
		t.Errorf("reordered columns misread: %+v", got)
	}
}

func TestParseUKSponsors_RejectsAMissingColumn(t *testing.T) {
	if _, err := ParseUKSponsors([]byte("Town/City,Route\nLondon,Skilled Worker")); err == nil {
		t.Error("ParseUKSponsors accepted a CSV with no organisation column")
	}
}

func TestParseUKSponsors_RejectsAnEmptyRegister(t *testing.T) {
	// A successful fetch that parses to nothing is a broken source, not an empty
	// register — returning no rows would reconcile the credential off every company.
	if _, err := ParseUKSponsors([]byte("Organisation Name,Town/City,County,Type & Rating,Route\n")); err == nil {
		t.Error("ParseUKSponsors accepted a register with no rows")
	}
}
