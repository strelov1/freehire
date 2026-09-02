package main

import (
	"context"
	"strings"
	"testing"
)

// edjoinProbeURL is the one URL the prober builds for a job type, spelled out so a change to the
// query the endpoint requires shows up as a test failure rather than as an empty harvest.
func edjoinProbeURL(jobType string) string {
	return "https://www.edjoin.org/Home/LoadJobs?catID=0&days=0&districtID=0&jobTypes=" + jobType +
		"&order=desc&page=1&recruitmentCenterID=0&rows=1&searchType=all&sort=postingDate&sortVal=0"
}

func TestEdjoinProbe(t *testing.T) {
	p := edjoinProber{}
	getter := fakeGetter{
		edjoinProbeURL("25"): `{"totalPages":71,"totalRecords":71,"displayRecords":1,` +
			`"data":[{"postingID":2243913,"positionTitle":"Network Technician"}]}`,
		// An unknown job type is answered 200 with an empty slice, not an error.
		edjoinProbeURL("9901"): `{"totalPages":0,"totalRecords":0,"displayRecords":0,"data":[]}`,
	}
	// A live board is counted by totalRecords, not by the single row asked for, and the label
	// is left to the seed: one board spans hundreds of districts, so no employer name fits it.
	if name, n, err := p.probe(context.Background(), getter, "25"); err != nil || name != "" || n != 71 {
		t.Errorf("live: got (%q,%d,%v), want (\"\",71,nil)", name, n, err)
	}
	// A job type EDJOIN does not have reads as an empty slice, which is not a board to commit.
	if _, n, err := p.probe(context.Background(), getter, "9901"); err != nil || n != 0 {
		t.Errorf("unknown: got n=%d err=%v, want 0,nil", n, err)
	}
	// An unreachable endpoint is "not a live board" for harvest, not a reason to abort the run.
	if _, n, err := p.probe(context.Background(), getter, "42"); err != nil || n != 0 {
		t.Errorf("unreachable: got n=%d err=%v, want 0,nil", n, err)
	}
	// A board is a numeric job-type id. Anything else addresses no slice, so it is refused
	// without a request rather than sent to the endpoint to be answered 500 or 200-with-zero.
	for _, bad := range []string{"", "  ", "abc", "25/2", "25a", "-1"} {
		if _, n, err := p.probe(context.Background(), getter, bad); err != nil || n != 0 {
			t.Errorf("malformed %q: got n=%d err=%v, want 0,nil", bad, n, err)
		}
	}
	// The board id is trimmed, so a seed's stray whitespace still probes the right slice.
	if _, n, err := p.probe(context.Background(), getter, " 25 "); err != nil || n != 71 {
		t.Errorf("padded: got n=%d err=%v, want 71,nil", n, err)
	}
}

// edjoinRecordingGetter answers nothing and remembers the URL it was asked for, so a test can
// assert on the request the prober actually built rather than on one the test built beside it.
type edjoinRecordingGetter struct {
	httpClient
	url string
}

func (g *edjoinRecordingGetter) GetJSON(_ context.Context, url string, _ any) error {
	g.url = url
	return errMissing
}

// The endpoint dereferences catID, districtID and recruitmentCenterID without a null check, so a
// probe that drops one is answered a 500 error page and reads as a dead board — a failure that
// looks exactly like a harvest that found nothing. This asserts on the URL the prober itself
// emits: pinning a string the test also wrote would pass however the prober changed.
func TestEdjoinProbeSendsMandatoryFilters(t *testing.T) {
	g := &edjoinRecordingGetter{}
	if _, _, err := (edjoinProber{}).probe(context.Background(), g, "25"); err != nil {
		t.Fatalf("probe: %v", err)
	}
	for _, want := range []string{
		"catID=0", "districtID=0", "recruitmentCenterID=0", "rows=1", "jobTypes=25",
	} {
		if !strings.Contains(g.url, want) {
			t.Errorf("probe requested %q, which is missing %q", g.url, want)
		}
	}
}

func TestEdjoinProberRegistered(t *testing.T) {
	p, ok := proberFor("edjoin")
	if !ok {
		t.Fatal(`proberFor("edjoin") not found`)
	}
	// The bespoke prober must win. The adapter fallback runs edjoin.Fetch, which hydrates a
	// ~147 KB detail page for every posting in the probed slice — probing one of the platform's
	// larger job types that way would spend hundreds of megabytes to learn a number the listing
	// states in one request.
	if _, fellBack := p.(adapterProber); fellBack {
		t.Errorf("edjoin resolved to the adapter fallback, got %T", p)
	}
}
