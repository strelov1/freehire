package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// paycomFake stands in for the three transport roles the Paycom adapter uses: GetText (the
// portal page carrying the session JWT), PostJSONWithHeaders (the authed previews search),
// and GetJSONWithHeaders (the authed company-name + job detail). It serves a canned body per
// URL substring and records the Authorization header it last saw.
type paycomFake struct {
	pages  map[string]string // GetText: url-substr -> html
	posts  map[string]string // PostJSON: url-substr -> json
	gets   map[string]string // GetJSON: url-substr -> json
	lastAu string
}

func (f *paycomFake) GetText(_ context.Context, url string) (string, error) {
	for k, v := range f.pages {
		if strings.Contains(url, k) {
			return v, nil
		}
	}
	return "", fmt.Errorf("paycomFake: no page for %s", url)
}

func (f *paycomFake) match(m map[string]string, url string) (string, bool) {
	// longest-key-first so "job-postings/" wins over a broader substring
	best, ok := "", false
	for k := range m {
		if strings.Contains(url, k) && (!ok || len(k) > len(best)) {
			best, ok = k, true
		}
	}
	if !ok {
		return "", false
	}
	return m[best], true
}

func (f *paycomFake) GetJSONWithHeaders(_ context.Context, url string, h map[string]string, v any) error {
	f.lastAu = h["Authorization"]
	body, ok := f.match(f.gets, url)
	if !ok {
		return fmt.Errorf("paycomFake: no GET for %s", url)
	}
	return json.Unmarshal([]byte(body), v)
}

func (f *paycomFake) PostJSONWithHeaders(_ context.Context, url string, h map[string]string, _, v any) error {
	f.lastAu = h["Authorization"]
	body, ok := f.match(f.posts, url)
	if !ok {
		return fmt.Errorf("paycomFake: no POST for %s", url)
	}
	return json.Unmarshal([]byte(body), v)
}

const paycomPortalPage = `<html><head><script>
var configsFromHost = {"sessionJWT":"JWT123","atsPortalMantleServiceUrl":"https:\/\/portal-applicant-tracking.us-cent.paycomonline.net\/"};
</script></head><body>Loading...</body></html>`

func TestPaycomProvider(t *testing.T) {
	if got := NewPaycom(nil).Provider(); got != "paycom" {
		t.Errorf("Provider() = %q, want %q", got, "paycom")
	}
}

func TestPaycomFetchBootstrapsListsAndMaps(t *testing.T) {
	fake := &paycomFake{
		pages: map[string]string{"/portal/CK/jobs/1": paycomPortalPage},
		gets: map[string]string{
			"/api/ats/company-name":        `{"companyName":"City Electric Supply"}`,
			"/api/ats/job-postings/546849": `{"jobPosting":{"jobId":546849,"jobTitle":"Van Driver","location":"Centennial, CO","city":"Centennial","remoteType":"","description":"<p>Drive a van.</p><script>x()<\/script>","startDate":"2026-06-20T00:00:00+00:00"}}`,
			"/api/ats/job-postings/537487": `{"jobPosting":{"jobId":537487,"jobTitle":"Gear Specialist","location":"Maitland, FL","city":"Maitland","remoteType":"Remote","description":"<p>Quote gears.</p>","startDate":"2026-06-19T00:00:00+00:00"}}`,
		},
		posts: map[string]string{
			// one page of two previews, total count 2; jobId is a JSON number
			"/api/ats/job-posting-previews/search": `{"jobPostingPreviewsCount":2,"jobPostingPreviews":[{"jobId":546849},{"jobId":537487}]}`,
		},
	}

	jobs, err := NewPaycom(fake).Fetch(context.Background(), CompanyEntry{Company: "fallback", Board: "CK"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.lastAu != "JWT123" {
		t.Errorf("Authorization header = %q, want the bootstrapped JWT", fake.lastAu)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	v, ok := byID["546849"]
	if !ok {
		t.Fatalf("missing job 546849; got %v", byID)
	}
	if v.Title != "Van Driver" || v.Company != "City Electric Supply" || v.Location != "Centennial, CO" {
		t.Errorf("job 546849: title=%q company=%q loc=%q", v.Title, v.Company, v.Location)
	}
	if strings.Contains(v.Description, "<script>") {
		t.Errorf("description not sanitized: %q", v.Description)
	}
	if v.PostedAt == nil {
		t.Errorf("job 546849 PostedAt is nil, want parsed startDate")
	}
	if r := byID["537487"]; !r.Remote {
		t.Errorf("job 537487 should be Remote (remoteType=Remote)")
	}
}
