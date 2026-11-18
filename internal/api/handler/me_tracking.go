package handler

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/go-faster/jx"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// ListTrackedJobs returns the caller's job interactions joined with the jobs, most recently
// touched first, narrowed by ?filter=all|viewed|saved|applied|board (default all; viewed is
// neither saved nor applied; board is jobs with saved_at, applied_at, or stage set). meta
// carries total/limit/offset for the filter plus per-filter counts for the tab badges, so it
// writes its own envelope. Closed jobs stay listed: history must not shrink when a posting
// closes.
//
// ?compact=1 is the Kanban board's read: identity and tag row only (no blurb, skills,
// collections or ghost signal), which skips the ghost pass's queries and Meilisearch round
// trip.
func (h *trackingHandlers) ListTrackedJobs(ctx context.Context, params apigen.ListTrackedJobsParams) (*apigen.TrackedJobPage, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	limit, offset := contractPageBounded(ctx, defaultLimit, trackingMaxLimit)
	detail := jobtracking.CardFull
	if compact, _ := strconv.ParseBool(params.Compact.Value); compact {
		detail = jobtracking.CardCompact
	}
	listing, err := h.tracking.ListTracked(ctx, userID, params.Filter.Value, detail, int32(limit), int32(offset))
	if err != nil {
		return nil, trackingError(err)
	}
	// The ghost signal is set on each card in place, so attach it before writing the cards.
	if detail == jobtracking.CardFull {
		h.attachGhostToTrackedCards(ctx, listing.Items)
	}

	// One clock for the whole page: silence is derived against now(), and rows must agree on it.
	now := time.Now()
	items := make([]apigen.TrackedJob, 0, len(listing.Items))
	for _, it := range listing.Items {
		// The card travels as the jobview wire shape.
		job, err := json.Marshal(it.Job)
		if err != nil {
			return nil, err
		}
		item := apigen.TrackedJob{
			ID:             it.ID,
			AddedToBoardAt: nilDateTime(it.AddedToBoardAt),
			CompanySlug:    it.CompanySlug,
			RoleTitle:      it.RoleTitle,
			Job:            jx.Raw(job),
			ViewedAt:       nilDateTime(it.ViewedAt),
			SavedAt:        nilDateTime(it.SavedAt),
			AppliedAt:      nilDateTime(it.AppliedAt),
			Stage:          nilString(it.Stage),
			Notes:          nilString(it.Notes),
			EmailCount:     int64(it.EmailCount),
			FollowedUpAt:   nilDateTime(it.FollowedUpAt),
			CvOpenedAt:     nilDateTime(it.CVOpenedAt),
		}
		if it.AutoApplyStatus != nil {
			item.AutoApplyStatus = apigen.NewOptAutoApplyReviewStatus(apigen.AutoApplyReviewStatus(*it.AutoApplyStatus))
		}
		if s := it.Silence(now); s != nil {
			item.LastActivityAt = apigen.NewNilDateTime(s.LastActivityAt)
			item.DaysSilent = apigen.NewNilInt64(int64(s.DaysSilent))
			item.SilenceState = apigen.NewNilSilenceState(apigen.SilenceState(s.State))
		} else {
			item.LastActivityAt.SetToNull()
			item.DaysSilent.SetToNull()
			item.SilenceState.SetToNull()
		}
		items = append(items, item)
	}

	return &apigen.TrackedJobPage{
		Data: items,
		Meta: apigen.TrackingMeta{
			Total:  listing.Total(),
			Limit:  int64(limit),
			Offset: int64(offset),
			Counts: apigen.TrackingCounts{
				All:       listing.Counts.All,
				Viewed:    listing.Counts.Viewed,
				Saved:     listing.Counts.Saved,
				Applied:   listing.Counts.Applied,
				Board:     listing.Counts.Board,
				Dismissed: listing.Counts.Dismissed,
			},
		},
	}, nil
}

