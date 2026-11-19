package sources

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func (p *pagedWorkday) PostJSON(_ context.Context, _ string, _, v any) error {
	body := `{"total":0,"jobPostings":[]}`
	if p.call < len(p.pages) {
		body = p.pages[p.call]
	}
	p.call++
	return json.Unmarshal([]byte(body), v)
}

// facetedWorkday serves a listing page keyed by the request's appliedFacets (JSON-marshaled,
// which — because Go's encoding/json sorts map keys — gives a stable key regardless of build
// order). This is what lets a test control exactly what a facet-scoped re-query returns, which
// a routedHTTP (URL-only routing) cannot do: every facet-scoped query hits the same listing URL.
type facetedWorkday struct {
	*routedHTTP
	mu     sync.Mutex
	pages  map[string]string
	served map[string]bool
	calls  int
}

// A board whose first page reports the capped total (2000) must not stop there: it should
// re-query per value of the response's own facet, and return the union.
func TestWorkdaySplitsByFacetWhenCapped(t *testing.T) {
	fake := (&facetedWorkday{routedHTTP: &routedHTTP{}}).
		route(`{}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"jobFamilyGroup","values":[
				{"id":"fam-eng","count":1200},
				{"id":"fam-sales","count":800}
			]}
		]}`).
		route(`{"jobFamilyGroup":["fam-eng"]}`, `{"total":1,"jobPostings":[
			{"title":"Engineer","externalPath":"/job/X/ENG-1","locationsText":"Berlin"}
		]}`).
		route(`{"jobFamilyGroup":["fam-sales"]}`, `{"total":1,"jobPostings":[
			{"title":"Sales Rep","externalPath":"/job/X/SALES-1","locationsText":"NYC"}
		]}`)

	b, err := parseWorkdayBoard("acme.wd1.myworkdayjobs.com/Careers")
	if err != nil {
		t.Fatalf("parseWorkdayBoard: %v", err)
	}
	postings, err := workday{http: fake}.listPostings(context.Background(), b)
	if err != nil {
		t.Fatalf("listPostings: %v", err)
	}
	if len(postings) != 2 {
		t.Fatalf("len(postings) = %d, want 2 (one per facet value, capped total must not stop the walk)", len(postings))
	}
	got := map[string]bool{}
	for _, p := range postings {
		got[p.ExternalPath] = true
	}
	if !got["/job/X/ENG-1"] || !got["/job/X/SALES-1"] {
		t.Errorf("postings = %+v, want both facet slices represented", postings)
	}
}

// When more than one facet dimension is available, the split must pick the one whose largest
// single value is smallest — the dimension that leaves the smallest oversized remainder — not
// the dimension with the single biggest value anywhere. The latter was tried first and, live
// against a real capped board, kept picking a dimension whose top bucket was still ~2000-sized,
// making no real progress. Only dimB's values have routes registered: if the wrong dimension
// (dimA) were picked, the request would hit an unregistered appliedFacets key and error.
func TestWorkdaySplitPicksDimensionWithSmallestMaxValue(t *testing.T) {
	fake := (&facetedWorkday{routedHTTP: &routedHTTP{}}).
		route(`{}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"dimA","values":[{"id":"a1","count":1900},{"id":"a2","count":100}]},
			{"facetParameter":"dimB","values":[{"id":"b1","count":700},{"id":"b2","count":700},{"id":"b3","count":600}]}
		]}`).
		route(`{"dimB":["b1"]}`, `{"total":1,"jobPostings":[{"title":"B1","externalPath":"/job/X/B1-1","locationsText":"Berlin"}]}`).
		route(`{"dimB":["b2"]}`, `{"total":1,"jobPostings":[{"title":"B2","externalPath":"/job/X/B2-1","locationsText":"Berlin"}]}`).
		route(`{"dimB":["b3"]}`, `{"total":1,"jobPostings":[{"title":"B3","externalPath":"/job/X/B3-1","locationsText":"Berlin"}]}`)

	b, err := parseWorkdayBoard("acme.wd1.myworkdayjobs.com/Careers")
	if err != nil {
		t.Fatalf("parseWorkdayBoard: %v", err)
	}
	postings, err := workday{http: fake}.listPostings(context.Background(), b)
	if err != nil {
		t.Fatalf("listPostings: %v (want dimB picked — dimA's max value of 1900 is worse than dimB's 700)", err)
	}
	if len(postings) != 3 {
		t.Fatalf("len(postings) = %d, want 3 (one per dimB value)", len(postings))
	}
}

