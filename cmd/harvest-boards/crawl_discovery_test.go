package main

import (
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
