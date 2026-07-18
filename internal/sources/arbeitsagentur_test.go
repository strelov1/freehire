package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// arbeitsagenturFake routes search calls by their page param and detail calls by the refnr in
// the URL, so a single fake drives both stages (paginated JSON search + per-posting SSR detail).
type arbeitsagenturFake struct {
	searchByPage map[int]string    // page -> search JSON body ("" => empty page)
	detailByRef  map[string]string // refnr -> detail HTML ("" => no ng-state)
	detailErr    map[string]bool   // refnr -> GetHTML returns an error
	gotHeaders   map[string]string
	searchPages  []int // pages requested, in order
}

func (f *arbeitsagenturFake) GetJSONWithHeaders(_ context.Context, u string, headers map[string]string, v any) error {
	f.gotHeaders = headers
	page := 1
	if pu, err := url.Parse(u); err == nil {
		if p, err := strconv.Atoi(pu.Query().Get("page")); err == nil {
			page = p
		}
	}
	f.searchPages = append(f.searchPages, page)
	body := f.searchByPage[page]
	if body == "" {
		body = `{"stellenangebote":[],"maxErgebnisse":0}`
	}
	return json.Unmarshal([]byte(body), v)
}

func (f *arbeitsagenturFake) GetHTML(_ context.Context, u string) (*html.Node, error) {
	ref := u[strings.LastIndex(u, "/")+1:]
	if f.detailErr[ref] {
		return nil, errors.New("detail boom")
	}
	return html.Parse(strings.NewReader(f.detailByRef[ref]))
}

// detailHTML wraps an ng-state script carrying the given description, mirroring the real SSR page.
func detailHTML(desc string) string {
	return `<html><body><script id="ng-state" type="application/json">{"jobdetail":{"stellenangebotsBeschreibung":` +
		strconv.Quote(desc) + `}}</script></body></html>`
}

func TestArbeitsagenturFetchMapsFirstPartyAndDropsExterne(t *testing.T) {
	const page1 = `{"maxErgebnisse":2,"stellenangebote":[
	  {"refnr":"20177-44320844-717-S","titel":"Fachinformatiker*in","arbeitgeber":"Boehringer Ingelheim Pharma GmbH & Co. KG","arbeitsort":{"ort":"Biberach an der Riß","region":"Baden-Württemberg","land":"Deutschland","strasse":"null"},"aktuelleVeroeffentlichungsdatum":"2026-07-18"},
	  {"refnr":"EXT-1","titel":"Re-listed","arbeitgeber":"Other","arbeitsort":{"ort":"Berlin"},"aktuelleVeroeffentlichungsdatum":"2026-07-10","externeUrl":"https://aubi-plus.de/x"},
	  {"refnr":"AC-2","titel":"DevOps Engineer","arbeitgeber":"Acme GmbH","arbeitsort":{"ort":"München","region":"Bayern","land":"Deutschland"},"aktuelleVeroeffentlichungsdatum":"2026-07-15"}
	]}`
	fake := &arbeitsagenturFake{
		searchByPage: map[int]string{1: page1},
		detailByRef: map[string]string{
			"20177-44320844-717-S": detailHTML("Bei Boehringer Ingelheim entwickeln wir <b>bahnbrechende</b> Therapien."),
			"AC-2":                 detailHTML("Wir suchen einen DevOps Engineer."),
		},
	}
	jobs, err := NewArbeitsagentur(fake).Fetch(context.Background(), CompanyEntry{
		Provider: "arbeitsagentur", Board: "Softwareentwicklung und Programmierung",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// The externeUrl re-list is dropped; two first-party postings map.
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (externeUrl dropped)", len(jobs))
	}
	// Header carried the static public key.
	if fake.gotHeaders["X-API-Key"] != "jobboerse-jobsuche" {
		t.Errorf("X-API-Key header = %q", fake.gotHeaders["X-API-Key"])
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	j, ok := byID["20177-44320844-717-S"]
	if !ok {
		t.Fatalf("first-party posting not mapped; got %d jobs", len(jobs))
	}
	if j.URL != "https://www.arbeitsagentur.de/jobsuche/jobdetail/20177-44320844-717-S" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Fachinformatiker*in" || j.Company != "Boehringer Ingelheim Pharma GmbH & Co. KG" {
		t.Errorf("title/company wrong: %q / %q", j.Title, j.Company)
	}
	if j.Location != "Biberach an der Riß, Baden-Württemberg, Deutschland" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-07-18" {
		t.Errorf("PostedAt = %v, want 2026-07-18", j.PostedAt)
	}
}

