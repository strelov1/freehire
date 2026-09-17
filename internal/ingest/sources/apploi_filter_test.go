package sources

import (
	"context"
	"strings"
	"testing"
)

// A second page, distinct from apploiListJSON, so a test can tell "the API answered for THIS
// employer" from "the API answered with the same global list again".
const apploiOtherEmployerJSON = `{"data":[
{"id":"7000001","name":"Dietary Aide","description":"<p>Serve meals.</p>","city":"Dallas","state":"Texas","country":"United States","published_date":"2026-06-18T17:23:00+00:00","brand_name_with_company_only":"Somebody Else","published":true,"archived":false,"private":false}
],"limit":100,"offset":0}`

const apploiEmptyJSON = `{"data":[],"limit":1,"offset":0}`

// The incident this guard exists for, in one test.
//
// apploi's API stopped honouring its own `employer` parameter: measured 2026-09-15, a real
// id, a nonsense id and no parameter at all returned byte-identical pages — the whole global
// catalogue. The adapter believed it, and every one of 5,833 boards stored that catalogue
// under ITS OWN company. 1,565,701 rows stood for 3,024 real postings; 99.81% of them were
// duplicates, each posting filed under 3,898 different employers, and the true employer was
// never stored in any column, so nothing could be repaired afterwards.
//
// Refusing is the only safe answer. A Fetch error fails the board — it cools down and is
// retried — while returning the postings files somebody else's job under this company, which
// no later crawl can undo.
func TestApploiRefusesWhenTheEmployerFilterIsIgnored(t *testing.T) {
	// The shape of the outage: every request answers with the same list, whatever employer
	// is asked for.
	fake := (&routedHTTP{}).route("/v1/jobs", apploiListJSON)

	jobs, err := NewApploi(fake).Fetch(context.Background(),
		CompanyEntry{Company: "OnTray", Board: "41350"})

	if err == nil {
		t.Fatalf("Fetch returned %d job(s) and no error; it must refuse rather than "+
			"attribute another employer's postings to this board", len(jobs))
	}
	if jobs != nil {
		t.Errorf("Fetch returned %d job(s) alongside the error; it must return none", len(jobs))
	}
	if !strings.Contains(err.Error(), "employer") {
		t.Errorf("error should name the ignored parameter, got: %v", err)
	}
}

// The healthy shape: the sentinel employer does not exist, so the API answers empty for it.
// That is the API proving it filters, and the crawl proceeds.
func TestApploiFetchesWhenTheFilterIsHonoured(t *testing.T) {
	fake := (&routedHTTP{}).
		route("employer="+apploiFilterProbeEmployer, apploiEmptyJSON).
		route("/v1/jobs", apploiListJSON)

	jobs, err := NewApploi(fake).Fetch(context.Background(),
		CompanyEntry{Company: "OnTray", Board: "41350"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}
}

// The subtler healthy shape: the sentinel returns SOMETHING, but not this board's postings.
// That is still an API that filters — it simply had rows for the sentinel id — so refusing
// here would strand a working board.
func TestApploiFetchesWhenTheSentinelAnswersDifferently(t *testing.T) {
	fake := (&routedHTTP{}).
		route("employer="+apploiFilterProbeEmployer, apploiOtherEmployerJSON).
		route("/v1/jobs", apploiListJSON)

	jobs, err := NewApploi(fake).Fetch(context.Background(),
		CompanyEntry{Company: "OnTray", Board: "41350"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}
}

// A board with no postings must not be read as a broken filter. Both sides empty means the
// employer has nothing listed, which is an ordinary answer and not evidence of anything.
func TestApploiEmptyBoardIsNotAFilterFailure(t *testing.T) {
	fake := (&routedHTTP{}).route("/v1/jobs", apploiEmptyJSON)

	jobs, err := NewApploi(fake).Fetch(context.Background(),
		CompanyEntry{Company: "OnTray", Board: "41350"})
	if err != nil {
		t.Fatalf("an empty board is not a filter failure: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("jobs = %d, want 0", len(jobs))
	}
}
