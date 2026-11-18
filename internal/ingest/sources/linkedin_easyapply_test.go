package sources

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// easyApplyFake serves scripted listings keyed by (f_AL, start) and details by id, and records
// every search it is asked for as "primary:<start>" or "easy:<start>".
type easyApplyFake struct {
	primary, easy map[string]string
	details       map[string]string
	detailErr     map[string]error
	searches      []string
	detailReads   []string
}

func (f *easyApplyFake) GetHTMLResolved(_ context.Context, raw string) (*html.Node, string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, "", err
	}
	body := "<html><body></body></html>"
	switch {
	case u.Path == "/jobs-guest/jobs/api/seeMoreJobPostings/search":
		q := u.Query()
		kind, pages := "primary", f.primary
		if q.Get("f_AL") == "true" {
			kind, pages = "easy", f.easy
		}
		if q.Get("keywords") != "go" || q.Get("location") != "UK" || q.Get("sortBy") != "DD" || q.Get("f_TPR") != "r604800" {
			return nil, "", fmt.Errorf("search parameters differ from the primary listing: %s", raw)
		}
		f.searches = append(f.searches, kind+":"+q.Get("start"))
		if page, ok := pages[q.Get("start")]; ok {
			body = page
		}
	case strings.HasPrefix(u.Path, "/jobs-guest/jobs/api/jobPosting/"):
		id := strings.TrimPrefix(u.Path, "/jobs-guest/jobs/api/jobPosting/")
		f.detailReads = append(f.detailReads, id)
		if err := f.detailErr[id]; err != nil {
			return nil, "", err
		}
		body = linkedInRelatedPosting(id)
		if page, ok := f.details[id]; ok {
			body = page
		}
	default:
		return nil, "", fmt.Errorf("unexpected request %s", raw)
	}
	root, err := html.Parse(strings.NewReader(body))
	return root, raw, err
}

func easyApplyFetch(t *testing.T, f *easyApplyFake, tenants map[string]string) ([]Job, error) {
	t.Helper()
	s := NewLinkedIn(f).(linkedIn)
	s.pace = &recordingWaiter{}
	return s.Fetch(context.Background(), CompanyEntry{Board: "go", Region: "UK", Tenants: tenants})
}

func jobIDs(jobs []Job) []string {
	var ids []string
	for _, j := range jobs {
		ids = append(ids, j.ExternalID)
	}
	slices.Sort(ids)
	return ids
}

func TestLinkedInEasyApplyOptionValidation(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int
		ok    bool
	}{
		{"0", 0, true}, {"1", 1, true}, {"40", 40, true}, {"100", 100, true},
		{"", 0, false}, {"-1", 0, false}, {"101", 0, false}, {"1.5", 0, false}, {"+5", 0, false}, {" 5 ", 0, false}, {"abc", 0, false},
	} {
		_, _, got, err := linkedInSearchOptions(map[string]string{"easyapply_pages": tc.value})
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("easyapply_pages=%q: got %d, %v", tc.value, got, err)
		}
	}
	if _, _, got, err := linkedInSearchOptions(nil); err != nil || got != 0 {
		t.Errorf("absent easyapply_pages = %d, %v; want 0 (off)", got, err)
	}
}

