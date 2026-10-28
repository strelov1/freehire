package sources

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// The fixtures are trimmed from live HiringRoom responses fetched on 2026-10-15: pages of
// stefaniniitsolutionsperu's and embluemarketingcloud's /jobs/getVacanciesForPortal/<page> JSON
// (vacancy cards verbatim, the stated total rewritten to the cards kept), emBlue's vacancy page
// (its <main> verbatim, three description sections) and Stefanini's catch-all "Postulación en
// base" page, whose description section is empty.

const hrStefPage = "https://stefaniniitsolutionsperu.hiringroom.com/jobs/getVacanciesForPortal/"

func hrFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/hiringroom/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func hrCards(t *testing.T, name string) []hiringroomCard {
	t.Helper()
	var p hiringroomPage
	if err := json.Unmarshal([]byte(hrFixture(t, name)), &p); err != nil {
		t.Fatal(err)
	}
	cards, err := parseHiringRoomCards(p.Data.HTMLContent)
	if err != nil {
		t.Fatal(err)
	}
	return cards
}

// hrFake serves pager pages and vacancy pages by URL.
type hrFake struct {
	pages map[string]string
	forms []url.Values
	gets  []string
	// missing is the status a GET for an unserved URL answers; 0 means 500.
	missing int
}

func (f *hrFake) PostFormJSON(_ context.Context, u string, v url.Values, out any) error {
	f.forms = append(f.forms, v)
	body, ok := f.pages[u]
	if !ok {
		return &StatusError{Method: "POST", Code: 500, URL: u}
	}
	return json.Unmarshal([]byte(body), out)
}

func (f *hrFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	f.gets = append(f.gets, u)
	body, ok := f.pages[u]
	if !ok {
		code := f.missing
		if code == 0 {
			code = 500
		}
		return nil, &StatusError{Method: "GET", Code: code, URL: u}
	}
	return html.Parse(strings.NewReader(body))
}

