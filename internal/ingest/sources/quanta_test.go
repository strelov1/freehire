package sources

import (
	"context"
	"os"
	"strings"
	"testing"
)

// quantaFixture is the real QueryJob dump (2026-10-25) trimmed to four vacancies: a hardware
// and a software R&D posting at the Linkou R&D centre, a Mexican one in the "other" category,
// and a Hwa Ya plant posting whose requirements are null.
func quantaFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/quanta/query_job.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestQuantaFetchMapsTheDump(t *testing.T) {
	f := (&routedHTTP{}).route(quantaListURL, quantaFixture(t))
	jobs, err := NewQuanta(f).Fetch(context.Background(), CompanyEntry{Company: "Quanta Computer"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 4 || f.calls != 1 {
		t.Fatalf("got %d jobs over %d requests, want 4 over one", len(jobs), f.calls)
	}
	byID := byExternalID(jobs)

	hw := byID["106"]
	if hw.Title != "雲端伺服器硬體設計" || hw.Company != "Quanta Computer" || hw.Category != "hardware" ||
		hw.URL != "https://hr.quantatw.com/QuantaRecruit/Home/Job?serial=106" {
		t.Errorf("identity: %q %q %q %s", hw.Title, hw.Company, hw.Category, hw.URL)
	}
	if hw.Location != "New Taipei" || len(hw.Countries) != 1 || hw.Countries[0] != "tw" {
		t.Errorf("place: %q %v", hw.Location, hw.Countries)
	}
	// Duties and requirements both reach the body.
	if !strings.Contains(hw.Description, "x86系統為主") || !strings.Contains(hw.Description, "多益600+") {
		t.Errorf("body: %.200s", hw.Description)
	}

	if byID["2073"].Category != "software_engineering" {
		t.Errorf("software category: %q", byID["2073"].Category)
	}
	mx := byID["1921"]
	if mx.Category != "" || mx.Location != "Mexico" || len(mx.Countries) != 1 || mx.Countries[0] != "mx" {
		t.Errorf("an unmapped category must stay empty and the country still resolve: %q %q %v", mx.Category, mx.Location, mx.Countries)
	}
	// A null requirements field keeps the posting on its duties alone.
	if plant := byID["2194"]; plant.Description == "" || plant.Category != "manufacturing" || plant.Location != "Taoyuan" {
		t.Errorf("null requirements: %q %q %.80s", plant.Category, plant.Location, plant.Description)
	}
}

func TestQuantaFailsRatherThanEmpty(t *testing.T) {
	dup := strings.Replace(quantaFixture(t), `"serial": "2073"`, `"serial": "106"`, 1)
	cases := map[string]string{
		"empty dump":    `{"jobResult": []}`,
		"no result key": `{}`,
		"duplicate id":  dup,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			f := (&routedHTTP{}).route(quantaListURL, body)
			if jobs, err := NewQuanta(f).Fetch(context.Background(), CompanyEntry{Company: "Quanta Computer"}); err == nil {
				t.Fatalf("got %d jobs and no error", len(jobs))
			}
		})
	}
}

func TestQuantaIsBoardlessNotFullListing(t *testing.T) {
	s := NewQuanta(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("must be boardless")
	}
	if _, ok := s.(fullBoardListing); ok {
		t.Error("a single response with no total must not claim fullBoardListing")
	}
}
