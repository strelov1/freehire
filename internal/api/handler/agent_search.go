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

// jobDescriptions loads full job descriptions by internal id, to rehydrate the truncated
// preview the search index serves.
type jobDescriptions interface {
	GetJobDescriptionsByIDs(ctx context.Context, ids []int64) ([]db.GetJobDescriptionsByIDsRow, error)
}

// AgentSearchJobs is the programmatic-consumer variant of SearchJobs: the same query (shared
// runJobSearch core), but every result is rehydrated with the full description from Postgres,
// in a selectable format. The truncated preview stays exclusive to /jobs/search. Public and
// unauthenticated like the other job reads.

func (h *searchHandlers) AgentSearchJobs(ctx context.Context, _ apigen.AgentSearchJobsParams) (*apigen.AgentJobSearchPageHeaders, error) {
	vals := contractQuery(ctx)
	userID, _ := auth.UserIDFromContext(ctx)
	out, err := h.runJobSearch(ctx, vals, userID, nil, "", nil)
	if err != nil {
		return nil, err
	}
	res := out.res
	if len(res.Hits) > 0 {
		if err := h.hydrateDescriptions(ctx, res.Hits); err != nil {
			return nil, err
		}
	}
	// Recorded only once the answer is certain to be served: a failed hydration serves nothing.
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
	}
	if res.TotalIsLowerBound {
		meta.TotalIsLowerBound = apigen.NewOptBool(true)
	}
	return &apigen.AgentJobSearchPageHeaders{
		CacheControl: optString(privateCacheControl(out.private)),
		Response:     apigen.AgentJobSearchPage{Data: data, Meta: meta},
	}, nil
}

// hydrateDescriptions replaces each hit's truncated preview with the full description from
// Postgres, keyed by internal id. Best-effort: a hit with no row (index lag) keeps its preview.
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

// formatDescription renders a job description in the requested format. Anything other than
// "text" or "markdown" yields the stored HTML, so a bad format param never fails the endpoint.
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
