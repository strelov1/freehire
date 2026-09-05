package handler

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/catalogstats"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/ghost"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
)

// jobsHandlers serves the public job catalogue reads (list, detail, repost
// copies) and the moderator-authored job writes (create/edit a hand-curated
// vacancy). The write use cases live in moderation.Service; the handlers
// translate wire ↔ domain and delegate to it.
type jobsHandlers struct {
	queries    *db.Queries
	moderation *moderation.Service
	// postings canonicalises the URL /jobs/find is asked about. Its zero value is the
	// offline rewrite, so a caller that hands over no client keeps every lookup that
	// does not need one.
	postings sources.PostingURLResolver
	// cache and estimator resolve the list's meta.total: the published
	// catalogue-scale snapshot when there is one, the approximate estimate when there
	// is not. See openJobTotal.
	cache     cache.Cache
	estimator catalogstats.Estimator
}

func newJobsHandlers(queries *db.Queries, moderation *moderation.Service, postings sources.PostingURLResolver, c cache.Cache) *jobsHandlers {
	return &jobsHandlers{queries: queries, moderation: moderation, postings: postings, cache: c, estimator: queries}
}

func (h *jobsHandlers) register(api fiber.Router, mw middleware) {
	// The literal /jobs/* routes (search, facets, sitemap) are registered before
	// this param route (see Register) so they are not read as slugs. optionalAuth
	// attaches the caller when signed in but never rejects, so the detail read can
	// overlay the caller's own vote (my_vote) while staying open to anonymous
	// visitors.
	// Every job read shares the public-read budget. Leaving any one of them out
	// would void the limit rather than narrow it: /jobs returns the same
	// catalogue as /jobs/search, so an unbounded one is simply the door a
	// throttled caller walks through instead.
	readLimit := publicReadLimiter(mw.throttler)
	api.Get("/jobs", readLimit, h.ListJobs)
	// Static route registered before /jobs/:slug so it isn't captured as a slug.
	api.Get("/jobs/find", readLimit, h.FindJob)
	api.Get("/jobs/:slug", readLimit, mw.optional, h.GetJob)
	api.Get("/jobs/:slug/copies", readLimit, h.JobCopies)
	api.Get("/jobs/:slug/apply-form", readLimit, h.JobApplyForm)

	// Moderator-authored jobs: create a hand-curated vacancy and edit it. Authenticated
	// by cookie or API key (the CLI uses a key), then gated on the moderator role. The
	// public job reads above stay unauthenticated; a non-moderator gets 403.
	api.Post("/jobs", mw.key, mw.moderator, h.CreateJob)
	api.Patch("/jobs/:slug", mw.key, mw.moderator, h.UpdateJob)
}

// ListJobs returns a page of jobs using limit/offset pagination. Jobs are
// served in the shared jobview wire shape (public_slug, no internal id) — the
// same shape the detail and search endpoints use. The page rides the partial
// index jobs_open_created_idx (no full-table sort) and meta.total comes from the
// precomputed catalogue-scale snapshot (see openJobTotal), so neither query scans
// the whole open-job set at catalogue scale.
func (h *jobsHandlers) ListJobs(c *fiber.Ctx) error {
	limit, offset := pageParams(c)

	jobs, err := h.queries.ListJobs(c.Context(), db.ListJobsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return err
	}

	total := h.openJobTotal(c.Context())

	views, err := jobview.FromRows(jobs)
	if err != nil {
		return err
	}
	h.attachGhostToRows(c, jobs, views)

	return listResponse(c, views, total, limit, offset)
}

// openJobTotal resolves the list's meta.total: the exact count from the published
// snapshot, or the approximate estimate when none is available.
//
// It cannot fail. The page of jobs is this endpoint's actual payload, and losing it to a
// 500 because a count was unavailable — which is what the previous EstimateOpenJobs call
// did — trades the whole response for one number.
func (h *jobsHandlers) openJobTotal(ctx context.Context) int64 {
	return catalogstats.Load(ctx, h.cache, h.estimator).OpenJobs
}

