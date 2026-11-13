package handler

import (
	"context"
	"net/url"

	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// The two reads every agent surface (ChatGPT app, OJCP, the in-app assistant) makes the same
// way: run a catalogue search, load one posting. Each caller keeps its own projection and
// output shape; only the chain in front of it lives here.

// runAgentSearch runs the query the public search runs, from the same values, and records
// the hits as served to an agent. The caller supplies the page and projects the result.
//
// There is no sort directive and no match vector, so a text query gets the index's own
// relevance ordering and an empty one the freshest first (defaultSort). The outcome says
// what became of the opt-in `semantic` flag; the surface reports it in its own vocabulary.
func runAgentSearch(ctx context.Context, s searcher, values url.Values, limit, offset int, semanticAsked bool) (search.SearchResult, semanticOutcome, error) {
	params := search.SearchParams{
		Query:  values.Get("q"),
		Filter: search.FilterFromValues(values),
		Sort:   defaultSort(values.Get("q")),
		Limit:  limit,
		Offset: offset,
	}
	res, err := s.Search(ctx, params)
	outcome := semanticOutcomeFor(params, semanticAsked)
	if err != nil {
		return search.SearchResult{}, outcome, err
	}
	agentview.Record(ctx, agentview.KindSearch, hitIDs(res.Hits))
	return res, outcome, nil
}

// loadAgentJob loads one posting through load (agentJobBySlug or publishedJobBySlug, the
// surface's own visibility rule), projects it to the wire shape, hangs the liveness signals
// on it when attach is set, and records the read. The error is load's, untouched: a missing
// row is still pgx.ErrNoRows for the caller to word.
func loadAgentJob(ctx context.Context, load func(context.Context, string) (db.Job, error), attach func(context.Context, db.Job, *jobview.Job), slug string) (db.Job, jobview.Job, error) {
	row, err := load(ctx, slug)
	if err != nil {
		return db.Job{}, jobview.Job{}, err
	}
	view, err := jobview.FromRow(row)
	if err != nil {
		return db.Job{}, jobview.Job{}, err
	}
	// Computed on read: the index's reality class is frozen at push time and the ghost
	// signal is never stored.
	if attach != nil {
		attach(ctx, row, &view)
	}
	agentview.Record(ctx, agentview.KindDetail, []int64{row.ID})
	return row, view, nil
}

// markdownJobs renders the hits as the assistant's tools hand them to a model: full
// descriptions read from Postgres, as markdown.
func (h *searchHandlers) markdownJobs(ctx context.Context, hits []search.JobDocument) ([]jobview.Job, error) {
	if len(hits) > 0 {
		if err := h.hydrateDescriptions(ctx, hits); err != nil {
			return nil, err
		}
	}
	jobs := make([]jobview.Job, len(hits))
	for i, hit := range hits {
		hit.Description = formatDescription(hit.Description, "markdown")
		jobs[i] = hit.Job
	}
	return jobs, nil
}

// noted appends "semantic" to ignored when the flag was dropped, so it reads like any other
// param the request carried that did nothing.
func (o semanticOutcome) noted(ignored []string) []string {
	if o == semanticDropped {
		return append(ignored, "semantic")
	}
	return ignored
}
