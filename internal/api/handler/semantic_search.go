package handler

import "github.com/strelov1/freehire/internal/search/search"

// semanticOutcome is what became of a request's opt-in `semantic` flag, which the three
// agent surfaces still accept (OJCP's `semantic: true`, /agent/jobs/search's
// `semantic=true`, the ChatGPT app's search_jobs). The index no longer carries
// description vectors, so the flag is never honoured: every request that asks is
// answered by keywords and told so.
type semanticOutcome int

const (
	// semanticNotAsked: the flag was absent or false, and the request ran as it always did.
	semanticNotAsked semanticOutcome = iota
	// semanticDropped: the flag was sent with nothing it could apply to — no text, or the
	// match sort, which ranks by its own vector. Each surface reports the flag the way it
	// reports any dropped param.
	semanticDropped
	// semanticUnavailable: the request was answered by keywords alone.
	semanticUnavailable
)

// semanticOutcomeFor decides the outcome for params in one place, so the three surfaces
// cannot disagree about it.
func semanticOutcomeFor(params search.SearchParams, asked bool) semanticOutcome {
	switch {
	case !asked:
		return semanticNotAsked
	case params.Query == "" || params.Vector != nil:
		return semanticDropped
	default:
		return semanticUnavailable
	}
}

// wire renders the outcome in a surface's own vocabulary: unavailable, or nothing when
// the flag was not asked or was dropped.
func (o semanticOutcome) wire(unavailable string) string {
	if o == semanticUnavailable {
		return unavailable
	}
	return ""
}
