package aggregators

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources/kit"
	"golang.org/x/net/html"
)

const (
	indeedListURL = "https://www.indeed.com/jobs?fromage=1&l=Remote&q=software+engineer&sort=date"
	// The fixture's three cards; the page opened the first.
	indeedOpened   = "b2d02bb2a34565d6"
	indeedApplyKey = "db63e75e96d7539a"
	indeedPlainKey = "9faafa2bebf4b435"
)

type indeedNoWait struct{}

func (indeedNoWait) Wait(context.Context) error { return nil }

type indeedAnswer struct {
	body, final string
	err         error
}

// indeedFake answers by requested URL and records every request.
type indeedFake struct {
	answers  map[string]indeedAnswer
	requests []string
}

func (f *indeedFake) GetHTMLResolved(_ context.Context, raw string) (*html.Node, string, error) {
	f.requests = append(f.requests, raw)
	a, ok := f.answers[raw]
	if !ok {
		return nil, "", fmt.Errorf("unexpected request %s", raw)
	}
	if a.err != nil {
		return nil, "", a.err
	}
	root, err := html.Parse(strings.NewReader(a.body))
	return root, a.final, err
}

func indeedFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/indeed/search_golang_remote.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// indeedOpening is the fixture as the site serves it with another card selected: the same cards,
// the opened posting swapped for key's.
func indeedOpening(page, key, description string) string {
	page = strings.Replace(page, `"jobKey": "`+indeedOpened+`"`, `"jobKey": "`+key+`"`, 1)
	i := strings.Index(page, `"sanitizedJobDescription": "`) + len(`"sanitizedJobDescription": "`)
	j := i + strings.Index(page[i:], `", "jobInfoHeaderModel"`)
	return page[:i] + description + page[j:]
}

func indeedEntry() kit.CompanyEntry {
	return kit.CompanyEntry{Provider: "indeed", Board: "software engineer", Region: "Remote"}
}

func newIndeedFake(t *testing.T) *indeedFake {
	page := indeedFixture(t)
	return &indeedFake{answers: map[string]indeedAnswer{
		indeedListURL: {body: page, final: indeedListURL + "&vjk=" + indeedOpened},
		indeedListURL + "&vjk=" + indeedApplyKey: {
			body:  indeedOpening(page, indeedApplyKey, `<p>Apply-key body</p>`),
			final: indeedListURL + "&vjk=" + indeedApplyKey,
		},
		indeedListURL + "&vjk=" + indeedPlainKey: {
			body:  indeedOpening(page, indeedPlainKey, `<p>Plain-key body</p>`),
			final: indeedListURL + "&vjk=" + indeedPlainKey,
		},
	}}
}

func byKey(jobs []kit.Job) map[string]kit.Job {
	out := map[string]kit.Job{}
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestIndeedReadsEveryCardAndOpensEachOnce(t *testing.T) {
	f := newIndeedFake(t)
	jobs, err := indeed{http: f, pace: indeedNoWait{}}.Fetch(context.Background(), indeedEntry())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}
	// The listing page pays for the card it opened: one navigation per OTHER card.
	if len(f.requests) != 3 {
		t.Errorf("requests = %v, want the listing plus two selections", f.requests)
	}
	got := byKey(jobs)

	first := got[indeedOpened]
	if first.Title != "Senior DevOps Engineer - Tempus" || first.Company != "PNC Financial Services Group" {
		t.Errorf("first card = %q at %q", first.Title, first.Company)
	}
	if first.URL != "https://www.indeed.com/viewjob?jk="+indeedOpened {
		t.Errorf("URL = %q", first.URL)
	}
	if !strings.Contains(first.Description, "At PNC, our people are our greatest differentiator") {
		t.Errorf("description not read from the opened posting: %.120q", first.Description)
	}
	if first.SalaryMin == nil || *first.SalaryMin != 86250 || *first.SalaryMax != 172500 || first.SalaryCurrency != "USD" || first.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %s/%s", first.SalaryMin, first.SalaryMax, first.SalaryCurrency, first.SalaryPeriod)
	}
	if first.WorkMode != "remote" || !first.Remote || first.EmploymentType != "full_time" || first.ApplyMethod != kit.ApplyExternal {
		t.Errorf("first card facets = mode %q remote %v type %q apply %q", first.WorkMode, first.Remote, first.EmploymentType, first.ApplyMethod)
	}
	if len(first.Countries) != 1 || first.Countries[0] != "us" || first.PostedAt == nil {
		t.Errorf("countries %v posted %v", first.Countries, first.PostedAt)
	}

	apply := got[indeedApplyKey]
	if apply.Description != "<p>Apply-key body</p>" {
		t.Errorf("selected card body = %q", apply.Description)
	}
	// Indeed Apply is on-site; "Full-time, Contract" names two types, so none.
	if apply.ApplyMethod != kit.ApplyOnSite || apply.EmploymentType != "" {
		t.Errorf("apply card = apply %q type %q", apply.ApplyMethod, apply.EmploymentType)
	}
	// No remoteWorkModel states no arrangement.
	if plain := got[indeedPlainKey]; plain.WorkMode != "" || plain.Remote {
		t.Errorf("plain card work mode = %q", plain.WorkMode)
	}
}

