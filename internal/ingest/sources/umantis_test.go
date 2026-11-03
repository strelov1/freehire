package sources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// umFake serves canned pages by URL and records the requests.
type umFake struct {
	pages map[string]string
}

func (f *umFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	p, ok := f.pages[u]
	if !ok {
		return nil, errors.New("404")
	}
	return html.Parse(strings.NewReader(p))
}

// umRowHTML renders one listing row the way the table layout does (customer-configured
// columns: place, date, title).
func umRowHTML(id, title, place, date string) string {
	return `<tr class="tableaslist_contentrow1"><td><div class="tableaslist_cell">` +
		`<span class="tableaslist_text tableaslist_element_1152486">Group</span>` +
		`<span class="tableaslist_text tableaslist_element_1152487">&nbsp;|&nbsp;Online since: ` + date + `</span>` +
		`<span class="tableaslist_subtitle tableaslist_element_1152488"><a href="/Vacancies/` + id +
		`/Description/2" class="HSTableLinkSubTitle">` + title + `</a></span>` +
		`<span class="tableaslist_subtitle tableaslist_element_1152495">&nbsp;|&nbsp;` + place + `</span>` +
		`<a href="/Vacancies/` + id + `/Application/CheckLogin/2?v4login=1">Apply now</a>` +
		`</div></td></tr>`
}

func umListing(next string, rows ...string) string {
	attr := ""
	if next != "" {
		attr = ` data-pagination-next-href="` + next + `#connectortable_1"`
	}
	return `<html><body><span class="tableaslist_paging_top"` + attr + `></span><table>` +
		strings.Join(rows, "") + `</table></body></html>`
}

const umDetail = `<html><head><title>x</title><script>var a=1;</script></head><body>` +
	`<header><img src="/logo"></header><div class="content"><h1>Role</h1><p>Do the <b>work</b>.</p></div>` +
	`<form><input></form><script>track()</script></body></html>`

func TestUmantisProvider(t *testing.T) {
	if got := NewUmantis(nil).Provider(); got != "umantis" {
		t.Errorf("Provider() = %q, want umantis", got)
	}
}

func TestUmantisRegisteredInAll(t *testing.T) {
	if _, ok := All(nil)["umantis"]; !ok {
		t.Fatal("All() missing provider umantis")
	}
}

func TestUmantisFetchPaginatesAndMaps(t *testing.T) {
	const host = "jobs.example.com"
	f := &umFake{pages: map[string]string{
		"https://" + host + "/Jobs/All": umListing("?tc1=p2&_search_token1=9",
			umRowHTML("9307", "Event Manager", "Switzerland (Mex)", "23.10.2026"),
			umRowHTML("9205", "Care Agent", "Ort: M&uuml;nchen", "14.10.2026")),
		"https://" + host + "/Jobs/All?tc1=p2&_search_token1=9": umListing("",
			umRowHTML("9205", "Care Agent", "Ort: München", "14.10.2026"), // repeat → one posting
			umRowHTML("8728", "Draftsman", "Remote", "bad")),
		"https://" + host + "/Vacancies/9307/Description/2": umDetail,
		"https://" + host + "/Vacancies/9205/Description/2": umDetail,
		"https://" + host + "/Vacancies/8728/Description/2": umDetail,
	}}
	jobs, err := NewUmantis(f).Fetch(context.Background(), CompanyEntry{Board: host, Company: "Example"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3: %+v", len(jobs), jobs)
	}
	j := jobs[0]
	if j.ExternalID != "9307" || j.Title != "Event Manager" || j.Company != "Example" ||
		j.Location != "Switzerland (Mex)" || j.URL != "https://"+host+"/Vacancies/9307/Description/2" {
		t.Errorf("job 0 = %+v", j)
	}
	if want := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC); j.PostedAt == nil || !j.PostedAt.Equal(want) {
		t.Errorf("PostedAt = %v, want %v", j.PostedAt, want)
	}
	if !strings.Contains(j.Description, "<b>work</b>") || strings.Contains(j.Description, "track()") ||
		strings.Contains(j.Description, "logo") {
		t.Errorf("Description = %q", j.Description)
	}
	if jobs[1].Location != "München" {
		t.Errorf("label not stripped: %q", jobs[1].Location)
	}
	if !jobs[2].Remote || jobs[2].PostedAt != nil {
		t.Errorf("job 2 remote=%v postedAt=%v, want remote with no date", jobs[2].Remote, jobs[2].PostedAt)
	}
}

