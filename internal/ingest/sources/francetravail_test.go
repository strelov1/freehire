package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// francetravailFake is the API: the token endpoint, and a search over a synthetic département
// whose offers are spread over time, answering the documented way — a page of at most 150
// offers, newest first, a typeContrat aggregation summing to the match count (unless
// noTotals), a 204 for an empty window, and a 400 for a range past the 1,150-result window.
type francetravailFake struct {
	offers   []francetravailOffer
	noTotals bool
	// refuseToken answers 401 to the first search made with this token.
	refuseToken string

	mu       sync.Mutex
	tokens   int
	searches []url.Values
	auths    []string
}

func (f *francetravailFake) PostFormJSON(_ context.Context, u string, v url.Values, out any) error {
	if u != francetravailTokenURL || v.Get("grant_type") != "client_credentials" || v.Get("scope") != francetravailScope ||
		v.Get("client_id") != "id" || v.Get("client_secret") != "secret" {
		return &StatusError{Method: "POST", Code: 400, URL: u}
	}
	f.mu.Lock()
	f.tokens++
	n := f.tokens
	f.mu.Unlock()
	return json.Unmarshal([]byte(fmt.Sprintf(`{"access_token":"t%d","token_type":"Bearer","expires_in":1499}`, n)), out)
}

func (f *francetravailFake) GetJSONOrNoContent(_ context.Context, raw string, headers map[string]string, out any) (bool, error) {
	u, err := url.Parse(raw)
	if err != nil || !strings.HasPrefix(raw, francetravailSearchURL+"?") {
		return false, fmt.Errorf("unexpected url %q", raw)
	}
	q := u.Query()
	f.mu.Lock()
	f.searches = append(f.searches, q)
	f.auths = append(f.auths, headers["Authorization"])
	refuse := f.refuseToken != "" && headers["Authorization"] == "Bearer "+f.refuseToken
	if refuse {
		f.refuseToken = ""
	}
	f.mu.Unlock()
	if refuse {
		return false, &StatusError{Method: "GET", Code: 401, URL: raw}
	}
	var lo, hi int
	if _, err := fmt.Sscanf(q.Get("range"), "%d-%d", &lo, &hi); err != nil || lo > 1000 || hi > 1149 || hi < lo {
		return false, &StatusError{Method: "GET", Code: 400, URL: raw}
	}
	var match []francetravailOffer
	from, to := q.Get("minCreationDate"), q.Get("maxCreationDate")
	for _, o := range f.offers {
		if from != "" && (o.Created < from || o.Created >= to) {
			continue
		}
		match = append(match, o)
	}
	sort.Slice(match, func(i, j int) bool { return match[i].Created > match[j].Created })
	if lo >= len(match) {
		return false, nil
	}
	page := match[lo:min(hi+1, len(match))]
	body := map[string]any{"resultats": page}
	if !f.noTotals {
		body["filtresPossibles"] = []any{map[string]any{"filtre": "typeContrat", "agregation": []any{
			map[string]any{"valeurPossible": "CDI", "nbResultats": len(match) / 2},
			map[string]any{"valeurPossible": "CDD", "nbResultats": len(match) - len(match)/2},
		}}}
	}
	b, _ := json.Marshal(body)
	return true, json.Unmarshal(b, out)
}

// francetravailSynthetic builds n offers, one every 37 minutes back from 2026-10-28.
func francetravailSynthetic(n int) []francetravailOffer {
	end := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	out := make([]francetravailOffer, n)
	for i := range out {
		o := &out[i]
		o.ID = "SYN" + strconv.Itoa(i)
		o.Title = "Offre " + strconv.Itoa(i)
		o.Description = "Description"
		o.Employer.Name = "EMPLOYEUR"
		o.Created = end.Add(-time.Duration(i) * 37 * time.Minute).Format("2006-01-02T15:04:05.000Z")
	}
	return out
}

func newTestFranceTravail(f *francetravailFake) francetravail {
	s := NewFranceTravail(f, "id", "secret").(francetravail)
	s.limiter = noWait{}
	s.now = func() time.Time { return time.Date(2026, 10, 28, 13, 0, 0, 0, time.UTC) }
	s.floor = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return s
}