func TestIndeedRefreshesSeenCardsWithoutOpeningThem(t *testing.T) {
	f := newIndeedFake(t)
	seen := func(id string) bool { return id == indeedApplyKey }
	jobs, err := indeed{http: f, pace: indeedNoWait{}}.FetchNew(context.Background(), indeedEntry(), seen)
	if err != nil {
		t.Fatal(err)
	}
	got := byKey(jobs)
	if j := got[indeedApplyKey]; !j.SeenRefresh || j.Description != "" {
		t.Errorf("seen card = %+v, want an identity-only refresh", j)
	}
	for _, r := range f.requests {
		if strings.HasSuffix(r, indeedApplyKey) {
			t.Errorf("opened a seen card: %s", r)
		}
	}
}

// A page that opened a different posting than the one asked for must not lend it its body.
func TestIndeedRejectsAPageThatOpenedAnotherPosting(t *testing.T) {
	f := newIndeedFake(t)
	wrong := f.answers[indeedListURL+"&vjk="+indeedPlainKey]
	wrong.final = indeedListURL + "&vjk=" + indeedApplyKey
	f.answers[indeedListURL+"&vjk="+indeedPlainKey] = wrong
	jobs, err := indeed{http: f, pace: indeedNoWait{}}.Fetch(context.Background(), indeedEntry())
	if err != nil {
		t.Fatal(err)
	}
	if j := byKey(jobs)[indeedPlainKey]; !j.Unreadable || j.Description != "" {
		t.Errorf("mismatched card = %+v, want unreadable", j)
	}
}

// The sign-in wall is a refusal: it ends the board, keeping what was already read.
func TestIndeedSignInWallEndsTheBoardKeepingProgress(t *testing.T) {
	f := newIndeedFake(t)
	f.answers[indeedListURL+"&vjk="+indeedApplyKey] = indeedAnswer{body: "<html>Sign In</html>", final: "https://secure.indeed.com/auth?from=bot-detection-anonymous"}
	jobs, err := indeed{http: f, pace: indeedNoWait{}}.Fetch(context.Background(), indeedEntry())
	if err != nil {
		t.Fatalf("progress lost: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != indeedOpened {
		t.Errorf("jobs = %v, want only the posting read before the wall", jobs)
	}
	if len(f.requests) != 2 {
		t.Errorf("kept crawling past the wall: %v", f.requests)
	}

	f.answers[indeedListURL] = indeedAnswer{body: "<html>Sign In</html>", final: "https://secure.indeed.com/auth?from=page-two-signin"}
	if _, err := (indeed{http: f, pace: indeedNoWait{}}).Fetch(context.Background(), indeedEntry()); err == nil {
		t.Error("a walled listing must fail the board")
	}
}

func TestIndeedListingShapes(t *testing.T) {
	empty := `<script>window.mosaic.providerData["mosaic-provider-jobcards"]={"metaData":{"mosaicProviderJobCardsModel":{"results":[]}}};</script>`
	f := &indeedFake{answers: map[string]indeedAnswer{indeedListURL: {body: empty, final: indeedListURL}}}
	jobs, err := indeed{http: f, pace: indeedNoWait{}}.Fetch(context.Background(), indeedEntry())
	if err != nil || len(jobs) != 0 {
		t.Errorf("empty search = %v, %v; want no jobs and no error", jobs, err)
	}

	challenge := `<html><title>Security Check - Indeed.com</title></html>`
	f.answers[indeedListURL] = indeedAnswer{body: challenge, final: indeedListURL}
	if _, err := (indeed{http: f, pace: indeedNoWait{}}).Fetch(context.Background(), indeedEntry()); err == nil {
		t.Error("a page without the card model must fail, not read as an empty search")
	}
}

func TestIndeedSearchOptions(t *testing.T) {
	e := indeedEntry()
	e.Tenants = map[string]string{"site": "de", "fromage": "7"}
	s, err := indeedSearchFor(e)
	if err != nil {
		t.Fatal(err)
	}
	if s.list != "https://de.indeed.com/jobs?fromage=7&l=Remote&q=software+engineer&sort=date" || s.postingURL(indeedOpened) != "https://de.indeed.com/viewjob?jk="+indeedOpened {
		t.Errorf("search = %+v", s)
	}
	for _, tenants := range []map[string]string{
		{"site": "de.example"}, {"site": "DE"}, {"fromage": "2"}, {"fromage": "30"},
	} {
		e.Tenants = tenants
		if _, err := indeedSearchFor(e); err == nil {
			t.Errorf("tenants %v accepted", tenants)
		}
	}
	e.Tenants, e.Board = nil, " "
	if _, err := indeedSearchFor(e); err == nil {
		t.Error("an empty keyword accepted")
	}
}

func TestIndeedJobKey(t *testing.T) {
	for id, want := range map[string]string{indeedOpened: indeedOpened, "B2D02BB2A34565D6": "", "b2d02bb2a3456": "", "software:b2d02bb2a34565d6": ""} {
		if got := IndeedJobKey(id); got != want {
			t.Errorf("IndeedJobKey(%q) = %q", id, got)
		}
	}
}
