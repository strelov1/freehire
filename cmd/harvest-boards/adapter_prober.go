package main

import (
	"context"

	"github.com/strelov1/freehire/internal/sources"
)

// adapterProber validates a board by running the real source adapter and counting the
// postings it returns. It suits platforms (Taleo, Cornerstone) whose crawl is too stateful
// (a session-bound careersection flow, or a home-page token exchange) to cheaply reimplement
// as a one-shot probe — reusing the proven adapter is both correct and DRY. These adapters
// expose no cheap employer name (the board id is an opaque tenant/host), so the empty name
// falls back to the seed-supplied company. Best-effort: a fetch error counts the board as not
// live rather than aborting the run.
type adapterProber struct {
	provider  string
	newSource func() sources.Source
}

func (a adapterProber) probe(ctx context.Context, _ httpClient, board string) (string, int, error) {
	jobs, err := a.newSource().Fetch(ctx, sources.CompanyEntry{Provider: a.provider, Board: board})
	if err != nil {
		return "", 0, nil
	}
	return "", len(jobs), nil
}