func TestLinkedInEasyApplyPassRunsAfterPrimaryAndDeduplicates(t *testing.T) {
	f := &easyApplyFake{
		primary: map[string]string{"0": linkedInRelatedListing("1") + linkedInRelatedListing("2")},
		easy:    map[string]string{"0": linkedInRelatedListing("2") + linkedInRelatedListing("3")},
	}
	jobs, err := easyApplyFetch(t, f, map[string]string{"pages": "5", "easyapply_pages": "5"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"primary:0", "primary:2", "easy:0", "easy:2"}; !slices.Equal(f.searches, want) {
		t.Errorf("searches = %v, want %v (primary first, then f_AL, each with its own offset)", f.searches, want)
	}
	if want := []string{"1", "2", "3"}; !slices.Equal(jobIDs(jobs), want) {
		t.Errorf("jobs = %v, want %v", jobIDs(jobs), want)
	}
	reads := slices.Clone(f.detailReads)
	slices.Sort(reads)
	if want := []string{"1", "2", "3"}; !slices.Equal(reads, want) {
		t.Errorf("detail reads = %v, want each id once", reads)
	}
}

func TestLinkedInEasyApplyPassRespectsItsPageLimit(t *testing.T) {
	f := &easyApplyFake{
		primary: map[string]string{"0": linkedInRelatedListing("1")},
		easy:    map[string]string{"0": linkedInRelatedListing("2"), "1": linkedInRelatedListing("3")},
	}
	if _, err := easyApplyFetch(t, f, map[string]string{"pages": "1", "easyapply_pages": "1"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"primary:0", "easy:0"}; !slices.Equal(f.searches, want) {
		t.Errorf("searches = %v, want %v", f.searches, want)
	}
}

func TestLinkedInEasyApplyPassIsOffByDefault(t *testing.T) {
	f := &easyApplyFake{primary: map[string]string{"0": linkedInRelatedListing("1")}}
	if _, err := easyApplyFetch(t, f, map[string]string{"pages": "1"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"primary:0"}; !slices.Equal(f.searches, want) {
		t.Errorf("searches = %v, want only the primary listing", f.searches)
	}
}

func TestLinkedInEasyApplyEmptyFirstPageIsNotAnError(t *testing.T) {
	f := &easyApplyFake{primary: map[string]string{"0": linkedInRelatedListing("1")}}
	jobs, err := easyApplyFetch(t, f, map[string]string{"pages": "1", "easyapply_pages": "3"})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%d err=%v; a market with no Easy Apply postings is not a failure", len(jobs), err)
	}
}

func TestLinkedInEasyApplyRefusalKeepsProgress(t *testing.T) {
	f := &easyApplyFake{
		primary:   map[string]string{"0": linkedInRelatedListing("1")},
		easy:      map[string]string{"0": linkedInRelatedListing("2") + linkedInRelatedListing("3")},
		detailErr: map[string]error{"3": &StatusError{Method: "GET", Code: 403}},
	}
	jobs, err := easyApplyFetch(t, f, map[string]string{"pages": "1", "easyapply_pages": "2"})
	if err != nil {
		t.Fatalf("a refusal in the pass lost the crawl's progress: %v", err)
	}
	if want := []string{"1", "2"}; !slices.Equal(jobIDs(jobs), want) {
		t.Errorf("jobs = %v, want the postings read before the refusal %v", jobIDs(jobs), want)
	}
}

func TestLinkedInEasyApplyListingFailureKeepsProgress(t *testing.T) {
	f := &easyApplyFake{primary: map[string]string{"0": linkedInRelatedListing("1")}}
	s := NewLinkedIn(linkedInGetterFunc(func(ctx context.Context, raw string) (*html.Node, string, error) {
		if strings.Contains(raw, "f_AL=true") {
			return nil, "", errors.New("boom")
		}
		return f.GetHTMLResolved(ctx, raw)
	})).(linkedIn)
	s.pace = &recordingWaiter{}
	jobs, err := s.Fetch(context.Background(), CompanyEntry{Board: "go", Region: "UK", Tenants: map[string]string{"pages": "1", "easyapply_pages": "2"}})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%d err=%v; want the primary progress kept", len(jobs), err)
	}
}

// f_AL is only a hint: a card the pass lists whose detail is off-site is never on-site.
func TestLinkedInEasyApplyFilterIsNotProofOfOnsiteApply(t *testing.T) {
	offsite := strings.Replace(linkedInRelatedPosting("2"), `public_jobs_apply-link-onsite`, `public_jobs_apply-link-offsite`, 1)
	f := &easyApplyFake{
		primary: map[string]string{"0": linkedInRelatedListing("1")},
		easy:    map[string]string{"0": linkedInRelatedListing("2")},
		details: map[string]string{"2": offsite},
	}
	jobs, err := easyApplyFetch(t, f, map[string]string{"pages": "1", "easyapply_pages": "1"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ApplicationMethod{}
	for _, j := range jobs {
		got[j.ExternalID] = j.ApplyMethod
	}
	want := map[string]ApplicationMethod{"1": ApplyOnSite, "2": ApplyExternal}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("application methods = %v, want exactly %v: the detail page alone decides, never f_AL", got, want)
	}
}

func TestLinkedInEasyApplyCardsUseTheSeenSet(t *testing.T) {
	f := &easyApplyFake{
		primary: map[string]string{"0": linkedInRelatedListing("1")},
		easy:    map[string]string{"0": linkedInRelatedListing("2")},
	}
	s := NewLinkedIn(f).(linkedIn)
	s.pace = &recordingWaiter{}
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Board: "go", Region: "UK", Tenants: map[string]string{"pages": "1", "easyapply_pages": "1"}}, func(id string) bool { return id == "2" })
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(f.detailReads, "2") {
		t.Errorf("a seen Easy Apply card had its detail read: %v", f.detailReads)
	}
	var refreshed bool
	for _, j := range jobs {
		refreshed = refreshed || (j.ExternalID == "2" && j.SeenRefresh)
	}
	if !refreshed {
		t.Errorf("seen Easy Apply card not emitted as SeenRefresh: %+v", jobs)
	}
}

// The newest Easy Apply cards are mostly in the primary listing: a page made only of
// already-listed ids must not end the pass.
func TestLinkedInEasyApplyPassContinuesPastAnAllDuplicatePage(t *testing.T) {
	f := &easyApplyFake{
		primary: map[string]string{"0": linkedInRelatedListing("1") + linkedInRelatedListing("2")},
		easy: map[string]string{
			"0": linkedInRelatedListing("1") + linkedInRelatedListing("2"),
			"2": linkedInRelatedListing("3"),
		},
	}
	jobs, err := easyApplyFetch(t, f, map[string]string{"pages": "1", "easyapply_pages": "3"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"1", "2", "3"}; !slices.Equal(jobIDs(jobs), want) {
		t.Errorf("jobs = %v, want the new id behind the duplicate page %v", jobIDs(jobs), want)
	}
	if !slices.Contains(f.detailReads, "3") {
		t.Errorf("new Easy Apply id not read: %v", f.detailReads)
	}
	if want := []string{"primary:0", "easy:0", "easy:2", "easy:3"}; !slices.Equal(f.searches, want) {
		t.Errorf("searches = %v, want %v (offset advances over the duplicates; the empty page ends the pass)", f.searches, want)
	}
}

// Only duplicates, then nothing: not an error, and the primary's rule for an unrecognizable
// first page is unchanged.
func TestLinkedInEasyApplyAllDuplicateListingIsNotAnError(t *testing.T) {
	f := &easyApplyFake{
		primary: map[string]string{"0": linkedInRelatedListing("1")},
		easy:    map[string]string{"0": linkedInRelatedListing("1")},
	}
	if jobs, err := easyApplyFetch(t, f, map[string]string{"pages": "1", "easyapply_pages": "2"}); err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%d err=%v", len(jobs), err)
	}
	unrecognizable := &easyApplyFake{primary: map[string]string{"0": `<li data-entity-urn="urn:li:jobPosting:5"></li>`}}
	if _, err := easyApplyFetch(t, unrecognizable, map[string]string{"pages": "1"}); err == nil {
		t.Fatal("a primary first page with unrecognizable cards no longer fails")
	}
}
