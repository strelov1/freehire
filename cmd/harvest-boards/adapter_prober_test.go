package main

import "testing"

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
	for _, provider := range []string{"rippling", "zohorecruit", "ukg", "manatal", "phenom", "hibob"} {
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
	for _, provider := range []string{"echojobs", "jobstash", "ozon"} {
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
