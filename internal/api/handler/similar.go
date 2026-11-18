package handler

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

var unseenWindows = search.UnseenWindows(sources.UnseenWindows(sources.Taxonomy(), search.UnseenWindow))

const (
	defaultSimilarLimit = 6
	maxSimilarLimit     = 20
)

// SimilarJobs searches the live index within the source's location and specialization.
func (h *searchHandlers) SimilarJobs(ctx context.Context, params apigen.SimilarJobsParams) (*apigen.SimilarJobsOK, error) {
	row, err := h.queries.GetJobBySlug(ctx, params.Slug)
	if err != nil {
		return nil, err
	}
	if err := jobVisibleToCaller(row, 0, false); err != nil {
		return nil, err
	}
	limit := min(max(queryIntOr(contractQuery(ctx), "limit", defaultSimilarLimit), 1), maxSimilarLimit)
	views, err := similarViews(ctx, h.search, h.queries, row, limit, func(row db.Job) bool {
		return !row.ClosedAt.Valid && !row.IsPrivate
	})
	if err != nil {
		return nil, err
	}
	data, err := marshalRaws(views)
	if err != nil {
		return nil, err
	}
	return &apigen.SimilarJobsOK{Data: data}, nil
}

type similarReader interface {
	GetJobDuplicateOwner(ctx context.Context, jobID int64) (int64, error)
	GetJobsByIDs(ctx context.Context, ids []int64) ([]db.Job, error)
}

// similarParams keeps geography AND specialization mandatory. Country and city must
// be separate groups: the general search filter ORs geography facets together.
// Use the most precise known location, without inventing a city for remote jobs.
// Unknown specialization or geography cannot safely yield a related-jobs list.
func similarParams(source jobview.Job, id int64, limit int) (search.SearchParams, bool) {
	if source.Enrichment.Category == "" || source.Enrichment.Category == "other" {
		return search.SearchParams{}, false
	}
	groups, ok := facetGroups("category", []string{source.Enrichment.Category})
	if !ok {
		return search.SearchParams{}, false
	}
	hasLocation := false
	for _, facet := range []struct {
		key    string
		values []string
	}{
		{"countries", source.Countries},
		{"cities", source.Cities},
	} {
		if len(facet.values) > 0 {
			part, ok := facetGroups(facet.key, facet.values)
			if !ok {
				return search.SearchParams{}, false
			}
			groups = append(groups, part...)
			hasLocation = true
		}
	}
	if !hasLocation && len(source.Regions) > 0 {
		part, ok := facetGroups("regions", source.Regions)
		if !ok {
			return search.SearchParams{}, false
		}
		groups = append(groups, part...)
		hasLocation = true
	}
	if !hasLocation {
		return search.SearchParams{}, false
	}
	groups = append(groups, []string{fmt.Sprintf("id != %d", id)})
	return search.SearchParams{Query: source.Title, LiteralQuery: true, Filter: groups, Limit: limit * 3}, true
}

// facetGroups turns one stored facet into filter groups. A stored value that is blank
// is dropped by the filter builder, which can leave no group at all; that must read as
// "unknown", never as a constraint quietly missing.
func facetGroups(key string, values []string) ([][]string, bool) {
	groups, ok := search.FilterFromValues(url.Values{key: values}).([][]string)
	return groups, ok && len(groups) > 0
}

// similarViews searches by the source's title under those filters, then re-reads the
// hits to enforce current visibility.
func similarViews(ctx context.Context, idx searcher, q similarReader, source db.Job, limit int, show func(db.Job) bool) ([]jobview.Job, error) {
	if idx == nil {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}
	view, err := jobview.FromRow(source)
	if err != nil {
		return nil, err
	}
	limit = min(max(limit, 1), maxSimilarLimit)
	params, ok := similarParams(view, source.ID, limit)
	if !ok {
		return []jobview.Job{}, nil
	}
	excludedID := source.ID
	if source.DuplicateOf.Valid {
		excludedID, err = q.GetJobDuplicateOwner(ctx, source.ID)
		if err != nil {
			return nil, err
		}
		if excludedID == 0 {
			return []jobview.Job{}, nil
		}
		params.Filter = append(params.Filter.([][]string), []string{fmt.Sprintf("id != %d", excludedID)})
	}
	res, err := idx.Search(ctx, params)
	if err != nil {
		return nil, err
	}
	return visibleSimilar(ctx, q, excludedID, res.Hits, limit, show)
}

func visibleSimilar(ctx context.Context, q similarReader, sourceID int64, hits []search.JobDocument, limit int, show func(db.Job) bool) ([]jobview.Job, error) {
	views := make([]jobview.Job, 0, limit)
	if len(hits) == 0 {
		return views, nil
	}
	ids := make([]int64, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.ID)
	}
	rows, err := q.GetJobsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]db.Job, len(rows))
	now := time.Now()
	for _, row := range rows {
		if row.ID != sourceID && !row.DuplicateOf.Valid && show(row) && !unseenWindows.Unseen(row, now) {
			byID[row.ID] = row
		}
	}
	for _, hit := range hits {
		row, ok := byID[hit.ID]
		if !ok {
			continue
		}
		delete(byID, hit.ID)
		view, err := jobview.FromRow(row)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
		if len(views) == limit {
			break
		}
	}
	return views, nil
}
