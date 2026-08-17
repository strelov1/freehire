package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/htmltext"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
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
func (h *searchHandlers) AgentSearchJobs(c *fiber.Ctx) error {
	res, limit, offset, err := h.runJobSearch(c)
	if err != nil {
		return err
	}

	if len(res.Hits) > 0 {
		if err := h.hydrateDescriptions(c.Context(), res.Hits); err != nil {
			return err
		}
	}

	format := c.Query("description_format")
	views := make([]jobview.Job, len(res.Hits))
	for i, hit := range res.Hits {
		hit.Description = formatDescription(hit.Description, format)
		views[i] = hit.Job
	}

	return listResponseWithIgnored(c, views, res.Total, limit, offset, ignoredParams(c, agentSearchParams))
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
