package sources

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

const ivHost = "autogrill.intervieweb.it"
const ivEndpoint = "https://autogrill.intervieweb.it/app.php?opmode=guest&module=newcareer&ajax=1&CSRFToken=t"

func ivURL(slug string) string { return "https://" + ivHost + "/jobs/" + slug + "/en/" }

// ivCards renders listing cards the way the platform does: each posting linked twice.
func ivCards(slugs ...string) string {
	var b strings.Builder
	for _, s := range slugs {
		b.WriteString(`<a href="` + ivURL(s) + `"><h3>T</h3></a><a href="` + ivURL(s) + `">more</a>`)
	}
	return b.String()
}

func ivLanding(pages int, slugs ...string) string {
	return `<html><body><script>researchAnnounces = {'section': 'Abc-ST', 'order': 'date'}</script>` +
		`<input type="hidden" id="url-for-announces" value="` + ivEndpoint + `">` +
		`<div>Page 1 of ` + strconv.Itoa(pages) + `</div>` + ivCards(slugs...) + `</body></html>`
}

func ivDetail(title, desc, posted, until string) string {
	return `<html><head><script type="application/ld+json">{"@type":"JobPosting","title":"` + title +
		`","description":"` + desc + `","datePosted":"` + posted + `","validThrough":"` + until +
		`","jobLocation":{"@type":"Place","address":{"addressLocality":"Altivole","addressRegion":"TV","addressCountry":"IT"}}}` +
		`</script></head><body></body></html>`
}

type ivFake struct {
	pages map[string]string // GET url -> body
	posts map[string]string // page number -> JSON body
	gets  []string
	forms []url.Values
}

func (f *ivFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.gets = append(f.gets, u)
	body, ok := f.pages[u]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return html.Parse(strings.NewReader(body))
}

func (f *ivFake) PostFormJSON(_ context.Context, u string, v url.Values, out any) error {
	f.forms = append(f.forms, v)
	body, ok := f.posts[v.Get("page")]
	if !ok || u != ivEndpoint {
		return &StatusError{Method: "POST", Code: 500, URL: u}
	}
	return json.Unmarshal([]byte(body), out)
}

func ivPage(slugs ...string) string {
	b, _ := json.Marshal(map[string]any{"success": true, "data": ivCards(slugs...)})
	return string(b)
}

func TestIntervieweb(t *testing.T) {
	if got := NewIntervieweb(nil).Provider(); got != "intervieweb" {
		t.Errorf("Provider() = %q", got)
	}
}

