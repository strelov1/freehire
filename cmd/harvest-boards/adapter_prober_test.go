package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// The Cornerstone and Taleo probes run the real (network + stateful) source adapters,
// so their behavior is validated live by a harvest run, not a fake here; these guard
// only that the providers stay wired into the registry (mirroring the other prober
// registration tests).

func TestCornerstoneRegistered(t *testing.T) {
	if _, ok := probers["cornerstone"]; !ok {
		t.Fatal(`probers["cornerstone"] missing`)
	}
}

func TestTaleoRegistered(t *testing.T) {
	if _, ok := probers["taleo"]; !ok {
		t.Fatal(`probers["taleo"] missing`)
	}
}

func TestNeogovRegistered(t *testing.T) {
	if _, ok := probers["neogov"]; !ok {
		t.Fatal(`probers["neogov"] missing`)
	}
}

func TestPageupRegistered(t *testing.T) {
	if _, ok := probers["pageup"]; !ok {
		t.Fatal(`probers["pageup"] missing`)
	}
}

func TestProberForPrefersTheBespokeProber(t *testing.T) {
	p, ok := proberFor("greenhouse")
	if !ok {
		t.Fatal(`proberFor("greenhouse") not found`)
	}
	if _, fellBack := p.(adapterProber); fellBack {
		t.Error("greenhouse resolved to the adapter fallback; its own prober (cheaper, and the "+
			"only one that reports an employer name) must win", p)
	}
}

func TestProberForFallsBackToTheProvidersAdapter(t *testing.T) {
	// Platforms with a board-keyed adapter and no bespoke prober. Before the fallback these
	// were unharvestable: harvest-boards refused the provider outright.
	for _, provider := range []string{"rippling", "ukg", "hibob", "herp", "hrmos", "scalis", "selfrecruit", "humanbit", "recrutei", "recruiterflow", "friendwork", "potok"} {
		p, ok := proberFor(provider)
		if !ok {
			t.Errorf("proberFor(%q) not found; the provider has an adapter and is board-keyed", provider)
			continue
		}
		a, isAdapter := p.(adapterProber)
		if !isAdapter {
			t.Errorf("proberFor(%q) = %T, want adapterProber", provider, p)
			continue
		}
		if a.provider != provider {
			t.Errorf("proberFor(%q).provider = %q", provider, a.provider)
		}
	}
}

func TestProberForRefusesBoardlessProviders(t *testing.T) {
	// A boardless adapter serves one catalogue whatever board it is handed, so it would
	// report jobs for a board that does not exist and confirm every candidate put to it.
	for _, provider := range []string{"echojobs", "jobstash", "ozon", "staffy"} {
		if p, ok := proberFor(provider); ok {
			t.Errorf("proberFor(%q) = %T, want no prober: a boardless adapter cannot refute a board", provider, p)
		}
	}
}

func TestProberForRefusesUnknownProvider(t *testing.T) {
	if _, ok := proberFor("nosuchplatform"); ok {
		t.Error(`proberFor("nosuchplatform") found a prober`)
	}
}

// companyGatedSource mimics the adapters that drop a posting when the board entry names no
// company — hrmanager's job() and tencent's detail mapping both refuse on e.Company == "",
// because the company label comes from the catalogue row rather than the platform. Handed a
// company-less entry it returns nothing for a board that is live.
type companyGatedSource struct {
	live map[string]int
	mu   sync.Mutex
	seen []sources.CompanyEntry
}

func (*companyGatedSource) Provider() string { return "gated" }

func (s *companyGatedSource) Fetch(_ context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	s.mu.Lock()
	s.seen = append(s.seen, e)
	s.mu.Unlock()
	if e.Company == "" {
		return nil, nil
	}
	return make([]sources.Job, s.live[e.Board]), nil
}

func TestAdapterProbeFindsBoardsOfACompanyGatedAdapter(t *testing.T) {
	src := &companyGatedSource{live: map[string]int{"energinet": 32, "aab": 6}}
	p := adapterProber{provider: "gated", newSource: func() sources.Source { return src }}
	// One candidate the seed names an employer for, one it does not.
	expected := map[string]expectation{"energinet": {company: "Energinet"}}

	kept, failures, _ := probeAll(context.Background(), nil, p, []string{"energinet", "aab"}, expected, defaultProbeWorkers)

	if failures != 0 {
		t.Errorf("failures = %d, want 0", failures)
	}
	if len(kept) != 2 {
		t.Fatalf("kept = %+v, want both live boards — a company-gated adapter must not read as dead", kept)
	}
	for _, e := range src.seen {
		if e.Company == "" {
			t.Errorf("board %q probed with no company", e.Board)
		}
		if e.Board == "energinet" && e.Company != "Energinet" {
			t.Errorf("energinet probed with company %q, want the seed's %q", e.Company, "Energinet")
		}
	}
}

type failingSource struct{}

func (failingSource) Provider() string { return "failing" }

func (failingSource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return nil, errors.New("platform down")
}

func TestAdapterProbeCountsAFetchErrorAsAFailure(t *testing.T) {
	p := adapterProber{provider: "failing", newSource: func() sources.Source { return failingSource{} }}

	kept, failures, _ := probeAll(context.Background(), nil, p, []string{"a", "b"}, nil, defaultProbeWorkers)

	if len(kept) != 0 || failures != 2 {
		t.Errorf("kept=%d failures=%d, want 0 and 2 — a dead platform must not read as empty boards", len(kept), failures)
	}
}

// hydratingSource lists n postings and counts the detail requests a crawl would spend: Fetch
// hydrates everything, FetchNew only what seen does not hold.
type hydratingSource struct {
	n       int
	details int
}

func (*hydratingSource) Provider() string { return "hydrating" }

func (s *hydratingSource) Fetch(ctx context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s *hydratingSource) FetchNew(_ context.Context, _ sources.CompanyEntry, seen func(string) bool) ([]sources.Job, error) {
	out := make([]sources.Job, s.n)
	for range out {
		if !seen("x") {
			s.details++
		}
	}
	return out, nil
}

func TestAdapterProbeOfAHydratingAdapterSpendsNoDetail(t *testing.T) {
	src := &hydratingSource{n: 7}
	p := adapterProber{provider: "hydrating", newSource: func() sources.Source { return src }}

	_, n, err := p.probe(context.Background(), nil, "acme")

	if err != nil || n != 7 {
		t.Fatalf("probe = %d, %v; want the listing's 7 postings", n, err)
	}
	if src.details != 0 {
		t.Errorf("probe spent %d detail requests, want none: it counts the listing", src.details)
	}
}

// goneSource answers ErrBoardGone the way ~30 adapters report the platform's own "no such
// board".
type goneSource struct{}

func (goneSource) Provider() string { return "gone" }

func (goneSource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return nil, fmt.Errorf("kenjo: board %q: %w", "nosuch", sources.ErrBoardGone)
}

func TestAdapterProbeReadsErrBoardGoneAsASkip(t *testing.T) {
	p := adapterProber{provider: "gone", newSource: func() sources.Source { return goneSource{} }}

	name, n, err := p.probe(context.Background(), nil, "nosuch")

	if name != "" || n != 0 || err != nil {
		t.Fatalf("probe = (%q, %d, %v), want (\"\", 0, nil): the platform's own \"not a board\" is a skip, not a failure", name, n, err)
	}
}