// A tenant whose public portal is off still lists its vacancies through the pager, and every
// vacancy page 404s. That is an answer — nothing is published — so the crawl yields nothing and
// does not fail.
func TestHiringRoomUnpublishedVacanciesAreNotAFailure(t *testing.T) {
	f := &hrFake{missing: 404, pages: map[string]string{
		hrStefPage + "1": hrFixture(t, "stefanini_page1.json"),
		hrStefPage + "2": hrFixture(t, "stefanini_page2.json"),
	}}
	jobs, err := NewHiringRoom(f).Fetch(context.Background(), CompanyEntry{Board: "stefaniniitsolutionsperu", Company: "Stefanini"})
	if err != nil {
		t.Fatalf("an all-404 board failed: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("jobs = %+v", jobs)
	}
}

func TestHiringRoomCardsReadTitleAndTags(t *testing.T) {
	cards := hrCards(t, "emblue_page1.json")
	want := hiringroomCard{
		ID: "69b1d48a1121b4005a5cce59", Title: "Account Executive (Hunter) - España", Location: "Madrid, España",
		Area: "Comercial, Ventas y Negocios / Comercial", Schedule: "Full-time", Modality: "Híbrido", Level: "Senior",
	}
	if len(cards) != 1 || cards[0] != want {
		t.Fatalf("cards = %+v", cards)
	}
	stef := hrCards(t, "stefanini_page1.json")
	if len(stef) != 2 || stef[0].Area != "Tecnología, Sistemas y Telecomunicaciones / Tecnologia / Sistemas" || stef[0].Schedule != "No especificado" {
		t.Errorf("stefanini cards = %+v", stef)
	}
}

func TestHiringRoomJobMapsThePicklists(t *testing.T) {
	job := hiringroomJob(hrCards(t, "emblue_page1.json")[0], "u", "d", CompanyEntry{Company: "emBlue"})
	if job.EmploymentType != "full_time" || job.WorkMode != "hybrid" || job.Seniority != "senior" || job.IsTechHint {
		t.Errorf("emblue = %+v", job)
	}
	job = hiringroomJob(hrCards(t, "stefanini_page1.json")[0], "u", "d", CompanyEntry{})
	if job.EmploymentType != "" || !job.IsTechHint {
		t.Errorf("stefanini = %+v", job)
	}
	job = hiringroomJob(hiringroomCard{Modality: "Remoto", Level: "Jefe"}, "u", "d", CompanyEntry{})
	if job.WorkMode != "remote" || !job.Remote || job.Seniority != "" {
		t.Errorf("remote = %+v", job)
	}
}

func TestHiringRoomDescriptionJoinsSections(t *testing.T) {
	root, _ := html.Parse(strings.NewReader(hrFixture(t, "emblue_vacancy.html")))
	desc, ok := hiringroomDescription(root)
	if !ok {
		t.Fatal("vacancy not read")
	}
	d, r, b := strings.Index(desc, "Descripción del puesto"), strings.Index(desc, "Requisitos"), strings.Index(desc, "Beneficios")
	if d < 0 || r < d || b < r || !strings.Contains(desc, "Ejecutivos de Cuentas") {
		t.Errorf("sections missing or out of order: %d %d %d", d, r, b)
	}
	// The catch-all vacancy is a vacancy page with an empty body, not an unreadable page.
	root, _ = html.Parse(strings.NewReader(hrFixture(t, "stefanini_catchall_vacancy.html")))
	if desc, ok := hiringroomDescription(root); !ok || desc != "" {
		t.Errorf("catch-all = %q, %v", desc, ok)
	}
	root, _ = html.Parse(strings.NewReader("<html><body><p>Página no encontrada</p></body></html>"))
	if _, ok := hiringroomDescription(root); ok {
		t.Error("a page with no description section was read")
	}
}

func TestHiringRoomFetchWalksToTheTotal(t *testing.T) {
	vacancy := "https://stefaniniitsolutionsperu.hiringroom.com/jobs/get_vacancy/"
	f := &hrFake{pages: map[string]string{
		hrStefPage + "1":                     hrFixture(t, "stefanini_page1.json"),
		hrStefPage + "2":                     hrFixture(t, "stefanini_page2.json"),
		vacancy + "66984666bb7169324800e116": hrFixture(t, "emblue_vacancy.html"),
		// 66708149410915a44e5d9e93 has no page: deferred.
	}}
	seen := func(id string) bool { return id == "6675a00fc0bd43544f2ce222" }
	jobs, err := NewHiringRoom(f).(hiringroom).FetchNew(context.Background(), CompanyEntry{Provider: "hiringroom", Board: "stefaniniitsolutionsperu", Company: "Stefanini"}, seen)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.forms) != 2 || f.forms[1].Get("selectedPage") != "2" || f.forms[1].Get("typePortal") != "external" {
		t.Fatalf("forms = %v", f.forms)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %+v", jobs)
	}
	if j := jobs[0]; j.ExternalID != "66984666bb7169324800e116" || j.URL != vacancy+j.ExternalID || j.Description == "" || j.Company != "Stefanini" {
		t.Errorf("hydrated = %+v", j)
	}
	if j := jobs[1]; !j.SeenRefresh || j.ExternalID != "6675a00fc0bd43544f2ce222" {
		t.Errorf("seen = %+v", j)
	}
}

func TestHiringRoomFetchFails(t *testing.T) {
	ctx := context.Background()
	e := CompanyEntry{Board: "stefaniniitsolutionsperu", Company: "Stefanini"}
	// The second page fails before the total is reached.
	f := &hrFake{pages: map[string]string{hrStefPage + "1": hrFixture(t, "stefanini_page1.json")}}
	if _, err := NewHiringRoom(f).Fetch(ctx, e); err == nil {
		t.Error("a walk short of the total succeeded")
	}
	// A page that repeats the previous one adds nothing.
	f = &hrFake{pages: map[string]string{hrStefPage + "1": hrFixture(t, "stefanini_page1.json"), hrStefPage + "2": hrFixture(t, "stefanini_page1.json")}}
	if _, err := NewHiringRoom(f).Fetch(ctx, e); err == nil {
		t.Error("a repeated page succeeded")
	}
	// Every new vacancy unreadable.
	f = &hrFake{pages: map[string]string{hrStefPage + "1": hrFixture(t, "stefanini_page1.json"), hrStefPage + "2": hrFixture(t, "stefanini_page2.json")}}
	if _, err := NewHiringRoom(f).Fetch(ctx, e); err == nil {
		t.Error("a crawl that read no vacancy page succeeded")
	}
	if _, err := NewHiringRoom(f).Fetch(ctx, CompanyEntry{Board: "stefanini.hiringroom.com", Company: "x"}); err == nil {
		t.Error("a host was accepted as a board")
	}
}

func TestHiringRoomIsFullBoardListing(t *testing.T) {
	if _, ok := NewHiringRoom(nil).(fullBoardListing); !ok {
		t.Fatal("hiringroom must claim fullBoardListing")
	}
}
