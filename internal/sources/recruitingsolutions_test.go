package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// rsFake is a HeaderJSONPoster that records the customerId header and returns the canned
// page for the request body's "skip" offset (an absent offset yields an empty page, so the
// pagination loop terminates).
type rsFake struct {
	pages      map[int]string
	customerID string
	calls      int
}

func (f *rsFake) PostJSONWithHeaders(_ context.Context, _ string, headers map[string]string, body, v any) error {
	f.customerID = headers["customerId"]
	f.calls++
	skip := rsBodySkip(body)
	page, ok := f.pages[skip]
	if !ok {
		page = `{"@odata.count":0,"value":[]}`
	}
	return json.Unmarshal([]byte(page), v)
}

// rsBodySkip extracts the skip offset from the adapter's request body regardless of whether
// it is a map or a typed struct, by round-tripping through JSON.
func rsBodySkip(body any) int {
	b, _ := json.Marshal(body)
	var req struct {
		Skip int `json:"skip"`
	}
	_ = json.Unmarshal(b, &req)
	return req.Skip
}

// rsRecord renders one Azure-search record (one locale of one posting) as the API returns it.
func rsRecord(baseID, lang, title, company, desc, teleID string, active bool) string {
	return fmt.Sprintf(`{"jobId":"%s-%s","title":%q,"company":%q,"description":%q,`+
		`"teleComuteId":%q,"isActive":%t,"datePosted":"2026-05-29T07:58:01Z","country":"Germany",`+
		`"location":{"city":"Berlin","countryProvince":"DE-BE"},`+
		`"link":"https://jobsearch.createyourowncareer.com/job-invite/%s/?locale=%s"}`,
		baseID, lang, title, company, desc, teleID, active, baseID, lang)
}

func rsPage(count int, records ...string) string {
	return fmt.Sprintf(`{"@odata.count":%d,"value":[%s]}`, count, strings.Join(records, ","))
}

func TestRecruitingSolutionsProvider(t *testing.T) {
	if got := NewRecruitingSolutions(nil).Provider(); got != "recruitingsolutions" {
		t.Errorf("Provider() = %q, want %q", got, "recruitingsolutions")
	}
}

func TestRecruitingSolutionsFetchDedupsLocalesAndMaps(t *testing.T) {
	page := rsPage(3,
		// base 100: three locales of one posting; en_GB is the English title we want kept.
		rsRecord("100", "de_DE", "Ingenieur", "Riverty Group GmbH", "<p>Body</p>", "telecommute1", true),
		rsRecord("100", "en_GB", "Engineer", "Riverty Group GmbH", "<p>Body</p>", "telecommute1", true),
		rsRecord("100", "en_US", "Engineer US", "Riverty Group GmbH", "<p>Body</p>", "telecommute1", true),
		// base 200: only a non-English locale — kept as the fallback. Office-based.
		rsRecord("200", "de_DE", "Buchhalter", "BFS Health Finance GmbH", "<p>Zahlen</p>", "telecommute3", true),
		// base 300: every locale inactive — dropped entirely.
		rsRecord("300", "en_GB", "Closed Role", "Riverty Norway AS", "<p>gone</p>", "telecommute1", false),
	)
	fake := &rsFake{pages: map[int]string{0: page}}

	jobs, err := NewRecruitingSolutions(fake).Fetch(context.Background(), CompanyEntry{Board: "riv-prod"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.customerID != "riv-prod" {
		t.Errorf("customerId header = %q, want %q", fake.customerID, "riv-prod")
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (locales deduped, inactive dropped): %+v", len(jobs), jobs)
	}

	by := map[string]Job{}
	for _, j := range jobs {
		by[j.ExternalID] = j
	}

	j100, ok := by["100"]
	if !ok {
		t.Fatal("missing job 100")
	}
	if j100.Title != "Engineer" {
		t.Errorf("job 100 Title = %q, want %q (English locale preferred)", j100.Title, "Engineer")
	}
	if j100.Company != "Riverty Group GmbH" {
		t.Errorf("job 100 Company = %q", j100.Company)
	}
	if want := "https://jobsearch.createyourowncareer.com/job-invite/100/"; j100.URL != want {
		t.Errorf("job 100 URL = %q, want %q (locale query stripped)", j100.URL, want)
	}
	if !strings.Contains(j100.Description, "Body") {
		t.Errorf("job 100 Description = %q, want it to carry the body", j100.Description)
	}
	if j100.WorkMode != "hybrid" {
		t.Errorf("job 100 WorkMode = %q, want hybrid (telecommute1)", j100.WorkMode)
	}
	if j100.PostedAt == nil {
		t.Error("job 100 PostedAt is nil, want parsed datePosted")
	}

	j200, ok := by["200"]
	if !ok {
		t.Fatal("missing job 200 (non-English fallback)")
	}
	if j200.WorkMode != "onsite" {
		t.Errorf("job 200 WorkMode = %q, want onsite (telecommute3)", j200.WorkMode)
	}
}

func TestRecruitingSolutionsPaginates(t *testing.T) {
	// A full first page forces a second request; the short second page completes the count.
	total := recruitingSolutionsPageSize + 2
	first := make([]string, recruitingSolutionsPageSize)
	for i := range first {
		first[i] = rsRecord(fmt.Sprintf("%d", i), "en_GB", "Role", "Riverty Group GmbH", "<p>x</p>", "telecommute1", true)
	}
	second := []string{
		rsRecord("9001", "en_GB", "Late A", "Riverty Group GmbH", "<p>x</p>", "telecommute1", true),
		rsRecord("9002", "en_GB", "Late B", "Riverty Group GmbH", "<p>x</p>", "telecommute1", true),
	}
	fake := &rsFake{pages: map[int]string{
		0:                           rsPage(total, first...),
		recruitingSolutionsPageSize: rsPage(total, second...),
	}}

	jobs, err := NewRecruitingSolutions(fake).Fetch(context.Background(), CompanyEntry{Board: "riv-prod"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != total {
		t.Errorf("got %d jobs, want %d (both pages collected)", len(jobs), total)
	}
	if fake.calls < 2 {
		t.Errorf("made %d requests, want at least 2 (pagination)", fake.calls)
	}
}