// A facet slice that is itself still capped must recurse into a second, different facet
// dimension — Workday's own facet does not rescope against itself once applied (verified live),
// so the second level has to be a dimension not yet used in this branch.
func TestWorkdaySplitsRecursesIntoSecondDimensionWhenSliceStillCapped(t *testing.T) {
	fake := (&facetedWorkday{routedHTTP: &routedHTTP{}}).
		route(`{}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"jobFamilyGroup","values":[{"id":"fam-eng","count":2000}]}
		]}`).
		route(`{"jobFamilyGroup":["fam-eng"]}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"jobFamilyGroup","values":[{"id":"fam-eng","count":2000}]},
			{"facetParameter":"workerSubType","values":[
				{"id":"full-time","count":1500},
				{"id":"contract","count":500}
			]}
		]}`).
		route(`{"jobFamilyGroup":["fam-eng"],"workerSubType":["full-time"]}`, `{"total":1,"jobPostings":[
			{"title":"Engineer FT","externalPath":"/job/X/FT-1","locationsText":"Berlin"}
		]}`).
		route(`{"jobFamilyGroup":["fam-eng"],"workerSubType":["contract"]}`, `{"total":1,"jobPostings":[
			{"title":"Engineer Contract","externalPath":"/job/X/CT-1","locationsText":"Berlin"}
		]}`)

	b, err := parseWorkdayBoard("acme.wd1.myworkdayjobs.com/Careers")
	if err != nil {
		t.Fatalf("parseWorkdayBoard: %v", err)
	}
	postings, err := workday{http: fake}.listPostings(context.Background(), b)
	if err != nil {
		t.Fatalf("listPostings: %v", err)
	}
	got := map[string]bool{}
	for _, p := range postings {
		got[p.ExternalPath] = true
	}
	if !got["/job/X/FT-1"] || !got["/job/X/CT-1"] {
		t.Errorf("postings = %+v, want both second-level facet slices represented", postings)
	}
}

// Recursion must not go deeper than maxFacetDepth combined dimensions: a slice still capped
// at that depth is paged as-is (best effort) rather than recursed further.
// A board still capped after exhausting maxFacetDepth cannot be proven complete, so the crawl
// must fail loudly rather than return the depth limit's leaf slice as if it were the whole
// board (see the fullBoardListing marker's bar, internal/ingest/sources/markers.go: an adapter
// that returns a partial listing as an unqualified success is exactly the solidjobs shape that
// forced freehire#2337's revert).
func TestWorkdaySplitFailsAtMaxDepth(t *testing.T) {
	fake := (&facetedWorkday{routedHTTP: &routedHTTP{}}).
		route(`{}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"dimA","values":[{"id":"a1","count":2000}]}
		]}`).
		route(`{"dimA":["a1"]}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"dimB","values":[{"id":"b1","count":2000}]}
		]}`).
		route(`{"dimA":["a1"],"dimB":["b1"]}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"dimC","values":[{"id":"c1","count":2000}]}
		]}`).
		// Three dimensions applied (dimA, dimB, dimC) is the depth limit; this slice is still
		// capped and carries a fourth, unused dimension, but recursion must not use it — it
		// must fail instead of paging this response as-is.
		route(`{"dimA":["a1"],"dimB":["b1"],"dimC":["c1"]}`, `{"total":2000,"jobPostings":[
			{"title":"Leaf","externalPath":"/job/X/LEAF-1","locationsText":"Remote"}
		],"facets":[
			{"facetParameter":"dimD","values":[{"id":"d1","count":2000}]}
		]}`)

	b, err := parseWorkdayBoard("acme.wd1.myworkdayjobs.com/Careers")
	if err != nil {
		t.Fatalf("parseWorkdayBoard: %v", err)
	}
	postings, err := workday{http: fake}.listPostings(context.Background(), b)
	if err == nil {
		t.Fatalf("listPostings = %+v, nil error — want a hard failure, a depth-limited leaf slice must never be returned as a complete board", postings)
	}
	if _, ok := fake.pages[`{"dimA":["a1"],"dimB":["b1"],"dimC":["c1"],"dimD":["d1"]}`]; ok {
		t.Error("a dimD route exists on the fake but must never be requested past max depth")
	}
}