func TestArbeitsagenturScrapesDescription(t *testing.T) {
	const page1 = `{"maxErgebnisse":2,"stellenangebote":[
	  {"refnr":"OK-1","titel":"A","arbeitgeber":"Co","arbeitsort":{"ort":"Berlin"},"aktuelleVeroeffentlichungsdatum":"2026-07-18"},
	  {"refnr":"NODESC-2","titel":"B","arbeitgeber":"Co","arbeitsort":{"ort":"Berlin"},"aktuelleVeroeffentlichungsdatum":"2026-07-18"},
	  {"refnr":"ERR-3","titel":"C","arbeitgeber":"Co","arbeitsort":{"ort":"Berlin"},"aktuelleVeroeffentlichungsdatum":"2026-07-18"}
	]}`
	fake := &arbeitsagenturFake{
		searchByPage: map[int]string{1: page1},
		detailByRef: map[string]string{
			// The real Stellenbeschreibung is plain text with newline paragraphs, no markup.
			"OK-1":     detailHTML("Bei uns arbeitest du remote.\n\nZweiter Absatz mit Details."),
			"NODESC-2": `<html><body><p>no ng-state here</p></body></html>`,
		},
		detailErr: map[string]bool{"ERR-3": true},
	}
	jobs, err := NewArbeitsagentur(fake).Fetch(context.Background(), CompanyEntry{Board: "Informatik"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// A detail error or a page without a description must not drop the posting.
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3 (missing/failed descriptions still emit)", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	// Plain-text newlines must be rebuilt into paragraph structure, not collapsed into one block.
	if d := byID["OK-1"].Description; !strings.Contains(d, "remote") || !strings.Contains(d, "Zweiter Absatz") || !strings.Contains(d, "<p>") {
		t.Errorf("OK-1 description = %q, want both paragraphs with rebuilt <p> structure", d)
	}
	if d := byID["NODESC-2"].Description; d != "" {
		t.Errorf("NODESC-2 description = %q, want empty (no ng-state block)", d)
	}
	if d := byID["ERR-3"].Description; d != "" {
		t.Errorf("ERR-3 description = %q, want empty (detail fetch failed)", d)
	}
}

func TestArbeitsagenturPaginates(t *testing.T) {
	full := arbeitsagenturPage(arbeitsagenturPageSize, 1) // a full page => keep paginating
	short := arbeitsagenturPage(3, 1000)                  // a short page => stop after it
	fake := &arbeitsagenturFake{
		searchByPage: map[int]string{1: full, 2: short},
		detailByRef:  map[string]string{}, // details resolve to empty descriptions
	}
	jobs, err := NewArbeitsagentur(fake).Fetch(context.Background(), CompanyEntry{Board: "Informatik"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !slices.Equal(fake.searchPages, []int{1, 2}) {
		t.Errorf("requested pages = %v, want [1 2]", fake.searchPages)
	}
	if len(jobs) != arbeitsagenturPageSize+3 {
		t.Errorf("len(jobs) = %d, want %d", len(jobs), arbeitsagenturPageSize+3)
	}
}

// arbeitsagenturPage builds a search body of n first-party postings with ids offset by base.
func arbeitsagenturPage(n, base int) string {
	items := make([]string, n)
	for i := range items {
		items[i] = fmt.Sprintf(`{"refnr":"R-%d","titel":"T","arbeitgeber":"Co","arbeitsort":{"ort":"Berlin"},"aktuelleVeroeffentlichungsdatum":"2026-07-18"}`, base+i)
	}
	return `{"maxErgebnisse":100000,"stellenangebote":[` + strings.Join(items, ",") + `]}`
}

func TestArbeitsagenturProviderRegistered(t *testing.T) {
	if got := NewArbeitsagentur(nil).Provider(); got != "arbeitsagentur" {
		t.Errorf("Provider() = %q, want arbeitsagentur", got)
	}
	if _, ok := All(nil)["arbeitsagentur"]; !ok {
		t.Error("All() should register provider arbeitsagentur")
	}
	if !slices.Contains(FilterableProviders(), "arbeitsagentur") {
		t.Error("FilterableProviders() should include arbeitsagentur")
	}
}

func TestArbeitsagenturBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/arbeitsagentur.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/arbeitsagentur.yml fails validation: %v", err)
	}
}