func TestFranceTravailMapsTheDocumentedOffer(t *testing.T) {
	raw, err := os.ReadFile("testdata/francetravail/search.json")
	if err != nil {
		t.Fatal(err)
	}
	var p francetravailPage
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	f := &francetravailFake{offers: p.Results}
	jobs, err := newTestFranceTravail(f).Fetch(context.Background(), CompanyEntry{Board: "75"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want two (the offer withholding its employer is dropped): %+v", len(jobs), jobs)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	d := byID["198XKLM"]
	if d.Title != "Développeur / Développeuse web (H/F)" || d.Company != "ACME LOGICIELS" {
		t.Errorf("title/company = %q / %q", d.Title, d.Company)
	}
	if d.URL != francetravailOfferURL+"198XKLM" || d.Location != "PARIS 11" || len(d.Countries) != 1 || d.Countries[0] != "fr" {
		t.Errorf("url %q location %q countries %v", d.URL, d.Location, d.Countries)
	}
	if !strings.Contains(d.Description, "<li>") && !strings.Contains(d.Description, "<p>") {
		t.Errorf("plain-text body not rebuilt: %q", d.Description)
	}
	if !d.IsTechHint || d.EmploymentType != "full_time" || d.ExperienceYearsMin == nil || *d.ExperienceYearsMin != 3 {
		t.Errorf("tech %v employment %q experience %v", d.IsTechHint, d.EmploymentType, d.ExperienceYearsMin)
	}
	if d.SalaryMin == nil || *d.SalaryMin != 42000 || *d.SalaryMax != 48000 || d.SalaryPeriod != "year" || d.SalaryCurrency != "EUR" {
		t.Errorf("salary = %v-%v %q %q", d.SalaryMin, d.SalaryMax, d.SalaryCurrency, d.SalaryPeriod)
	}
	if d.PostedAt == nil || d.PostedAt.UTC().Format("2006-01-02") != "2026-10-27" {
		t.Errorf("posted at %v", d.PostedAt)
	}
	m := byID["198XKLN"]
	if m.EmploymentType != "contract" || m.IsTechHint || m.ExperienceYearsMin != nil {
		t.Errorf("interim offer: employment %q tech %v experience %v", m.EmploymentType, m.IsTechHint, m.ExperienceYearsMin)
	}
	if m.SalaryMin == nil || *m.SalaryMin != 12 || *m.SalaryMax != 12 || m.SalaryPeriod != "hour" {
		t.Errorf("hourly salary = %v-%v %q", m.SalaryMin, m.SalaryMax, m.SalaryPeriod)
	}
	q := f.searches[0]
	if q.Get("departement") != "75" || q.Get("origineOffre") != "1" || q.Get("range") != "0-149" || q.Get("minCreationDate") != "" {
		t.Errorf("first search = %v", q)
	}
	if f.auths[0] != "Bearer t1" || f.tokens != 1 {
		t.Errorf("auth %q after %d token requests", f.auths[0], f.tokens)
	}
}

// A département matching more than the 1,150-result window is read in creation-date windows
// that each fit inside it, and every offer is read exactly once.
func TestFranceTravailSlicesPastTheResultWindow(t *testing.T) {
	for _, noTotals := range []bool{false, true} {
		f := &francetravailFake{offers: francetravailSynthetic(3000), noTotals: noTotals}
		jobs, err := newTestFranceTravail(f).Fetch(context.Background(), CompanyEntry{Board: "75"})
		if err != nil {
			t.Fatal(err)
		}
		if len(jobs) != 3000 {
			t.Errorf("noTotals=%v: read %d of 3000 offers", noTotals, len(jobs))
		}
		ids := map[string]bool{}
		for _, j := range jobs {
			if ids[j.ExternalID] {
				t.Errorf("offer %s read twice", j.ExternalID)
			}
			ids[j.ExternalID] = true
		}
		if f.tokens != 1 {
			t.Errorf("noTotals=%v: %d token requests in one run, want the cached one", noTotals, f.tokens)
		}
	}
}

func TestFranceTravailRefreshesARefusedToken(t *testing.T) {
	f := &francetravailFake{offers: francetravailSynthetic(10), refuseToken: "t1"}
	jobs, err := newTestFranceTravail(f).Fetch(context.Background(), CompanyEntry{Board: "2A"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 10 || f.tokens != 2 {
		t.Errorf("read %d offers with %d tokens, want 10 with a second token after the 401", len(jobs), f.tokens)
	}
}

func TestFranceTravailRejectsABoardThatIsNotADepartement(t *testing.T) {
	for _, b := range []string{"", "Paris", "99", "975x", "00"} {
		if _, err := newTestFranceTravail(&francetravailFake{}).Fetch(context.Background(), CompanyEntry{Board: b}); err == nil {
			t.Errorf("board %q accepted", b)
		}
	}
	for _, b := range []string{"01", "75", "2a", "971", "95"} {
		if !francetravailDepartement.MatchString(strings.ToUpper(b)) {
			t.Errorf("board %q refused", b)
		}
	}
}

func TestFranceTravailSalary(t *testing.T) {
	for in, want := range map[string]string{
		`{"libelle":"Mensuel de 1800.00 Euros à 2000.00 Euros sur 12 mois"}`: "1800-2000 month",
		`{"libelle":"Annuel de 26400,00 Euros sur 12 mois"}`:                 "26400-26400 year",
		`{"libelle":"Mensuel de 2000.00 Euros à 1800.00 Euros"}`:             "",
		`{"commentaire":"selon profil"}`:                                     "",
		`[]`:                                                                 "",
	} {
		lo, hi, period, ok := francetravailSalary(json.RawMessage(in))
		got := ""
		if ok {
			got = fmt.Sprintf("%d-%d %s", lo, hi, period)
		}
		if got != want {
			t.Errorf("francetravailSalary(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestFranceTravailIsRegisteredOnlyWithItsCredentials(t *testing.T) {
	t.Setenv("FRANCE_TRAVAIL_CLIENT_ID", "")
	t.Setenv("FRANCE_TRAVAIL_CLIENT_SECRET", "")
	if _, ok := All(NewClient())["francetravail"]; ok {
		t.Error("registered with no credentials")
	}
	if _, ok := Taxonomy()["francetravail"]; !ok {
		t.Error("missing from the taxonomy")
	}
	t.Setenv("FRANCE_TRAVAIL_CLIENT_ID", "id")
	t.Setenv("FRANCE_TRAVAIL_CLIENT_SECRET", "secret")
	if _, ok := All(NewClient())["francetravail"]; !ok {
		t.Error("not registered with its credentials")
	}
}

// A window that fits under the result window but runs past 1,050 offers is read to its end: the
// last page starts at 1,000, never at 1,050, which the API refuses.
func TestFranceTravailReadsAWindowToItsLastPage(t *testing.T) {
	for _, noTotals := range []bool{false, true} {
		f := &francetravailFake{offers: francetravailSynthetic(1100), noTotals: noTotals}
		jobs, err := newTestFranceTravail(f).Fetch(context.Background(), CompanyEntry{Board: "75"})
		if err != nil {
			t.Fatalf("noTotals=%v: %v", noTotals, err)
		}
		if len(jobs) != 1100 {
			t.Errorf("noTotals=%v: read %d of 1100 offers", noTotals, len(jobs))
		}
	}
}
