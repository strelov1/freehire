package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
)

// mediatekFake serves the real search response (2026-10-25) split over two pages of three
// postings — a Software posting in HsinChu, a Chip Design one in ChuPei and an Information
// Technology one — and each posting's real page trimmed to its <main>.
type mediatekFake struct {
	mutate     func(page int, body map[string]any)
	failDetail string
	mu         sync.Mutex
	pages      []int
	details    int
	cookies    []string
}

func (f *mediatekFake) GetJSON(_ context.Context, u string, v any) error {
	if !strings.HasPrefix(u, mediatekSearchURL) {
		return errors.New("unexpected GET " + u)
	}
	raw, err := url.QueryUnescape(strings.TrimPrefix(u, mediatekSearchURL))
	if err != nil {
		return err
	}
	var in struct {
		JSON mediatekSearchInput `json:"json"`
	}
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return err
	}
	f.pages = append(f.pages, in.JSON.Page)
	file, err := os.ReadFile("testdata/mediatek/search_page" + map[int]string{1: "1", 2: "2"}[in.JSON.Page] + ".json")
	if err != nil {
		return err
	}
	var body map[string]any
	if err := json.Unmarshal(file, &body); err != nil {
		return err
	}
	if f.mutate != nil {
		f.mutate(in.JSON.Page, body)
	}
	out, _ := json.Marshal(body)
	return json.Unmarshal(out, v)
}

func (f *mediatekFake) GetTextWithHeaders(_ context.Context, u string, headers map[string]string) (string, error) {
	f.mu.Lock()
	f.details++
	f.cookies = append(f.cookies, headers["Cookie"])
	f.mu.Unlock()
	id := strings.TrimPrefix(u, mediatekJobURL)
	if id == f.failDetail {
		return "", errors.New("HTTP 500")
	}
	raw, err := os.ReadFile("testdata/mediatek/job_" + id + ".html")
	return string(raw), err
}

