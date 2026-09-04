package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// arbeitsagenturFake routes search calls by their page param and detail calls by the base64
// referenznummer suffix of the URL, so a single fake drives both stages (paginated JSON search +
// per-posting JSON detail).
type arbeitsagenturFake struct {
	searchByPage map[int]string    // page -> search JSON body ("" => empty page)
	detailByRef  map[string]string // refnr -> detail JSON body ("" => empty detail)
	detailErr    map[string]bool   // refnr -> detail fetch returns an error
	gotHeaders   map[string]string
	searchPages  []int // pages requested, in order
}

func (f *arbeitsagenturFake) GetJSONWithHeaders(_ context.Context, u string, headers map[string]string, v any) error {
	f.gotHeaders = headers
	if strings.Contains(u, arbeitsagenturDetailAPIURL) {
		enc := u[strings.LastIndex(u, "/")+1:]
		raw, err := base64.StdEncoding.DecodeString(enc)
		if err != nil {
			return err
		}
		ref := string(raw)
		if f.detailErr[ref] {
			return errors.New("detail boom")
		}
		body := f.detailByRef[ref]
		if body == "" {
			body = `{}`
		}
		return json.Unmarshal([]byte(body), v)
	}
	page := 1
	if pu, err := url.Parse(u); err == nil {
		if p, err := strconv.Atoi(pu.Query().Get("page")); err == nil {
			page = p
		}
	}
	f.searchPages = append(f.searchPages, page)
	body := f.searchByPage[page]
	if body == "" {
		body = `{"ergebnisliste":[],"maxErgebnisse":0}`
	}
	return json.Unmarshal([]byte(body), v)
}

// arbeitsagenturDetailJSON builds a jobdetails response carrying only the description this adapter reads.
func arbeitsagenturDetailJSON(desc string) string {
	b, _ := json.Marshal(map[string]string{"stellenangebotsBeschreibung": desc})
	return string(b)
}

func TestArbeitsagenturFetchMapsFirstPartyAndDropsExterne(t *testing.T) {
	const page1 = `{"maxErgebnisse":2,"ergebnisliste":[
	  {"referenznummer":"20177-44320844-717-S","stellenangebotsTitel":"Fachinformatiker*in","firma":"Boehringer Ingelheim Pharma GmbH & Co. KG","stellenlokationen":[{"adresse":{"ort":"Biberach an der Riß","region":"Baden-Württemberg","land":"Deutschland"}}],"datumErsteVeroeffentlichung":"2026-07-18","homeofficemoeglich":true},
	  {"referenznummer":"EXT-1","stellenangebotsTitel":"Re-listed","firma":"Other","stellenlokationen":[{"adresse":{"ort":"Berlin"}}],"datumErsteVeroeffentlichung":"2026-07-10","externeURL":"https://aubi-plus.de/x"},
	  {"referenznummer":"AC-2","stellenangebotsTitel":"DevOps Engineer","firma":"Acme GmbH","stellenlokationen":[{"adresse":{"ort":"München","region":"Bayern","land":"Deutschland"}}],"datumErsteVeroeffentlichung":"2026-07-15","homeofficemoeglich":false}
	]}`
	fake := &arbeitsagenturFake{
		searchByPage: map[int]string{1: page1},
		detailByRef: map[string]string{
			"20177-44320844-717-S": arbeitsagenturDetailJSON("Bei Boehringer Ingelheim entwickeln wir bahnbrechende Therapien."),
			"AC-2":                 arbeitsagenturDetailJSON("Wir suchen einen DevOps Engineer."),
		},
	}
	jobs, err := NewArbeitsagentur(fake).Fetch(context.Background(), CompanyEntry{
		Provider: "arbeitsagentur", Board: "Softwareentwicklung und Programmierung",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// The externeURL re-list is dropped; two first-party postings map.
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2 (externeURL dropped)", len(jobs))
	}
	// Header carried the static public key.
	if fake.gotHeaders["X-API-Key"] != arbeitsagenturAPIKey {
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
	// homeofficemoeglich:true (carried directly on the search result) → remote.
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("home-office job: Remote=%v WorkMode=%q, want true/remote", j.Remote, j.WorkMode)
	}
	if ac := byID["AC-2"]; ac.Remote || ac.WorkMode != "" {
		t.Errorf("non-home-office job AC-2: Remote=%v WorkMode=%q, want false/empty", ac.Remote, ac.WorkMode)
	}
}

func TestArbeitsagenturScrapesDescription(t *testing.T) {
	const page1 = `{"maxErgebnisse":3,"ergebnisliste":[
	  {"referenznummer":"OK-1","stellenangebotsTitel":"A","firma":"Co","stellenlokationen":[{"adresse":{"ort":"Berlin"}}],"datumErsteVeroeffentlichung":"2026-07-18"},
	  {"referenznummer":"NODESC-2","stellenangebotsTitel":"B","firma":"Co","stellenlokationen":[{"adresse":{"ort":"Berlin"}}],"datumErsteVeroeffentlichung":"2026-07-18"},
	  {"referenznummer":"ERR-3","stellenangebotsTitel":"C","firma":"Co","stellenlokationen":[{"adresse":{"ort":"Berlin"}}],"datumErsteVeroeffentlichung":"2026-07-18"}
	]}`
	fake := &arbeitsagenturFake{
		searchByPage: map[int]string{1: page1},
		detailByRef: map[string]string{
			// The real Stellenbeschreibung is plain text with newline paragraphs, no markup.
			"OK-1": arbeitsagenturDetailJSON("Bei uns arbeitest du remote.\n\nZweiter Absatz mit Details."),
			// NODESC-2 has no entry, so the fake serves "{}" — an empty description.
		},
		detailErr: map[string]bool{"ERR-3": true},
	}
	jobs, err := NewArbeitsagentur(fake).Fetch(context.Background(), CompanyEntry{Board: "Informatik"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// A detail error or an empty description must not drop the posting.
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
		t.Errorf("NODESC-2 description = %q, want empty (no description in detail response)", d)
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
		items[i] = fmt.Sprintf(`{"referenznummer":"R-%d","stellenangebotsTitel":"T","firma":"Co","stellenlokationen":[{"adresse":{"ort":"Berlin"}}],"datumErsteVeroeffentlichung":"2026-07-18"}`, base+i)
	}
	return `{"maxErgebnisse":100000,"ergebnisliste":[` + strings.Join(items, ",") + `]}`
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
