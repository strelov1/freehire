package sources

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
)

// foxconnFake serves the real responses (2026-10-25): the listing trimmed to four vacancies — a
// server-R&D BIOS posting, a Vietnamese one, a disability-friendly umbrella posting located
// "other", and a general-hiring Java posting — and each vacancy's detail.
type foxconnFake struct {
	list       string
	failDetail string
	mu         sync.Mutex
	details    int
}

func (f *foxconnFake) GetJSON(_ context.Context, u string, v any) error {
	if u == foxconnListURL {
		body := f.list
		if body == "" {
			raw, err := os.ReadFile("testdata/foxconn/vacancies.json")
			if err != nil {
				return err
			}
			body = string(raw)
		}
		return json.Unmarshal([]byte(body), v)
	}
	f.mu.Lock()
	f.details++
	f.mu.Unlock()
	id := strings.TrimPrefix(u, foxconnDetailURL)
	if id == f.failDetail {
		return errors.New("HTTP 404")
	}
	raw, err := os.ReadFile("testdata/foxconn/vacancy_" + id + ".json")
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

const (
	foxconnBIOS     = "b99c9a75006b4591b732f9f57c464364"
	foxconnVietnam  = "73c854df5ff7443384fd0977938ba493"
	foxconnUmbrella = "d3740d19133b4d74a23d7b018194ffe4"
	foxconnJava     = "08defc39efe613a7befb8586d5a87e67"
)

func TestFoxconnFetchMapsThePortal(t *testing.T) {
	f := &foxconnFake{}
	jobs, err := NewFoxconn(f).Fetch(context.Background(), CompanyEntry{Company: "Foxconn"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 4 || f.details != 4 {
		t.Fatalf("got %d jobs over %d details", len(jobs), f.details)
	}
	byID := byExternalID(jobs)

	bios := byID[foxconnBIOS]
	if bios.Title != "BIOS 工程師" || bios.Company != "Foxconn" || !bios.IsTechHint ||
		bios.URL != "https://recruit.foxconn.com/isite-web-tw/main/jobsearch/detail?id="+foxconnBIOS {
		t.Errorf("identity: %q %q %v %s", bios.Title, bios.Company, bios.IsTechHint, bios.URL)
	}
	if bios.Location != "Taiwan" || len(bios.Countries) != 1 || bios.Countries[0] != "tw" {
		t.Errorf("place: %q %v", bios.Location, bios.Countries)
	}
	if bios.Description == "" {
		t.Error("empty body")
	}

	if vn := byID[foxconnVietnam]; vn.Location != "Vietnam" || len(vn.Countries) != 1 || vn.Countries[0] != "vn" || vn.IsTechHint {
		t.Errorf("Vietnam: %q %v %v", vn.Location, vn.Countries, vn.IsTechHint)
	}
	// "Other" names no place, so it states neither a location nor a country.
	if other := byID[foxconnUmbrella]; other.Location != "" || other.Countries != nil {
		t.Errorf("other: %q %v", other.Location, other.Countries)
	}
	// A general-hiring track says nothing about the discipline.
	if java := byID[foxconnJava]; java.IsTechHint || !strings.Contains(java.Title, "Java") {
		t.Errorf("general hiring: %q %v", java.Title, java.IsTechHint)
	}
}

func TestFoxconnSeenPostingsCostNoDetail(t *testing.T) {
	f := &foxconnFake{}
	jobs, err := NewFoxconn(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Company: "Foxconn"},
		func(id string) bool { return id != foxconnJava })
	if err != nil {
		t.Fatal(err)
	}
	if f.details != 1 || len(jobs) != 4 {
		t.Fatalf("details read %d, jobs %d", f.details, len(jobs))
	}
	if j := byExternalID(jobs)[foxconnBIOS]; !j.SeenRefresh {
		t.Errorf("seen posting not a refresh: %+v", j)
	}
}

func TestFoxconnDefersAFailedDetail(t *testing.T) {
	f := &foxconnFake{failDetail: foxconnVietnam}
	jobs, err := NewFoxconn(f).Fetch(context.Background(), CompanyEntry{Company: "Foxconn"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := byExternalID(jobs)[foxconnVietnam]; ok || len(jobs) != 3 {
		t.Fatalf("a failed detail must be deferred, got %d jobs", len(jobs))
	}
	f = &foxconnFake{failDetail: foxconnVietnam}
	if _, err := NewFoxconn(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Company: "Foxconn"},
		func(id string) bool { return id != foxconnVietnam }); err == nil {
		t.Error("the only new posting failed to read and the crawl still succeeded")
	}
}

func TestFoxconnFailsRatherThanEmpty(t *testing.T) {
	raw, err := os.ReadFile("testdata/foxconn/vacancies.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"empty listing": `[]`,
		"duplicate id":  strings.Replace(string(raw), foxconnJava, foxconnBIOS, 1),
		"malformed id":  strings.Replace(string(raw), foxconnJava, "PP26022700001", 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if jobs, err := NewFoxconn(&foxconnFake{list: body}).Fetch(context.Background(), CompanyEntry{Company: "Foxconn"}); err == nil {
				t.Fatalf("got %d jobs and no error", len(jobs))
			}
		})
	}
}

func TestFoxconnIsBoardlessNotFullListing(t *testing.T) {
	s := NewFoxconn(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("must be boardless")
	}
	if _, ok := s.(fullBoardListing); ok {
		t.Error("a single response with no total must not claim fullBoardListing")
	}
}
