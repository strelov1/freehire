package main

import (
	"context"
	"errors"
	"testing"
)

// scriptedProber answers each board with a canned (company, openJobs, err) triple, so the
// probeAll gate can be exercised without touching a platform API.
type scriptedProber map[string]struct {
	company  string
	openJobs int
	err      error
}

func (p scriptedProber) probe(_ context.Context, _ httpClient, slug string) (string, int, error) {
	r, ok := p[slug]
	if !ok {
		return "", 0, nil
	}
	return r.company, r.openJobs, r.err
}

func TestProbeAllNameGate(t *testing.T) {
	prober := scriptedProber{
		// A live board whose reported employer matches the seed's expectation.
		"adoreal": {company: "Adoreal", openJobs: 26},
		// A live board that belongs to somebody else entirely — the iCIMS `prequel` shape.
		"prequel": {company: "A. C. Coy", openJobs: 4},
		// A live board on a platform that reports no name of its own.
		"nameless": {company: "", openJobs: 3},
		// The Workday shape: the prober used to answer with a token derived from the board
		// id (the tenant), which is not a name the platform reported. Treating it as one
		// armed the gate against every Workday seed harvest-ats writes.
		"acme.wd1.myworkdayjobs.com/careers": {company: "", openJobs: 5},
		// A live board reached from a seed that never named an expected employer.
		"unclaimed": {company: "Some Other Name", openJobs: 7},
		// A candidate whose probe errored.
		"broken": {err: errors.New("transport failed")},
		// A live board whose seed names an employer that folds to nothing. Punctuation is
		// not an expectation, so the board must be kept rather than rejected.
		"unnameable": {company: "Real Employer", openJobs: 2},
	}
	seed := map[string]string{
		"adoreal":                            "Adoreal",
		"prequel":                            "Prequel",
		"nameless":                           "Nameless Co",
		"broken":                             "Broken",
		"acme.wd1.myworkdayjobs.com/careers": "Acme Corporation",
		"unnameable":                         "???",
	}
	candidates := []string{
		"adoreal", "prequel", "nameless", "unclaimed", "broken", "unnameable",
		"acme.wd1.myworkdayjobs.com/careers",
	}

	kept, failures, mismatches := probeAll(context.Background(), nil, prober, candidates, seed, defaultProbeWorkers)

	got := make(map[string]string, len(kept))
	for _, e := range kept {
		got[e.Board] = e.Company
	}
	if _, ok := got["prequel"]; ok {
		t.Error("a board reported under a different employer must not be kept")
	}
	if got["adoreal"] != "Adoreal" {
		t.Errorf("matching board should be kept, got %q", got["adoreal"])
	}
	if got["nameless"] != "Nameless Co" {
		t.Errorf("board whose platform reports no name should keep the seed label, got %q", got["nameless"])
	}
	if got["unclaimed"] != "Some Other Name" {
		t.Errorf("seed without an expected employer should not be gated, got %q", got["unclaimed"])
	}
	if got["acme.wd1.myworkdayjobs.com/careers"] != "Acme Corporation" {
		t.Errorf("a platform reporting no name must keep the seed label, got %q",
			got["acme.wd1.myworkdayjobs.com/careers"])
	}
	if got["unnameable"] != "Real Employer" {
		t.Errorf("an expected name that folds to nothing states no expectation, got %q",
			got["unnameable"])
	}
	if mismatches != 1 {
		t.Errorf("mismatches = %d, want 1", mismatches)
	}
	if failures != 1 {
		t.Errorf("failures = %d, want 1 (the errored probe only)", failures)
	}
}
