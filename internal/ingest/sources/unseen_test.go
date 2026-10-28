package sources

import (
	"context"
	"maps"
	"slices"
	"testing"
	"time"
)

// fakeSelfClosingSource emits removal events from a change stream (see selfClosing).
type fakeSelfClosingSource struct{ provider string }

func (f fakeSelfClosingSource) Provider() string { return f.provider }

func (f fakeSelfClosingSource) Fetch(context.Context, CompanyEntry) ([]Job, error) {
	return nil, nil
}

func (f fakeSelfClosingSource) selfClosing() {}

// fakeLivenessProbedSource re-lists only the newest slice of its feed and has its older
// postings judged by a probe instead (see livenessProbed).
type fakeLivenessProbedSource struct{ provider string }

func (f fakeLivenessProbedSource) Provider() string { return f.provider }

func (f fakeLivenessProbedSource) Fetch(context.Context, CompanyEntry) ([]Job, error) {
	return nil, nil
}

func (f fakeLivenessProbedSource) aggregator()     {}
func (f fakeLivenessProbedSource) livenessProbed() {}

func TestUnseenWindowsFloorsEveryCrawledProviderAndLeavesTheRestOut(t *testing.T) {
	const floor = 14 * 24 * time.Hour
	got := UnseenWindows(reg(
		fakeSource{"greenhouse"},                         // ordinary crawl → the floor
		fakeGraceSource{"whatjobs", 14 * 24 * time.Hour}, // declares the floor itself → the floor
		fakeGraceSource{"builtin", 30 * 24 * time.Hour},  // its own sweep waits longer → its window
		fakeGraceSource{"gem", 24 * time.Hour},           // narrower sweep → never below the floor
		fakeSelfClosingSource{"jobtech"},                 // change stream: unseen is no signal → absent
		fakeLivenessProbedSource{"hh"},                   // newest-slice crawl, probed → absent
	), floor)

	want := map[string]time.Duration{
		"greenhouse": floor,
		"whatjobs":   floor,
		"builtin":    30 * 24 * time.Hour,
		"gem":        floor,
	}
	if !maps.Equal(got, want) {
		t.Errorf("UnseenWindows() = %v, want %v", got, want)
	}
}

// The members are the providers whose crawl cannot re-list every live posting it holds and
// whose removals another worker judges from the source's own pages. Pinning the set keeps a
// marker added or dropped in an adapter file a reviewed decision rather than a side effect.
func TestLivenessProbedProvidersAreTheProbedOnes(t *testing.T) {
	var got []string
	for name, src := range Taxonomy() {
		if _, ok := src.(livenessProbed); ok {
			got = append(got, name)
		}
	}
	slices.Sort(got)
	want := []string{"adzuna", "echojobs", "empregos", "hh", "himalayas", "jobicy", "remoteok"}
	if !slices.Equal(got, want) {
		t.Errorf("livenessProbed providers = %v, want %v", got, want)
	}
}
