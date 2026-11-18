package main

import (
	"context"
	"slices"
	"sync"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// proberFor resolves the prober for a provider: the bespoke one when probers names it, else
// the provider's own source adapter run as a probe.
//
// The fallback exists because probers is hand-maintained and lags internal/ingest/sources: a
// platform missing from it is one whose boards nobody can propose, not one we cannot crawl. A
// bespoke prober still wins (one cheap request instead of a full crawl, and on some platforms
// the employer name that powers the corroboration gate).
//
// The fallback does not inherit -pace or the refusal counter: the adapter carries its own
// sources.Client, bypassing the run's paced, counting client.
func proberFor(provider string) (prober, bool) {
	if p, ok := probers[provider]; ok {
		return p, true
	}
	reg := adapterRegistry()
	src, ok := reg[provider]
	if !ok || !slices.Contains(sources.BoardKeyedProviders(reg), provider) {
		return nil, false
	}
	return adapterProber{provider: provider, newSource: func() sources.Source { return src }}, true
}

// adapterRegistry builds the source registry once and shares it across probes, which is how
// cmd/ingest uses it too — the adapters hold a concurrency-safe client and no per-crawl state.
var adapterRegistry = sync.OnceValue(func() map[string]sources.Source {
	return sources.All(sources.NewClient())
})

// adapterProber validates a board by running the real source adapter and counting the
// postings it returns. It suits platforms (Taleo, Cornerstone) whose crawl is too stateful to
// reimplement as a one-shot probe. These adapters expose no cheap employer name, so the empty
// name falls back to the seed-supplied company.
//
// A fetch error is returned so probeAll counts it as a probe failure: an adapter error is as
// likely a dead platform or broken adapter as a dead board, and "0 postings" would hide that.
type adapterProber struct {
	provider  string
	newSource func() sources.Source
}

func (a adapterProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	return a.probeSeeded(ctx, c, board, "")
}

// probeSeeded runs the adapter with the company the seed names for the board. The entry must
// carry SOME company: several adapters (hrmanager, tencent) take the employer from the
// catalogue row and drop every posting when it is empty, so a company-less probe reads every
// live board as empty. With no seed company the board id stands in, as in chooseCompany.
func (a adapterProber) probeSeeded(ctx context.Context, _ httpClient, board, company string) (string, int, error) {
	if company == "" {
		company = board
	}
	e := sources.CompanyEntry{Provider: a.provider, Board: board, Company: company}
	src := a.newSource()
	// A hydrating adapter is asked for its listing alone: every posting reads as already held,
	// so the probe skips detail requests whose bodies it would throw away.
	if hs, ok := src.(sources.HydratingSource); ok {
		jobs, err := hs.FetchNew(ctx, e, func(string) bool { return true })
		if err != nil {
			return "", 0, err
		}
		return "", len(jobs), nil
	}
	jobs, err := src.Fetch(ctx, e)
	if err != nil {
		return "", 0, err
	}
	return "", len(jobs), nil
}
