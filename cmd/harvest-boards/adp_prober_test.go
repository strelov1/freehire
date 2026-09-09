package main

import (
	"context"
	"strings"
	"testing"
)

// headerSpy wraps a fakeGetter and records the headers each call carried, for the one thing a
// header-dropping fake cannot show: that the ADP MyJobs listing is actually authorised.
type headerSpy struct {
	fakeGetter
	sent []map[string]string
}

func (h *headerSpy) GetJSONWithHeaders(ctx context.Context, url string, headers map[string]string, v any) error {
	h.sent = append(h.sent, headers)
	return h.fakeGetter.GetJSONWithHeaders(ctx, url, headers, v)
}

const (
	adpMyJobsProbeSiteURL = "https://myjobs.adp.com/public/staffing/v1/career-site/acme"
	adpMyJobsProbeListURL = "https://my.adp.com/myadp_prefix/mycareer/public/staffing/v1/job-requisitions?%24top=1&%24skip=0"
)

// The page size is the one measured fact this prober rests on: ADP answers "$top=1" with an
// empty array and no meta, which a liveness check reads as a dead board. A future edit that
// "optimises" it back to one row would report every ADP board dead while the crawl reads them
// fine, and no test over a fake can catch that — so the guard is on the URL itself.
func TestADPProbeAsksForAPageADPActuallyServes(t *testing.T) {
	got := adpProbeURL("thecid", "9201_2")
	if !strings.Contains(got, "%24top=50") {
		t.Errorf("probe URL = %q, want the adapter's page size; ADP serves no page of 1", got)
	}
	if !strings.Contains(got, "cid=thecid") || !strings.Contains(got, "ccId=9201_2") {
		t.Errorf("probe URL = %q, want both halves of the board", got)
	}
}

func TestADPProberReadsTheCount(t *testing.T) {
	fake := fakeGetter{
		adpProbeURL("thecid", "9201_2"): `{"meta":{"totalNumber":329},"jobRequisitions":[{"itemID":"1"}]}`,
	}
	name, n, err := (adpProber{}).probe(context.Background(), fake, "thecid:9201_2")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if n != 329 {
		t.Errorf("openJobs = %d, want the platform's totalNumber", n)
	}
	// The list response carries no account record, so there is no name to gate on.
	if name != "" {
		t.Errorf("company = %q, want empty — this endpoint publishes none", name)
	}
}

// A tenant whose configuration omits the count still reads as live off the rows it returned,
// rather than being discarded over a field that is not always there.
func TestADPProberFallsBackToTheRowsWhenTheCountIsAbsent(t *testing.T) {
	fake := fakeGetter{
		adpProbeURL("thecid", "9201_2"): `{"jobRequisitions":[{"itemID":"1"},{"itemID":"2"}]}`,
	}
	_, n, err := (adpProber{}).probe(context.Background(), fake, "thecid:9201_2")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if n != 2 {
		t.Errorf("openJobs = %d, want the row count", n)
	}
}

// A board that is not the pair this platform uses is declined, not errored: a seed may propose
// a shape the platform does not address, and that is an answer rather than a failure.
func TestADPProberDeclinesABoardThatIsNotAPair(t *testing.T) {
	fake := fakeGetter{}
	for _, board := range []string{"", "justacid", ":9201_2", "thecid:"} {
		_, n, err := (adpProber{}).probe(context.Background(), fake, board)
		if err != nil || n != 0 {
			t.Errorf("probe(%q) = (%d, %v), want (0, nil)", board, n, err)
		}
	}
}

func TestADPMyJobsProberReadsTheSiteThenTheCount(t *testing.T) {
	spy := &headerSpy{fakeGetter: fakeGetter{
		adpMyJobsProbeSiteURL: `{"orgoid":"ORG1","clientName":"Acme","name":"Acme External CX"}`,
		adpMyJobsProbeListURL: `{"count":464,"jobRequisitions":[{"reqId":"1"}]}`,
	}}

	name, n, err := (adpMyJobsProber{}).probe(context.Background(), spy, "acme")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if n != 464 {
		t.Errorf("openJobs = %d, want the site's count", n)
	}
	// MyJobs is the one ADP product that names the employer, so this prober can feed the
	// corroboration gate its sibling has to stand down. clientName wins over the site's own
	// name, which reads as a board ("Acme External CX") rather than an employer.
	if name != "Acme" {
		t.Errorf("company = %q, want the site's clientName", name)
	}
	if len(spy.sent) != 2 {
		t.Fatalf("made %d calls, want 2", len(spy.sent))
	}
	if got := spy.sent[1]["orgoid"]; got != "ORG1" {
		t.Errorf("the listing carried orgoid %q, want the one the site published", got)
	}
}

// No orgoid means the listing cannot be authorised, so the board cannot be crawled even if
// postings sit behind it. Reporting it dead is the honest answer, and it must not go on to ask.
func TestADPMyJobsProberDeclinesASiteWithNoOrgoid(t *testing.T) {
	spy := &headerSpy{fakeGetter: fakeGetter{
		adpMyJobsProbeSiteURL: `{"clientName":"Acme"}`,
		adpMyJobsProbeListURL: `{"count":464,"jobRequisitions":[{"reqId":"1"}]}`,
	}}

	name, n, err := (adpMyJobsProber{}).probe(context.Background(), spy, "acme")
	if err != nil || n != 0 || name != "" {
		t.Errorf(`probe = (%q, %d, %v), want ("", 0, nil)`, name, n, err)
	}
	if len(spy.sent) != 1 {
		t.Errorf("made %d calls; it must stop after the career site", len(spy.sent))
	}
}

func TestADPMyJobsProberDeclinesAnEmptySlug(t *testing.T) {
	spy := &headerSpy{fakeGetter: fakeGetter{}}
	if _, n, err := (adpMyJobsProber{}).probe(context.Background(), spy, ""); n != 0 || err != nil {
		t.Errorf(`probe("") = (%d, %v), want (0, nil)`, n, err)
	}
	if len(spy.sent) != 0 {
		t.Errorf("made %d calls for an empty slug", len(spy.sent))
	}
}

// Both ADP products fold their board to lower case, because both platforms resolve either
// spelling to the same board and the catalogue holds one of them.
func TestADPProbersFoldTheirBoards(t *testing.T) {
	if got := (adpProber{}).dedupKey("8A680274-0B2E:9201_2"); got != "8a680274-0b2e:9201_2" {
		t.Errorf("adp dedupKey = %q", got)
	}
	if got := (adpMyJobsProber{}).dedupKey("StellantisExternalCX"); got != "stellantisexternalcx" {
		t.Errorf("adpmyjobs dedupKey = %q", got)
	}
}

// Both must be reachable by provider key, or a harvest falls back to running the whole source
// adapter per candidate — a crawl each, which for a 6,000-board seed is hours past the unit's
// timeout, and harvest-boards persists only at the end.
func TestADPProbersAreRegistered(t *testing.T) {
	for _, provider := range []string{"adp", "adpmyjobs"} {
		p, ok := proberFor(provider)
		if !ok {
			t.Fatalf("no prober for %q", provider)
		}
		if _, viaAdapter := p.(adapterProber); viaAdapter {
			t.Errorf("%q falls back to the adapter probe; it has a prober of its own", provider)
		}
	}
}