func TestUmantisMarksUnreadableDetail(t *testing.T) {
	const host = "recruitingapp-1.umantis.com"
	f := &umFake{pages: map[string]string{
		"https://" + host + "/Jobs/All": umListing("",
			umRowHTML("1", "A", "X", "01.01.2026"), umRowHTML("2", "B", "Y", "01.01.2026")),
		"https://" + host + "/Vacancies/2/Description/2": umDetail,
	}}
	jobs, err := NewUmantis(f).Fetch(context.Background(), CompanyEntry{Board: host})
	if err != nil || len(jobs) != 2 || !jobs[0].Unreadable || jobs[0].ExternalID != "1" || jobs[1].Unreadable {
		t.Fatalf("jobs = %+v, err = %v", jobs, err)
	}
}

func TestUmantisRowWithoutVacancyIDIsSkipped(t *testing.T) {
	const host = "recruitingapp-1.umantis.com"
	f := &umFake{pages: map[string]string{
		"https://" + host + "/Jobs/All": `<html><body><table><tr><td>` +
			`<a href="/Vacancies/abc/Description/1">bad</a><a href="/Jobs/All">all</a></td></tr></table></body></html>`,
	}}
	jobs, err := NewUmantis(f).Fetch(context.Background(), CompanyEntry{Board: host})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs = %+v, err = %v", jobs, err)
	}
}

func TestUmantisLaterPageFailureKeepsGathered(t *testing.T) {
	const host = "recruitingapp-1.umantis.com"
	f := &umFake{pages: map[string]string{
		"https://" + host + "/Jobs/All":                  umListing("?tc1=p2", umRowHTML("1", "A", "X", "01.01.2026")),
		"https://" + host + "/Vacancies/1/Description/2": umDetail,
	}}
	jobs, err := NewUmantis(f).Fetch(context.Background(), CompanyEntry{Board: host})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs = %+v, err = %v", jobs, err)
	}
}

func TestUmantisAllDetailsUnreadableFails(t *testing.T) {
	const host = "recruitingapp-1.umantis.com"
	f := &umFake{pages: map[string]string{
		"https://" + host + "/Jobs/All": umListing("", umRowHTML("1", "A", "X", "01.01.2026")),
	}}
	if _, err := NewUmantis(f).Fetch(context.Background(), CompanyEntry{Board: host}); err == nil {
		t.Error("a crawl that read none of its listed postings must fail")
	}
}

func TestUmantisFirstPageFailureAndBadBoard(t *testing.T) {
	if _, err := NewUmantis(&umFake{}).Fetch(context.Background(), CompanyEntry{Board: "recruitingapp-1.umantis.com"}); err == nil {
		t.Error("listing failure: want error")
	}
	for _, b := range []string{"", "host/path", "a b", "x?y"} {
		if _, err := NewUmantis(&umFake{}).Fetch(context.Background(), CompanyEntry{Board: b}); err == nil {
			t.Errorf("board %q: want error", b)
		}
	}
}

func TestUmantisPageCapStopsEndlessPagination(t *testing.T) {
	const host = "recruitingapp-1.umantis.com"
	f := &umFake{pages: map[string]string{}}
	for i := 1; i <= umantisMaxPages+5; i++ {
		u := "https://" + host + "/Jobs/All"
		if i > 1 {
			u += fmt.Sprintf("?p=%d", i)
		}
		f.pages[u] = umListing(fmt.Sprintf("?p=%d", i+1), umRowHTML(fmt.Sprint(i), "T", "X", "01.01.2026"))
		f.pages[fmt.Sprintf("https://%s/Vacancies/%d/Description/2", host, i)] = umDetail
	}
	jobs, err := NewUmantis(f).Fetch(context.Background(), CompanyEntry{Board: host})
	if err != nil || len(jobs) != umantisMaxPages {
		t.Fatalf("got %d jobs, err %v; want %d", len(jobs), err, umantisMaxPages)
	}
}
