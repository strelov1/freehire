package main

import (
	"context"
	"testing"
)

// fakeDiscoverer is a prober that also discovers a canned candidate list, standing in for
// a discovery-capable provider (e.g. gupy) without hitting any API.
type fakeDiscoverer struct{ ids []string }

func (fakeDiscoverer) probe(context.Context, httpClient, string) (string, int, error) {
	return "", 0, nil
}

func (f fakeDiscoverer) discover(context.Context, httpClient) ([]string, error) {
	return f.ids, nil
}

func TestResolveCandidatesDiscoversWhenNoSeed(t *testing.T) {
	got, _, err := resolveCandidates(context.Background(), fakeDiscoverer{ids: []string{"316", "89896"}}, nil, "")
	if err != nil {
		t.Fatalf("resolveCandidates: %v", err)
	}
	if len(got) != 2 || got[0] != "316" || got[1] != "89896" {
		t.Errorf("candidates = %v, want discovered [316 89896]", got)
	}
}

func TestResolveCandidatesNonDiscovererNeedsSeed(t *testing.T) {
	_, _, err := resolveCandidates(context.Background(), greenhouseProber{}, nil, "")
	if err == nil {
		t.Error("a non-discoverer prober with no seed file should error, not silently no-op")
	}
}
