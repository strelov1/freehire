package main

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/atsboard"
)

// Each pattern must point at hosts the recogniser maps back to the same provider, or the sweep
// would read the index and yield nothing.
func TestCrawlPatternsMatchRecogniser(t *testing.T) {
	for provider, patterns := range crawlPatterns {
		p, ok := probers[provider]
		if !ok {
			t.Errorf("%s: no prober", provider)
			continue
		}
		if _, own := p.(discoverer); own {
			t.Errorf("%s: already discovers by itself; drop its crawlPatterns entry", provider)
		}
		if _, ok := crawlDiscovery(provider, p).(discoverer); !ok {
			t.Errorf("%s: crawlDiscovery added no discover", provider)
		}
		for _, pattern := range patterns {
			var sample string
			if host, ok := strings.CutPrefix(pattern, "*."); ok {
				sample = "https://acme." + host + "/careers/123"
			} else {
				sample = "https://" + strings.TrimSuffix(pattern, "*") + "acme/123"
			}
			if src, board, _, ok := atsboard.Recognize(sample); !ok || src != provider || board == "" {
				t.Errorf("%s: %s recognised as %q/%q (ok=%v)", provider, sample, src, board, ok)
			}
		}
	}
}

func TestCrawlDiscoverySweepsEveryPattern(t *testing.T) {
	const cdx = "https://index.commoncrawl.org/CC-MAIN-2026-30-index?url="
	f := fakeGetter{
		"https://index.commoncrawl.org/collinfo.json": commonCrawlCollInfoBody,
		// Only the newest snapshot answers; the other two fail and are skipped.
		cdx + "jobs.smartrecruiters.com/*&output=json&showNumPages=true&pageSize=1":    `{"pages":1}`,
		cdx + "jobs.smartrecruiters.com/*&output=json&page=0&pageSize=1":               `{"url": "https://jobs.smartrecruiters.com/Acme/123"}`,
		cdx + "careers.smartrecruiters.com/*&output=json&showNumPages=true&pageSize=1": `{"pages":1}`,
		cdx + "careers.smartrecruiters.com/*&output=json&page=0&pageSize=1":            `{"url": "https://careers.smartrecruiters.com/Beta"}`,
	}
	d, ok := crawlDiscovery("smartrecruiters", probers["smartrecruiters"]).(discoverer)
	if !ok {
		t.Fatal("smartrecruiters: no discover")
	}
	got, err := d.discover(context.Background(), f)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if want := []string{"Acme", "Beta"}; !slices.Equal(got, want) {
		t.Errorf("discover = %v, want %v", got, want)
	}

	if _, err := d.discover(context.Background(), fakeGetter{}); err == nil {
		t.Error("discover with an unreachable index: want an error")
	}
}

func TestCrawlDiscoveryPassesThrough(t *testing.T) {
	if _, wrapped := crawlDiscovery("greenhouse", probers["greenhouse"]).(crawlDiscoverer); wrapped {
		t.Error("greenhouse discovers by itself and must not be wrapped")
	}
	if _, wrapped := crawlDiscovery("no-such-provider", probers["lever"]).(crawlDiscoverer); wrapped {
		t.Error("a provider with no crawlPatterns must not be wrapped")
	}
}
