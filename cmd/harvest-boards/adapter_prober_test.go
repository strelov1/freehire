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