// GetJob returns a single job addressed by its public slug.
func (h *jobsHandlers) GetJob(c *fiber.Ctx) error {
	job, err := h.queries.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		// RenderError maps pgx.ErrNoRows to 404, anything else to 500.
		return err
	}

	view, err := jobview.FromRow(job)
	if err != nil {
		return err
	}

	// Attach the job-reality signal (see internal/job/jobreality): the badge on the detail
	// page. A count-query failure degrades to a unique role (counts 1) rather than
	// dropping the whole response.
	repost, mass := int64(1), int64(1)
	if cnt, err := h.queries.RoleClusterCount(c.Context(), db.RoleClusterCountParams{
		CompanySlug:     job.CompanySlug,
		RoleFingerprint: job.RoleFingerprint,
	}); err == nil {
		repost, mass = cnt.RepostCount, cnt.MassCount
	}
	now := time.Now()
	reality := jobview.ClassifyReality(job, now, int(repost), int(mass))
	view.Reality = &reality

	// The ghost signal reads the reality class as one of its four criteria, so it is
	// derived here rather than recomputed. Best-effort like the rest of this block: an
	// evidence-lookup failure leaves the signal off the response instead of failing the
	// job read, and the honest direction for a missing lookup is to say nothing.
	view.Ghost = jobview.ClassifyGhost(jobview.GhostInput{
		Now:          now,
		Closed:       job.ClosedAt.Valid,
		RealityClass: reality.Class,
		ATSAbsentAt:  job.AtsAbsentAt.Time,
		HasATSAbsent: job.AtsAbsentAt.Valid,
		Evidence:     h.ghostEvidence(c, job.ID),
	})

	// Referral availability: true when the company has an approved referrer, so the detail
	// page can show the "ask for a referral" block. Best-effort — a lookup error degrades
	// to false (block hidden), never failing the job read.
	if avail, err := h.queries.CompanyHasApprovedReferrer(c.Context(), job.CompanySlug); err == nil {
		view.ReferralAvailable = avail
	}

	// Caller's own thumbs vote, overlaid only when signed in (OptionalAuth attaches
	// the id on this public read). Best-effort: a lookup error leaves my_vote 0.
	if userID, ok := auth.UserID(c); ok {
		if mv, err := h.queries.GetJobVote(c.Context(), db.GetJobVoteParams{UserID: userID, JobID: job.ID}); err == nil {
			view.MyVote = int32(mv)
		}
	}

	// Caller's own auto-apply status for this job (openspec/changes/auto-apply-submit-trigger),
	// same caller-scoped, signed-in-only overlay as MyVote above. No row is the common
	// case (nil, omitted) and is not logged as an error; a real lookup failure also
	// degrades to nil rather than failing the whole job read. Scoped to Greenhouse — the
	// only source auto-apply resolves today (autoApplyEnqueueSource) — so every other
	// job's read skips a query that could only ever come back empty.
	if userID, ok := auth.UserID(c); ok && job.Source == autoApplyEnqueueSource {
		if entry, err := h.queries.GetAutoApplyQueueEntryForJob(c.Context(), db.GetAutoApplyQueueEntryForJobParams{
			UserID: userID, JobID: job.ID,
		}); err == nil {
			status := autoApplyEntryStatus(entry.ReviewDecision, entry.FailedAt, entry.BlockedAt)
			view.AutoApplyStatus = &status
		}
	}

	return c.JSON(fiber.Map{"data": view})
}

// ghostEvidence gathers the outcome evidence for one job.
func (h *jobsHandlers) ghostEvidence(c *fiber.Ctx, jobID int64) ghost.Evidence {
	return ghostEvidenceFor(c.Context(), h.queries, []int64{jobID})[jobID]
}

// attachGhostToRows attaches the ghost signal to a page served from Postgres, where
// the rows are already in hand — so the closed state and the absence stamp come
// straight off them, and only the outcome evidence needs a lookup.
//
// Best-effort throughout: a failed lookup leaves the signal off those cards. A list
// must not fail because an optional badge could not be computed.
func (h *jobsHandlers) attachGhostToRows(c *fiber.Ctx, rows []db.Job, views []jobview.Job) {
	if len(rows) == 0 {
		return
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	evidence := ghostEvidenceFor(c.Context(), h.queries, ids)
	clusters := h.roleClusterCounts(c, rows)

	now := time.Now()
	for i, r := range rows {
		// A lookup miss is a singleton cluster (counts 1) — the same default the
		// per-row query degrades to, and what the bulk query deliberately omits.
		repost, mass := 1, 1
		if cnt, ok := clusters[clusterKey{r.CompanySlug, r.RoleFingerprint.String}]; ok {
			repost, mass = cnt.repost, cnt.mass
		}
		reality := jobview.ClassifyReality(r, now, repost, mass)
		views[i].Ghost = jobview.ClassifyGhost(jobview.GhostInput{
			Now:          now,
			Closed:       r.ClosedAt.Valid,
			RealityClass: reality.Class,
			ATSAbsentAt:  r.AtsAbsentAt.Time,
			HasATSAbsent: r.AtsAbsentAt.Valid,
			Evidence:     evidence[r.ID],
		})
	}
}

// clusterKey identifies one role cluster. The bulk count query matches its two sets
// as a cross product, so the caller keys by the exact pair rather than trusting that
// every returned row belongs to a card on this page.
type clusterKey struct {
	companySlug     string
	roleFingerprint string
}

type clusterCount struct{ repost, mass int }

// roleClusterCounts resolves the repost/mass-posting counts for a page of rows in one
// query. The per-card alternative would issue a query per card on a public list
// endpoint; this is the same lookup the reindex builds, scoped to the page.
func (h *jobsHandlers) roleClusterCounts(c *fiber.Ctx, rows []db.Job) map[clusterKey]clusterCount {
	slugs := make([]string, 0, len(rows))
	prints := make([]string, 0, len(rows))
	seen := map[clusterKey]struct{}{}
	for _, r := range rows {
		if !r.RoleFingerprint.Valid || r.RoleFingerprint.String == "" {
			continue
		}
		key := clusterKey{r.CompanySlug, r.RoleFingerprint.String}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		slugs = append(slugs, r.CompanySlug)
		prints = append(prints, r.RoleFingerprint.String)
	}
	if len(slugs) == 0 {
		return nil
	}

	rowsOut, err := h.queries.RoleClusterCountsFor(c.Context(), db.RoleClusterCountsForParams{
		CompanySlugs:     slugs,
		RoleFingerprints: prints,
	})
	if err != nil {
		return nil
	}
	out := make(map[clusterKey]clusterCount, len(rowsOut))
	for _, r := range rowsOut {
		out[clusterKey{r.CompanySlug, r.RoleFingerprint.String}] = clusterCount{int(r.RepostCount), int(r.MassCount)}
	}
	return out
}
