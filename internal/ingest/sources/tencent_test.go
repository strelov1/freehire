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

// tencentFake serves the trimmed real responses (2026-10-15): a technology-category listing of
// four Tencent-authored postings and two Workday re-listings, and each native posting's detail.
type tencentFake struct {
	mutate     func(body map[string]any)
	failDetail string
	// mu guards asked and detailsRead: the detail workers call this fake concurrently.
	mu          sync.Mutex
	asked       []url.Values
	detailsRead int
}

func (f *tencentFake) GetJSON(_ context.Context, u string, v any) error {
	parsed, err := url.Parse(u)
	if err != nil {
		return err
	}
	q := parsed.Query()
	var file string
	switch {
	case strings.HasPrefix(u, tencentQueryURL+"?"):
		f.mu.Lock()
		f.asked = append(f.asked, q)
		f.mu.Unlock()
		file = "query.json"
	case strings.HasPrefix(u, tencentDetailURL+"?"):
		f.mu.Lock()
		f.detailsRead++
		f.mu.Unlock()
		if q.Get("postId") == f.failDetail {
			return errors.New("HTTP 500")
		}
		file = "post_" + q.Get("postId") + ".json"
	default:
		return errors.New("unexpected GET " + u)
	}
	raw, err := os.ReadFile("testdata/tencent/" + file)
	if err != nil {
		return err
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	if file == "query.json" && f.mutate != nil {
		f.mutate(body)
	}
	out, _ := json.Marshal(body)
	return json.Unmarshal(out, v)
}

func tencentByID(jobs []Job) map[string]Job {
	out := map[string]Job{}
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestTencentFetchMapsTheCatalogue(t *testing.T) {
	f := &tencentFake{}
	jobs, err := NewTencent(f).Fetch(context.Background(), CompanyEntry{Company: "Tencent"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want the 4 Tencent-authored ones", len(jobs))
	}
	q := f.asked[0]
	if q.Get("parentCategoryId") != "40001" || q.Get("pageSize") != "2000" || q.Get("language") != "en-us" {
		t.Errorf("listing query: %v", q)
	}
	byID := tencentByID(jobs)
	be := byID["2071854045013913600"]
	if be.Title != "混元后台开发工程师（深圳/北京）" || be.Company != "Tencent" || !be.IsTechHint ||
		be.URL != "https://careers.tencent.com/jobdesc.html?postId=2071854045013913600" {
		t.Errorf("identity: %q %q %v %s", be.Title, be.Company, be.IsTechHint, be.URL)
	}
	if be.Location != "Shenzhen, China" || len(be.Countries) != 1 || be.Countries[0] != "cn" || be.PostedAt == nil {
		t.Errorf("place/date: %q %v %v", be.Location, be.Countries, be.PostedAt)
	}
	// The listing has no requirements; the detail does.
	if !strings.Contains(be.Description, "Linux后台开发经验") || be.Category != "" {
		t.Errorf("body/category: %q %.80s", be.Category, be.Description)
	}
	if byID["2008500879786921984"].Category != "qa" || byID["2083853733489459200"].Category != "security" {
		t.Errorf("sub-categories: %q %q", byID["2008500879786921984"].Category, byID["2083853733489459200"].Category)
	}
}

func TestTencentSeenPostingsCostNoDetail(t *testing.T) {
	f := &tencentFake{}
	jobs, err := NewTencent(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{Company: "Tencent"},
		func(id string) bool { return id != "2071854045013913600" })
	if err != nil {
		t.Fatal(err)
	}
	if f.detailsRead != 1 || len(jobs) != 4 {
		t.Fatalf("details read %d, jobs %d", f.detailsRead, len(jobs))
	}
	if j := tencentByID(jobs)["2083853733489459200"]; !j.SeenRefresh {
		t.Errorf("seen posting not a refresh: %+v", j)
	}
}

func TestTencentDefersAFailedDetail(t *testing.T) {
	f := &tencentFake{failDetail: "2083853733489459200"}
	jobs, err := NewTencent(f).Fetch(context.Background(), CompanyEntry{Company: "Tencent"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tencentByID(jobs)["2083853733489459200"]; ok || len(jobs) != 3 {
		t.Fatalf("a failed detail must be deferred, got %d jobs", len(jobs))
	}
}

func TestTencentFailsRatherThanTruncate(t *testing.T) {
	data := func(b map[string]any) map[string]any { return b["Data"].(map[string]any) }
	cases := map[string]func(map[string]any){
		"more postings than read": func(b map[string]any) { data(b)["Count"] = 7.0 },
		"over one request":        func(b map[string]any) { data(b)["Count"] = float64(tencentPageSize + 1) },
		"no count":                func(b map[string]any) { delete(data(b), "Count") },
		"only workday copies": func(b map[string]any) {
			posts := data(b)["Posts"].([]any)
			data(b)["Posts"], data(b)["Count"] = posts[4:], 2.0
		},
		"a duplicated id": func(b map[string]any) {
			posts := data(b)["Posts"].([]any)
			posts[1] = posts[0]
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			jobs, err := NewTencent(&tencentFake{mutate: mutate}).Fetch(context.Background(), CompanyEntry{Company: "Tencent"})
			if err == nil {
				t.Fatalf("got %d jobs and no error", len(jobs))
			}
		})
	}
}

func TestTencentIsBoardlessFullBoardListing(t *testing.T) {
	s := NewTencent(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("tencent must be boardless")
	}
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("tencent must be fullBoardListing")
	}
}
