package sources

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/MoeForge/moego/regexp"
)

// garenaFake serves the real listing and detail fixtures. A detail id with no fixture answers
// the platform's own "gone" body; ids in fail answer a transport error of the given status.
type garenaFake struct {
	list    string
	details map[string]string
	fail    map[string]error

	mu      sync.Mutex
	getURLs []string
}

var garenaDetailRE = regexp.MustCompile(`/api/job/detail/([^?]+)`)

func (f *garenaFake) PostJSON(_ context.Context, _ string, _, v any) error {
	return json.Unmarshal([]byte(f.list), v)
}

func (f *garenaFake) GetJSON(_ context.Context, u string, v any) error {
	f.mu.Lock()
	f.getURLs = append(f.getURLs, u)
	f.mu.Unlock()
	id := garenaDetailRE.FindStringSubmatch(u)[1]
	if err := f.fail[id]; err != nil {
		return err
	}
	raw, ok := f.details[id]
	if !ok {
		raw = `{"job": {}}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func garenaFixtures(t *testing.T) *garenaFake {
	t.Helper()
	read := func(name string) string {
		raw, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	f := &garenaFake{list: read("garena_list.json"), details: map[string]string{}}
	for _, id := range []string{"J02058270", "J02104326", "J02187012", "J02186174"} {
		f.details[id] = read("garena_detail_" + id + ".json")
	}
	return f
}

func TestGarenaProviderAndMarkers(t *testing.T) {
	s := NewGarena(nil)
	if s.Provider() != "garena" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	if _, ok := s.(boardless); !ok {
		t.Error("garena should implement the boardless marker")
	}
	if !FullBoardListingProviders(All(nil))["garena"] {
		t.Error("FullBoardListingProviders(All(nil)) should include garena")
	}
}

func TestGarenaFetchMapsRealResponses(t *testing.T) {
	f := garenaFixtures(t)
	jobs, err := NewGarena(f).Fetch(context.Background(), CompanyEntry{Company: "Garena"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want 4", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	intern := byID["J02058270"]
	if intern.URL != "https://careers.garena.com/global/careers/J02058270" || intern.Company != "Garena" {
		t.Errorf("intern URL/company = %q / %q", intern.URL, intern.Company)
	}
	if intern.Title != "Game Mode Developer Freelance (Onsite)" || intern.Location != "Jakarta" {
		t.Errorf("intern title/location = %q / %q", intern.Title, intern.Location)
	}
	if intern.EmploymentType != "internship" || intern.Seniority != "intern" {
		t.Errorf("intern employment/seniority = %q/%q", intern.EmploymentType, intern.Seniority)
	}
	// The summary paragraph comes first, then both detail-only sections under their headings.
	d := intern.Description
	if !strings.HasPrefix(d, "<p>As a Game Mode Developer") ||
		!strings.Contains(d, "<h3>Job Description</h3><ul><li>Propose, build, and publish") ||
		!strings.Contains(d, "<h3>Job Requirements</h3><ul><li>Fresh graduates") {
		t.Errorf("description not assembled from all three parts: %.300s", d)
	}

	if ic := byID["J02104326"]; ic.Seniority != "" || ic.Location != "Singapore" || !strings.Contains(ic.Description, "<h3>Job Requirements</h3>") {
		t.Errorf("IC posting = seniority %q location %q", ic.Seniority, ic.Location)
	}
	if remote := byID["J02187012"]; remote.WorkMode != "remote" || !remote.Remote || remote.Location != "" {
		t.Errorf("Remote location tag = mode %q remote %v location %q", remote.WorkMode, remote.Remote, remote.Location)
	}
	if lead := byID["J02186174"]; lead.Seniority != "lead" {
		t.Errorf("team lead seniority = %q", lead.Seniority)
	}
}

func TestGarenaDetailGoneAndUnreadable(t *testing.T) {
	gone, err := os.ReadFile("testdata/garena_detail_gone.json")
	if err != nil {
		t.Fatal(err)
	}
	f := garenaFixtures(t)
	f.details["J02104326"] = string(gone) // the platform's own answer for a withdrawn posting
	f.fail = map[string]error{
		"J02187012": &StatusError{Code: 404},
		"J02186174": &StatusError{Code: 503},
	}
	jobs, err := NewGarena(f).Fetch(context.Background(), CompanyEntry{Company: "Garena"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, j := range jobs {
		got[j.ExternalID] = j.Unreadable
	}
	if _, ok := got["J02104326"]; ok {
		t.Error("an empty job object is gone and must be dropped")
	}
	if _, ok := got["J02187012"]; ok {
		t.Error("a 404 is gone and must be dropped")
	}
	if unreadable, ok := got["J02186174"]; !ok || !unreadable {
		t.Error("a 503 must yield an Unreadable marker")
	}
	if unreadable := got["J02058270"]; unreadable {
		t.Error("a readable posting must not be marked")
	}
}

func TestGarenaFetchFails(t *testing.T) {
	t.Run("no jobs array", func(t *testing.T) {
		f := &garenaFake{list: `{"filters": {}}`}
		if _, err := NewGarena(f).Fetch(context.Background(), CompanyEntry{Company: "Garena"}); err == nil {
			t.Error("a listing without its jobs array must fail")
		}
	})
	t.Run("read none of what it listed", func(t *testing.T) {
		f := garenaFixtures(t)
		f.fail = map[string]error{}
		for id := range f.details {
			f.fail[id] = &StatusError{Code: 429}
		}
		if _, err := NewGarena(f).Fetch(context.Background(), CompanyEntry{Company: "Garena"}); err == nil {
			t.Error("a crawl that read none of its postings must fail")
		}
	})
}