func TestIVJobID(t *testing.T) {
	cases := map[string]string{
		ivURL("barista-autogrill-altivole-sud-treviso-7395"):   "7395",
		ivURL("categoria-protetta-6899-autogrill-modena-7394"): "7394", // slug carries its own number
		"https://x.intervieweb.it/jobs/a-1/it/":                "1",
		"https://x.intervieweb.it/en/career":                   "",
		"https://x.intervieweb.it/jobs/no-id-here/en/":         "",
	}
	for u, want := range cases {
		if got := ivJobID(u); got != want {
			t.Errorf("ivJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestIntervieweb_FetchPaginatesAndMaps(t *testing.T) {
	f := &ivFake{
		pages: map[string]string{
			"https://" + ivHost + "/en/career": ivLanding(2, "barista-a-1", "cook-b-2"),
			ivURL("barista-a-1"):               ivDetail("BARISTA A", "<p>Body one</p>", "2026-11-02", "2026-12-02"),
			ivURL("cook-b-2"):                  ivDetail("COOK B", "<p>Body two</p>", "2026-11-01", ""),
			ivURL("waiter-c-3"):                ivDetail("WAITER C", "<p>Body three</p>", "2026-10-30", ""),
		},
		// page 2 repeats id 2 (a row shifted between requests) and adds id 3.
		posts: map[string]string{"2": ivPage("cook-b-2", "waiter-c-3")},
	}
	jobs, err := NewIntervieweb(f).Fetch(context.Background(), CompanyEntry{Board: ivHost, Company: "Avolta"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	j := byID["1"]
	if j.Title != "BARISTA A" || j.Company != "Avolta" || j.URL != ivURL("barista-a-1") ||
		j.Location != "Altivole, TV" || len(j.Countries) != 1 || j.Countries[0] != "it" ||
		!strings.Contains(j.Description, "Body one") {
		t.Errorf("job 1 = %+v", j)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-11-02" {
		t.Errorf("PostedAt = %v", j.PostedAt)
	}
	if j.ApplicationDeadline == nil || j.ApplicationDeadline.Format("2006-01-02") != "2026-12-02" {
		t.Errorf("ApplicationDeadline = %v", j.ApplicationDeadline)
	}
	if byID["2"].ApplicationDeadline != nil {
		t.Error("empty validThrough must leave the deadline nil")
	}
	if len(f.forms) != 1 || f.forms[0].Get("section") != "Abc-ST" || f.forms[0].Get("act1") != "vacancyListCareer" {
		t.Errorf("pager form = %v", f.forms)
	}
}

func TestIntervieweb_FetchNewRefreshesSeenWithoutDetail(t *testing.T) {
	f := &ivFake{pages: map[string]string{
		"https://" + ivHost + "/en/career": ivLanding(1, "barista-a-1", "cook-b-2"),
		ivURL("cook-b-2"):                  ivDetail("COOK B", "<p>x</p>", "2026-11-01", ""),
	}}
	jobs, err := NewIntervieweb(f).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Board: ivHost}, func(id string) bool { return id == "1" })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || !jobs[0].SeenRefresh || jobs[0].ExternalID != "1" || jobs[1].SeenRefresh {
		t.Errorf("jobs = %+v", jobs)
	}
	for _, g := range f.gets {
		if g == ivURL("barista-a-1") {
			t.Error("a seen posting must not be fetched")
		}
	}
}

func TestIntervieweb_MarksPostingWithoutJobPostingUnreadable(t *testing.T) {
	f := &ivFake{pages: map[string]string{
		"https://" + ivHost + "/en/career": ivLanding(1, "barista-a-1", "cook-b-2"),
		ivURL("barista-a-1"):               `<html><body>no markup</body></html>`,
		ivURL("cook-b-2"):                  ivDetail("COOK B", "<p>x</p>", "2026-11-01", ""),
	}}
	jobs, err := NewIntervieweb(f).Fetch(context.Background(), CompanyEntry{Board: ivHost})
	if err != nil || len(jobs) != 2 || jobs[0].ExternalID != "1" || !jobs[0].Unreadable || jobs[1].ExternalID != "2" || jobs[1].Unreadable {
		t.Fatalf("a page without a JobPosting must be an unreadable marker, not an absence: jobs = %+v, err = %v", jobs, err)
	}
}

func TestIntervieweb_BoardAndPagerErrors(t *testing.T) {
	for _, board := range []string{"", "a.intervieweb.it/en", "a b"} {
		if _, err := NewIntervieweb(&ivFake{}).Fetch(context.Background(), CompanyEntry{Board: board}); err == nil {
			t.Errorf("board %q: want error", board)
		}
	}
	// A pager stating more pages than it exposes an endpoint for is an error, not a silent truncation.
	noEndpoint := `<html><body><div>Page 1 of 3</div>` + ivCards("a-1") + `</body></html>`
	f := &ivFake{pages: map[string]string{"https://" + ivHost + "/en/career": noEndpoint}}
	if _, err := NewIntervieweb(f).Fetch(context.Background(), CompanyEntry{Board: ivHost}); err == nil {
		t.Error("missing pager endpoint: want error")
	}
	// A failing page POST fails the run rather than returning a partial board.
	f = &ivFake{pages: map[string]string{"https://" + ivHost + "/en/career": ivLanding(2, "a-1")}}
	if _, err := NewIntervieweb(f).Fetch(context.Background(), CompanyEntry{Board: ivHost}); err == nil {
		t.Error("failing page: want error")
	}
	// Listing unreachable.
	if _, err := NewIntervieweb(&ivFake{}).Fetch(context.Background(), CompanyEntry{Board: ivHost}); err == nil {
		t.Error("unreachable listing: want error")
	}
}
