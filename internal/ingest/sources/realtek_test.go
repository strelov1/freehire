package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
)

// realtekFake serves the real responses (2026-10-25): the two experience halves of the search,
// two postings each, and each posting's detail. goneDetail answers like the site does for an
// unknown id (200, all-null Data).
type realtekFake struct {
	mutate     func(xp string, body map[string]any)
	failDetail string
	goneDetail string
	mu         sync.Mutex
	asked      []string
	details    int
}

func (f *realtekFake) PostFormJSON(_ context.Context, u string, values url.Values, v any) error {
	if u != realtekSearchURL {
		return errors.New("unexpected POST " + u)
	}
	xp := values.Get("xp")
	f.asked = append(f.asked, xp)
	raw, err := os.ReadFile("testdata/realtek/search_xp" + xp + ".json")
	if err != nil {
		return err
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	if f.mutate != nil {
		f.mutate(xp, body)
	}
	out, _ := json.Marshal(body)
	return json.Unmarshal(out, v)
}

func (f *realtekFake) GetJSON(_ context.Context, u string, v any) error {
	f.mu.Lock()
	f.details++
	f.mu.Unlock()
	id := strings.TrimPrefix(u, realtekDetailURL)
	file := "detail_" + id + ".json"
	switch id {
	case f.failDetail:
		return errors.New("HTTP 500")
	case f.goneDetail:
		file = "detail_gone.json"
	}
	raw, err := os.ReadFile("testdata/realtek/" + file)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func TestRealtekFetchReadsBothHalves(t *testing.T) {
	f := &realtekFake{}
	jobs, err := NewRealtek(f).Fetch(context.Background(), CompanyEntry{Company: "Realtek"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 4 || strings.Join(f.asked, ",") != "0,1" {
		t.Fatalf("got %d jobs over halves %v", len(jobs), f.asked)
	}
	byID := byExternalID(jobs)

	layout := byID["18"]
	if layout.Title != "IC Layout工程師" || layout.Company != "Realtek" || layout.Category != "hardware" ||
		layout.URL != "https://recruit.realtek.com/Job/JobDetail?jobid=18" {
		t.Errorf("identity: %q %q %q %s", layout.Title, layout.Company, layout.Category, layout.URL)
	}
	if layout.Location != "Hsinchu" || len(layout.Countries) != 1 || layout.Countries[0] != "tw" {
		t.Errorf("place: %q %v", layout.Location, layout.Countries)
	}
	if !strings.Contains(layout.Description, "IC layout相關經驗") || layout.ExperienceYearsMin != nil {
		t.Errorf("body/experience: %v %.200s", layout.ExperienceYearsMin, layout.Description)
	}

	// A broad R&D type names no discipline but states the posting is technical.
	if sys := byID["202"]; sys.Category != "" || !sys.IsTechHint {
		t.Errorf("system design: %q %v", sys.Category, sys.IsTechHint)
	}
	if dv := byID["343"]; dv.Category != "hardware" || dv.ExperienceYearsMin == nil || *dv.ExperienceYearsMin != 1 {
		t.Errorf("digital design: %q %v", dv.Category, dv.ExperienceYearsMin)
	}
	if hq := byID["1554"]; hq.Location != "Taipei" || hq.Category != "" || hq.IsTechHint {
		t.Errorf("Taipei office: %q %q %v", hq.Location, hq.Category, hq.IsTechHint)
	}
}

func TestRealtekSeenPostingsCostNoDetail(t *testing.T) {
	f := &realtekFake{}
	jobs, err := NewRealtek(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Company: "Realtek"},
		func(id string) bool { return id != "343" })
	if err != nil {
		t.Fatal(err)
	}
	if f.details != 1 || len(jobs) != 4 {
		t.Fatalf("details read %d, jobs %d", f.details, len(jobs))
	}
	if j := byExternalID(jobs)["18"]; !j.SeenRefresh {
		t.Errorf("seen posting not a refresh: %+v", j)
	}
}

func TestRealtekDetailGoneAndFailed(t *testing.T) {
	f := &realtekFake{goneDetail: "202", failDetail: "343"}
	jobs, err := NewRealtek(f).Fetch(context.Background(), CompanyEntry{Company: "Realtek"})
	if err != nil {
		t.Fatal(err)
	}
	byID := byExternalID(jobs)
	if _, ok := byID["202"]; ok {
		t.Error("a posting the site answers as not found must be dropped")
	}
	if _, ok := byID["343"]; ok || len(jobs) != 2 {
		t.Errorf("a failed detail must be deferred, got %d jobs", len(jobs))
	}
	f = &realtekFake{failDetail: "343"}
	if _, err := NewRealtek(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Company: "Realtek"},
		func(id string) bool { return id != "343" }); err == nil {
		t.Error("the only new posting failed to read and the crawl still succeeded")
	}
}

func TestRealtekFailsRatherThanTruncate(t *testing.T) {
	cases := map[string]func(string, map[string]any){
		"a half at the row cap": func(xp string, b map[string]any) {
			if xp == "1" {
				rows := b["Data"].([]any)
				var full []any
				for i := 0; i < realtekRowCap; i++ {
					row := map[string]any{}
					for k, v := range rows[0].(map[string]any) {
						row[k] = v
					}
					row["JobOppId"] = fmt.Sprintf("9%04d", i)
					full = append(full, row)
				}
				b["Data"] = full
			}
		},
		"an id in both halves": func(xp string, b map[string]any) {
			if xp == "1" {
				b["Data"].([]any)[0].(map[string]any)["JobOppId"] = "18"
			}
		},
		"Pass false":   func(_ string, b map[string]any) { b["Pass"] = false },
		"empty board":  func(_ string, b map[string]any) { b["Data"] = []any{} },
		"malformed id": func(_ string, b map[string]any) { b["Data"].([]any)[0].(map[string]any)["JobOppId"] = "x1" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			jobs, err := NewRealtek(&realtekFake{mutate: mutate}).Fetch(context.Background(), CompanyEntry{Company: "Realtek"})
			if err == nil {
				t.Fatalf("got %d jobs and no error", len(jobs))
			}
		})
	}
}

func TestRealtekIsBoardless(t *testing.T) {
	if _, ok := NewRealtek(nil).(boardless); !ok {
		t.Error("realtek must be boardless")
	}
}