// attachGhostToTrackedCards attaches each tracked job's ghost signal to its already-built
// card, mutating tracked[i].Job in place (the same *jobview.Card pointer already copied into
// myJobResponse items). Best-effort like ghostEvidenceFor/attachGhost: a lookup failure
// leaves the signal off rather than failing the read.
//
// The reality class comes from Meilisearch, never recomputed from a description: this
// listing's query deliberately never reads one (TestMeasureBoardLoad).
func (h *trackingHandlers) attachGhostToTrackedCards(ctx context.Context, tracked []jobtracking.TrackedJob) {
	if h.search == nil || h.queries == nil {
		return
	}
	ids := make([]int64, 0, len(tracked))
	for _, it := range tracked {
		if it.Job != nil {
			ids = append(ids, it.JobID)
		}
	}
	if len(ids) == 0 {
		return
	}

	stampRows, err := h.queries.ListJobGhostStamps(ctx, ids)
	if err != nil {
		logGhostLookup(ctx, "tracker absence stamps", len(ids), err)
		return
	}
	stamps := make(map[int64]db.ListJobGhostStampsRow, len(stampRows))
	for _, r := range stampRows {
		stamps[r.ID] = r
	}
	evidence := ghostEvidenceFor(ctx, h.queries, ids)

	// A Search failure degrades to an empty reality map rather than returning early: the ghost
	// signal may still fire from ATS-absence or outcome evidence alone.
	reality := make(map[int64]string, len(ids))
	if res, err := h.search.Search(ctx, search.SearchParams{Filter: search.In("id", ids), Limit: len(ids)}); err != nil {
		logGhostLookup(ctx, "tracker reality class", len(ids), err)
	} else {
		for _, hit := range res.Hits {
			if hit.Reality != nil {
				reality[hit.ID] = hit.Reality.Class
			}
		}
	}

	now := time.Now()
	for _, it := range tracked {
		if it.Job == nil {
			continue
		}
		row := stamps[it.JobID]
		it.Job.Ghost = jobview.ClassifyGhost(jobview.GhostInput{
			Now:          now,
			Closed:       row.ClosedAt.Valid,
			RealityClass: reality[it.JobID],
			ATSAbsentAt:  row.AtsAbsentAt.Time,
			HasATSAbsent: row.AtsAbsentAt.Valid,
			Evidence:     evidence[it.JobID],
		})
	}
}

// TrackingPipeline returns the caller's application-pipeline snapshot: the total count and
// the count at each stage, aggregated server-side. The SPA groups the stages with the
// generated STAGE_GROUPS to draw the Sankey and derives the rate cards.
func (h *trackingHandlers) TrackingPipeline(ctx context.Context) (*apigen.TrackingPipelineOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	pipeline, err := h.tracking.Pipeline(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := apigen.TrackingPipeline{Applications: pipeline.Applications, Stages: apigen.TrackingPipelineStages(pipeline.Stages)}
	if pipeline.ReplyRate != nil {
		out.ReplyRate = apigen.NewOptReplyRateBenchmark(apigen.ReplyRateBenchmark{
			You:    apigen.ReplyRateSide(pipeline.ReplyRate.You),
			Global: apigen.ReplyRateSide(pipeline.ReplyRate.Global),
		})
	}
	return &apigen.TrackingPipelineOK{Data: out}, nil
}

// ListViewedSlugs returns the public job slugs the caller has interacted with (every
// user_jobs row counts as viewed). The SPA dims already-seen cards with it; viewed state is
// cross-referenced client-side, never joined into the public ListJobs/SearchJobs. Response:
// {"data": [slug, ...]}.
func (h *trackingHandlers) ListViewedSlugs(ctx context.Context) (*apigen.ListViewedSlugsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	slugs, err := h.tracking.ViewedSlugs(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &apigen.ListViewedSlugsOK{Data: slugs}, nil
}

// ListSavedSlugs returns the public job slugs the caller has saved (bookmarked), cross-
// referenced client-side like ListViewedSlugs. Response: {"data": [slug, ...]}.
func (h *trackingHandlers) ListSavedSlugs(ctx context.Context) (*apigen.ListSavedSlugsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	slugs, err := h.tracking.SavedSlugs(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &apigen.ListSavedSlugsOK{Data: slugs}, nil
}

// ListDismissedSlugs returns the public job slugs the caller has hidden (dismissed), so the
// SPA can exclude them from the browse feed, mirroring ListSavedSlugs.
func (h *trackingHandlers) ListDismissedSlugs(ctx context.Context) (*apigen.ListDismissedSlugsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	slugs, err := h.tracking.DismissedSlugs(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &apigen.ListDismissedSlugsOK{Data: slugs}, nil
}

// ListAppliedCompanySlugs returns the company slugs of the employers the caller has applied
// to, cross-referenced client-side so public job reads stay unauthenticated and cacheable.
// Response: {"data": ["acme", ...]}.
func (h *trackingHandlers) ListAppliedCompanySlugs(ctx context.Context) (*apigen.ListAppliedCompanySlugsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	slugs, err := h.tracking.AppliedCompanySlugs(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &apigen.ListAppliedCompanySlugsOK{Data: slugs}, nil
}