// Postings that surface in more than one recursed slice (a multi-counting facet dimension can
// place the same posting under two different values) must be deduped by ExternalPath.
func TestWorkdaySplitDedupsOverlappingPostings(t *testing.T) {
	fake := (&facetedWorkday{routedHTTP: &routedHTTP{}}).
		route(`{}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"workerSubType","values":[
				{"id":"remote-eligible","count":1200},
				{"id":"full-time","count":1900}
			]}
		]}`).
		route(`{"workerSubType":["remote-eligible"]}`, `{"total":1,"jobPostings":[
			{"title":"Engineer","externalPath":"/job/X/SHARED-1","locationsText":"Remote"}
		]}`).
		route(`{"workerSubType":["full-time"]}`, `{"total":1,"jobPostings":[
			{"title":"Engineer","externalPath":"/job/X/SHARED-1","locationsText":"Remote"}
		]}`)

	b, err := parseWorkdayBoard("acme.wd1.myworkdayjobs.com/Careers")
	if err != nil {
		t.Fatalf("parseWorkdayBoard: %v", err)
	}
	postings, err := workday{http: fake}.listPostings(context.Background(), b)
	if err != nil {
		t.Fatalf("listPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("len(postings) = %d, want 1 — the same posting from two slices must be deduped", len(postings))
	}
}

// A board below the cap must page exactly as before: no appliedFacets-scoped requests at all.
func TestWorkdayBelowCapDoesNotSplit(t *testing.T) {
	fake := (&facetedWorkday{routedHTTP: &routedHTTP{}}).
		route(`{}`, `{"total":1,"jobPostings":[
			{"title":"Engineer","externalPath":"/job/X/E-1","locationsText":"Berlin"}
		]}`)

	b, err := parseWorkdayBoard("acme.wd1.myworkdayjobs.com/Careers")
	if err != nil {
		t.Fatalf("parseWorkdayBoard: %v", err)
	}
	postings, err := workday{http: fake}.listPostings(context.Background(), b)
	if err != nil {
		t.Fatalf("listPostings: %v", err)
	}
	if len(postings) != 1 || postings[0].ExternalPath != "/job/X/E-1" {
		t.Fatalf("postings = %+v, want the single below-cap posting", postings)
	}
	if fake.calls != 1 {
		t.Errorf("calls = %d, want 1 (a single unfiltered page, no facet split triggered)", fake.calls)
	}
}

// A capped page carrying no facets at all cannot be split further, and a board this size
// cannot be proven complete by paging the capped response as-is — the crawl must fail loudly,
// naming the provider and board, rather than return a silently truncated board as a success.
func TestWorkdayFailsWhenCappedWithNoUsableDimension(t *testing.T) {
	fake := (&facetedWorkday{routedHTTP: &routedHTTP{}}).
		route(`{}`, `{"total":2000,"jobPostings":[
			{"title":"Engineer","externalPath":"/job/X/E-1","locationsText":"Berlin"}
		],"facets":[]}`)

	b, err := parseWorkdayBoard("acme.wd1.myworkdayjobs.com/Careers")
	if err != nil {
		t.Fatalf("parseWorkdayBoard: %v", err)
	}
	postings, err := workday{http: fake}.listPostings(context.Background(), b)
	if err == nil {
		t.Fatalf("listPostings = %+v, nil error — want a hard failure, a capped page with no usable dimension must never be returned as a complete board", postings)
	}
	if !strings.Contains(err.Error(), "workday") || !strings.Contains(err.Error(), "acme") {
		t.Errorf("error = %q, want it to name the provider and board", err.Error())
	}
}

// Workday nests its location dimensions inside a "locationMainGroup" facet whose values are
// themselves facets, with no id and no count (verified live on michaels.wd5, 2026-10-27). The
// nested dimensions must be split on like any other — they are often the only fine-grained
// ones — and the wrapper itself, whose zero-count "values" looked like the smallest dimension
// of all, must never be chosen.
func TestWorkdaySplitsByANestedLocationFacet(t *testing.T) {
	fake := (&facetedWorkday{routedHTTP: &routedHTTP{}}).
		route(`{}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"jobFamilyGroup","values":[{"id":"f1","count":3000},{"id":"f2","count":1300}]},
			{"facetParameter":"locationMainGroup","values":[
				{"facetParameter":"locationCountry","values":[{"id":"us","count":4100}]},
				{"facetParameter":"locationRegionStateProvince","values":[{"id":"tx","count":1400},{"id":"ca","count":1300}]}
			]}
		]}`).
		route(`{"locationRegionStateProvince":["tx"]}`, `{"total":1,"jobPostings":[{"title":"TX","externalPath":"/job/X/TX-1","locationsText":"Dallas"}]}`).
		route(`{"locationRegionStateProvince":["ca"]}`, `{"total":1,"jobPostings":[{"title":"CA","externalPath":"/job/X/CA-1","locationsText":"Fresno"}]}`)

	b, err := parseWorkdayBoard("acme.wd1.myworkdayjobs.com/Careers")
	if err != nil {
		t.Fatalf("parseWorkdayBoard: %v", err)
	}
	postings, err := workday{http: fake}.listPostings(context.Background(), b)
	if err != nil {
		t.Fatalf("listPostings: %v (want the nested locationRegionStateProvince picked)", err)
	}
	if len(postings) != 2 {
		t.Fatalf("len(postings) = %d, want 2 (one per state)", len(postings))
	}
}

// A facet whose values do not add up to the capped slice cannot partition it: Workday's
// "distance" facet (10/25/50 miles) sums to 5 postings on a 4,390-posting board (michaels.wd5,
// measured 2026-10-27). Its tiny largest value made it look like the best split, and taking it
// listed 548 postings and reported success — a silent truncation the unseen sweep would read as
// thousands of removals.
func TestWorkdaySplitIgnoresAFacetThatDoesNotCoverTheBoard(t *testing.T) {
	fake := (&facetedWorkday{routedHTTP: &routedHTTP{}}).
		route(`{}`, `{"total":2000,"jobPostings":[],"facets":[
			{"facetParameter":"distance","values":[{"id":"d10","count":1},{"id":"d25","count":1}]},
			{"facetParameter":"state","values":[{"id":"tx","count":1500},{"id":"ca","count":1400}]}
		]}`).
		route(`{"state":["tx"]}`, `{"total":1,"jobPostings":[{"title":"TX","externalPath":"/job/X/TX-1","locationsText":"Dallas"}]}`).
		route(`{"state":["ca"]}`, `{"total":1,"jobPostings":[{"title":"CA","externalPath":"/job/X/CA-1","locationsText":"Fresno"}]}`)

	b, err := parseWorkdayBoard("acme.wd1.myworkdayjobs.com/Careers")
	if err != nil {
		t.Fatalf("parseWorkdayBoard: %v", err)
	}
	postings, err := workday{http: fake}.listPostings(context.Background(), b)
	if err != nil {
		t.Fatalf("listPostings: %v (want state picked, never distance)", err)
	}
	if len(postings) != 2 {
		t.Fatalf("len(postings) = %d, want 2 (one per state)", len(postings))
	}
}