func TestMediaTekFetchMapsTheCatalogue(t *testing.T) {
	f := &mediatekFake{}
	jobs, err := NewMediaTek(f).Fetch(context.Background(), CompanyEntry{Company: "MediaTek"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 || len(f.pages) != 2 || f.pages[0] != 1 || f.pages[1] != 2 {
		t.Fatalf("got %d jobs over pages %v", len(jobs), f.pages)
	}
	for _, c := range f.cookies {
		if c != "NEXT_LOCALE=en" {
			t.Errorf("posting page asked without the locale cookie: %q", c)
		}
	}
	byID := byExternalID(jobs)

	sw := byID["MTK120260519002"]
	if sw.Title != "System Performance Optimization Technical Manager - HsinChu" || sw.Company != "MediaTek" ||
		sw.URL != "https://careers.mediatek.com/en/jobs/MTK120260519002" || sw.Category != "software_engineering" {
		t.Errorf("identity: %q %q %s %q", sw.Title, sw.Company, sw.URL, sw.Category)
	}
	if sw.Location != "HsinChu" || len(sw.Countries) != 1 || sw.Countries[0] != "tw" || sw.PostedAt == nil {
		t.Errorf("place/date: %q %v %v", sw.Location, sw.Countries, sw.PostedAt)
	}
	// The listing carries the responsibilities; the page adds the requirements.
	if !strings.Contains(sw.Description, "Proficient in C programming") {
		t.Errorf("requirements missing: %.300s", sw.Description)
	}

	chip := byID["MTK120260630006"]
	if chip.Category != "hardware" || chip.Location != "ChuPei" || len(chip.Countries) != 1 || chip.Countries[0] != "tw" {
		t.Errorf("chip design: %q %q %v", chip.Category, chip.Location, chip.Countries)
	}
	// A broad R&D category names no discipline but states the posting is technical; the
	// site's trailing space in "Information Technology " must not hide it.
	if it := byID["MTK120261006013"]; it.Category != "" || !it.IsTechHint {
		t.Errorf("information technology: %q %v", it.Category, it.IsTechHint)
	}
}

func TestMediaTekSeenPostingsCostNoDetail(t *testing.T) {
	f := &mediatekFake{}
	jobs, err := NewMediaTek(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Company: "MediaTek"},
		func(id string) bool { return id != "MTK120260630006" })
	if err != nil {
		t.Fatal(err)
	}
	if f.details != 1 || len(jobs) != 3 {
		t.Fatalf("details read %d, jobs %d", f.details, len(jobs))
	}
	if j := byExternalID(jobs)["MTK120260519002"]; !j.SeenRefresh || j.Description != "" {
		t.Errorf("seen posting not a bare refresh: %+v", j)
	}
}

func TestMediaTekDefersAFailedDetail(t *testing.T) {
	f := &mediatekFake{failDetail: "MTK120260630006"}
	jobs, err := NewMediaTek(f).Fetch(context.Background(), CompanyEntry{Company: "MediaTek"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := byExternalID(jobs)["MTK120260630006"]; ok || len(jobs) != 2 {
		t.Fatalf("a failed detail must be deferred, got %d jobs", len(jobs))
	}
}

func TestMediaTekFailsWhenNoNewPostingReads(t *testing.T) {
	f := &mediatekFake{failDetail: "MTK120260630006"}
	if _, err := NewMediaTek(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Company: "MediaTek"},
		func(id string) bool { return id != "MTK120260630006" }); err == nil {
		t.Fatal("the only new posting failed to read and the crawl still succeeded")
	}
}

func TestMediaTekFailsRatherThanTruncate(t *testing.T) {
	pagination := func(b map[string]any) map[string]any {
		return b["result"].(map[string]any)["data"].(map[string]any)["json"].(map[string]any)["pagination"].(map[string]any)
	}
	jobsOf := func(b map[string]any) []any {
		return b["result"].(map[string]any)["data"].(map[string]any)["json"].(map[string]any)["jobs"].([]any)
	}
	cases := map[string]func(int, map[string]any){
		"more postings than read": func(_ int, b map[string]any) { pagination(b)["total_items"] = 4.0 },
		"total moves mid-walk": func(p int, b map[string]any) {
			if p == 2 {
				pagination(b)["total_items"] = 5.0
			}
		},
		"a duplicated id": func(p int, b map[string]any) {
			if p == 2 {
				jobsOf(b)[0].(map[string]any)["id"] = "MTK120260519002"
			}
		},
		"no postings at all": func(_ int, b map[string]any) {
			b["result"].(map[string]any)["data"].(map[string]any)["json"].(map[string]any)["jobs"] = []any{}
			pagination(b)["total_items"], pagination(b)["total_pages"] = 0.0, 1.0
		},
		"error status": func(_ int, b map[string]any) {
			b["result"].(map[string]any)["data"].(map[string]any)["json"].(map[string]any)["status"] = "error"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			jobs, err := NewMediaTek(&mediatekFake{mutate: mutate}).Fetch(context.Background(), CompanyEntry{Company: "MediaTek"})
			if err == nil {
				t.Fatalf("got %d jobs and no error", len(jobs))
			}
		})
	}
}

func TestMediaTekTitle(t *testing.T) {
	cases := map[string]string{
		"2027 Campus_IT Engineer":                "2027 Campus - IT Engineer",
		"_5G Modem system validation engineer":   "5G Modem system validation engineer",
		"Senior Software Driver Engineer_Chupei": "Senior Software Driver Engineer - Chupei",
		"AI Software Tool Engineer":              "AI Software Tool Engineer",
	}
	for in, want := range cases {
		if got := mediatekTitle(in); got != want {
			t.Errorf("mediatekTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMediaTekIsBoardlessFullBoardListing(t *testing.T) {
	s := NewMediaTek(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("mediatek must be boardless")
	}
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("mediatek must be fullBoardListing")
	}
}
