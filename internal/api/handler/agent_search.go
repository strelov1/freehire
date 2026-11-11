package handler

import (
	"context"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/htmltext"
	"github.com/strelov1/freehire/internal/search/search"
)

// jobDescriptions loads full job descriptions by internal id, to rehydrate the
// truncated preview the search index serves. *db.Queries satisfies it; tests
// inject a fake.
type jobDescriptions interface {
	GetJobDescriptionsByIDs(ctx context.Context, ids []int64) ([]db.GetJobDescriptionsByIDsRow, error)
}

// AgentSearchJobs is the programmatic-consumer variant of SearchJobs: it runs the
// exact same query (via the shared runJobSearch core) and always rehydrates each
// result with the full description from Postgres, in a selectable format. The
// truncated preview stays exclusive to /jobs/search — a caller that wants the
// lightweight variant uses that endpoint instead. Public and unauthenticated like
// the other job reads; full descriptions are already public via the detail
// endpoint.
//
// `semantic=true` is still accepted but never honoured (see semanticOutcome):
// meta.semantic is then "unavailable", the request having been answered by keywords.
// Omitted when not asked.
func (h *searchHandlers) AgentSearchJobs(ctx context.Context, _ apigen.AgentSearchJobsParams) (*apigen.AgentJobSearchPageHeaders, error) {
	vals := contractQuery(ctx)
	userID, _ := auth.UserIDFromContext(ctx)
	out, err := h.runJobSearch(ctx, vals, userID, wantsParam(vals, "semantic"), nil, "", nil)
	if err != nil {
		return nil, err
	}
	res := out.res
	if len(res.Hits) > 0 {
		if err := h.hydrateDescriptions(ctx, res.Hits); err != nil {
			return nil, err
		}
	}
	// Recorded only once the answer is certain to be served: a failed hydration serves no
	// postings, so it must not count them as seen.
	agentview.Record(ctx, agentview.KindSearch, hitIDs(res.Hits))

	format := vals.Get("description_format")
	views := make([]jobview.Job, len(res.Hits))
	for i, hit := range res.Hits {
		hit.Description = formatDescription(hit.Description, format)
		views[i] = hit.Job
	}

	data, err := marshalRaws(views)
	if err != nil {
		return nil, err
	}

	ignored := mergedJobSearchIgnored(vals, out.own(agentSearchParams), out.dropped)
	meta := apigen.AgentJobSearchMeta{
		Total: res.Total, Limit: int64(out.limit), Offset: int64(out.offset),
		IgnoredParams: contractIgnoredParams(ignored),
		IgnoredValues: contractIgnoredValues(ignoredJobValues(vals)),
		Semantic:      optString(out.semantic),
	}
	if res.TotalIsLowerBound {
		meta.TotalIsLowerBound = apigen.NewOptBool(true)
	}
	return &apigen.AgentJobSearchPageHeaders{
		CacheControl: optString(privateCacheControl(out.private)),
		Response:     apigen.AgentJobSearchPage{Data: data, Meta: meta},
	}, nil
}

// hydrateDescriptions replaces each hit's truncated preview description with the
// job's full description read from Postgres, keyed by internal id. Best-effort: a
// hit whose id has no row (index lag vs a just-removed job) keeps its preview
// rather than being dropped.
func (h *searchHandlers) hydrateDescriptions(ctx context.Context, hits []search.JobDocument) error {
	ids := make([]int64, len(hits))
	for i := range hits {
		ids[i] = hits[i].ID
	}
	rows, err := h.descriptions.GetJobDescriptionsByIDs(ctx, ids)
	if err != nil {
		return err
	}
	byID := make(map[int64]string, len(rows))
	for _, r := range rows {
		byID[r.ID] = r.Description
	}
	for i := range hits {
		if full, ok := byID[hits[i].ID]; ok {
			hits[i].Description = full
		}
	}
	return nil
}

// formatDescription renders a job description in the requested format. Any value
// other than "text" or "markdown" — including the empty default and unrecognized
// values — yields the stored verbatim HTML, so the endpoint never fails on a bad
// format param.
func formatDescription(html, format string) string {
	switch format {
	case "text":
		return htmltext.ToText(html)
	case "markdown":
		return htmltext.ToMarkdown(html)
	default:
		return html
	}
}
