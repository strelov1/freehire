package main

import (
	"slices"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// registeredAgeFallback includes every classified market, even without a local crawl credential.
func registeredAgeFallback() ([]string, map[string]time.Duration) {
	registry := sources.Taxonomy()
	return matchingProviders(providerKeys(registry), expireDespiteRegisteredPrefixes), sources.SweepGraceWindows(registry)
}

// Classification must not depend on whether this host can authenticate a crawl.
func classifiedProviderKeys() []string {
	return providerKeys(sources.Taxonomy())
}

// providerKeys returns the keys of a crawl or classification registry.
func providerKeys(registry map[string]sources.Source) []string {
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	return keys
}

// filterSources narrows a source list to sourceFilter for a debug run (-source=X);
// an empty sourceFilter returns list unchanged, and a filter absent from list returns
// nil rather than list, so a run scoped to a source outside this list touches none of it.
func filterSources(list []string, sourceFilter string) []string {
	if sourceFilter == "" {
		return list
	}
	if slices.Contains(list, sourceFilter) {
		return []string{sourceFilter}
	}
	return nil
}

// unseenWindow reports how long these sources' postings must have gone unseen by the crawl
// before the age rule may presume them filled (the second clock CloseStaleUnseenUnprobeableJobs
// takes).
//
// It is the provider's own post-run sweep window (sources.SweepGraceWindows, else
// DefaultSweepGrace): a posting past it is one CloseUnseenJobs would have closed had its
// company_slug scope reached it. Reading it from the registry keeps the two from drifting.
//
// Where the named sources disagree it takes the WIDEST of their windows. That is the
// under-closing bias every other verdict in this worker takes: this close rests on a
// guess, so where the evidence is mixed the guess waits for the most patient of them.
func unseenWindow(srcs []string, declared map[string]time.Duration) time.Duration {
	window := sources.DefaultSweepGrace
	for _, s := range srcs {
		if d, ok := declared[s]; ok && d > window {
			window = d
		}
	}
	return window
}

// unmatchedPrefixes returns the configured prefixes that match no registered provider, a
// drift guard matchingProviders cannot give (an empty match looks like nothing to close).
//
// This matters beyond typos: every expireDespiteRegisteredPrefixes member is credential-gated
// in sources.All, so on a host without the credential the source would silently drop out of
// the age rule and into the orphan probe, leaving its postings open forever.
func unmatchedPrefixes(providers, prefixes []string) []string {
	var out []string
	for _, prefix := range prefixes {
		if len(matchingProviders(providers, []string{prefix})) == 0 {
			out = append(out, prefix)
		}
	}
	return out
}

// matchingProviders returns every entry of providers that equals one of prefixes or has
// it as a "<prefix>-" prefix — how a provider family sharing one adapter across many
// per-market registry keys (e.g. whatjobs' one row per country, see
// expireDespiteRegisteredPrefixes) is matched without enumerating every key.
func matchingProviders(providers, prefixes []string) []string {
	var out []string
	for _, p := range providers {
		for _, prefix := range prefixes {
			if p == prefix || strings.HasPrefix(p, prefix+"-") {
				out = append(out, p)
				break
			}
		}
	}
	return out
}
