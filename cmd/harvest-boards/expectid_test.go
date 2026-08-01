package main

import (
	"context"
	"testing"
)

// scriptedIDProber is a scriptedProber that also reports each board's live posting ids, so
// the expected-id gate can be exercised without touching a platform API.
type scriptedIDProber struct {
	scriptedProber
	ids map[string][]string
}

func (p scriptedIDProber) postingIDs(_ context.Context, _ httpClient, slug string) ([]string, error) {
	return p.ids[slug], nil
}

func TestPostingIDsPerProvider(t *testing.T) {
	cases := []struct {
		name   string
		prober prober
		client fakeGetter
		slug   string
		want   []string
	}{
		{
			name:   "greenhouse",
			prober: greenhouseProber{},
			client: fakeGetter{
				"https://boards-api.greenhouse.io/v1/boards/acme/jobs": `{"jobs":[{"id":4698693006},{"id":4698693007}]}`,
			},
			slug: "acme",
			want: []string{"4698693006", "4698693007"},
		},
		{
			name:   "lever",
			prober: leverProber{},
			client: fakeGetter{
				"https://api.lever.co/v0/postings/acme?mode=json": `[{"id":"c2627bcd-915c-4076-98f5-000000000000"}]`,
			},
			slug: "acme",
			want: []string{"c2627bcd-915c-4076-98f5-000000000000"},
		},
		{
			name:   "ashby",
			prober: ashbyProber{},
			client: fakeGetter{
				"https://api.ashbyhq.com/posting-api/job-board/acme": `{"jobs":[{"id":"01e5aa43-0c4c-4655-be41-000000000000"}]}`,
			},
			slug: "acme",
			want: []string{"01e5aa43-0c4c-4655-be41-000000000000"},
		},
		{
			name:   "recruitee",
			prober: recruiteeProber{},
			client: fakeGetter{
				"https://acme.recruitee.com/api/offers/": `{"offers":[{"id":99580,"company_name":"Acme"}]}`,
			},
			slug: "acme",
			want: []string{"99580"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ip, ok := tc.prober.(idProber)
			if !ok {
				t.Fatalf("%s prober does not report posting ids", tc.name)
			}
			got, err := ip.postingIDs(context.Background(), tc.client, tc.slug)
			if err != nil {
				t.Fatalf("postingIDs: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("id[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestPostingIDsPartialListersStayInert(t *testing.T) {
	// SmartRecruiters is probed with limit=1 and Teamtailor reads one page, so an id absent
	// from what they return would not mean an absent posting. They must not claim otherwise.
	for _, p := range []prober{smartRecruitersProber{}, teamtailorProber{}} {
		if _, ok := p.(idProber); ok {
			t.Errorf("%T lists postings partially and must not implement idProber", p)
		}
	}
}

func TestProbeAllExpectedPostingIDGate(t *testing.T) {
	prober := scriptedIDProber{
		scriptedProber: scriptedProber{
			// The board really does carry the posting the seed expects.
			"acme": {company: "Acme Inc", openJobs: 12},
			// A live board that simply belongs to somebody else — the hazard an offline-derived
			// slug creates, and the reason the id is checked at all.
			"doodle": {company: "Doodle Digital", openJobs: 3},
			// A live board whose seed expects an id AND names an employer the platform reports
			// differently. The id is evidence; the name is a resemblance.
			"picnic": {company: "Picnic Technologies BV", openJobs: 8},
		},
		ids: map[string][]string{
			"acme":   {"4698693006", "4698693007"},
			"doodle": {"111", "222"},
			"picnic": {"5276503008"},
		},
	}
	expected := map[string]expectation{
		"acme":   {postingID: "4698693006"},
		"doodle": {postingID: "4698693006"},
		"picnic": {company: "Totally Different Name", postingID: "5276503008"},
	}
	candidates := []string{"acme", "doodle", "picnic"}

	kept, failures, mismatches := probeAll(context.Background(), nil, prober, candidates, expected, defaultProbeWorkers)

	got := make(map[string]string, len(kept))
	for _, e := range kept {
		got[e.Board] = e.Company
	}
	if got["acme"] != "Acme Inc" {
		t.Errorf("a board carrying the expected posting should be kept, got %q", got["acme"])
	}
	if _, ok := got["doodle"]; ok {
		t.Error("a board that does not carry the expected posting must not be kept")
	}
	if got["picnic"] != "Picnic Technologies BV" {
		t.Errorf("an expected id should decide the board over a name resemblance, got %q", got["picnic"])
	}
	if mismatches != 1 {
		t.Errorf("mismatches = %d, want 1", mismatches)
	}
	if failures != 0 {
		t.Errorf("failures = %d, want 0", failures)
	}
}

func TestProbeAllExpectedPostingIDIsInertWithoutIDProber(t *testing.T) {
	// The same expectation against a prober that reports no posting ids at all. The board is
	// validated on liveness alone: supplying an id must never be worse than omitting one.
	prober := scriptedProber{"acme": {company: "Acme Inc", openJobs: 12}}
	expected := map[string]expectation{"acme": {postingID: "no-such-posting"}}

	kept, failures, mismatches := probeAll(context.Background(), nil, prober, []string{"acme"}, expected, defaultProbeWorkers)

	if len(kept) != 1 || kept[0].Company != "Acme Inc" {
		t.Errorf("an expected id must be inert on a prober that reports none, kept %v", kept)
	}
	if failures != 0 || mismatches != 0 {
		t.Errorf("failures = %d, mismatches = %d, want 0 and 0", failures, mismatches)
	}
}
