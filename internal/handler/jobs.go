package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/moderation"
)

// jobsHandlers serves the public job catalogue reads (list, detail, repost
// copies) and the moderator-authored job writes (create/edit a hand-curated
// vacancy). The write use cases live in moderation.Service; the handlers
// translate wire ↔ domain and delegate to it.
type jobsHandlers struct {
	queries    *db.Queries
	moderation *moderation.Service
}

func newJobsHandlers(queries *db.Queries, moderation *moderation.Service) *jobsHandlers {
	return &jobsHandlers{queries: queries, moderation: moderation}
}

func (h *jobsHandlers) register(api fiber.Router, mw middleware) {
	// The literal /jobs/* routes (search, facets, sitemap) are registered before
	// this param route (see Register) so they are not read as slugs. optionalAuth
	// attaches the caller when signed in but never rejects, so the detail read can
	// overlay the caller's own vote (my_vote) while staying open to anonymous
	// visitors.
	api.Get("/jobs", h.ListJobs)
	// Static route registered before /jobs/:slug so it isn't captured as a slug.
	api.Get("/jobs/find", h.FindJob)
	api.Get("/jobs/:slug", mw.optional, h.GetJob)
	api.Get("/jobs/:slug/copies", h.JobCopies)

	// Moderator-authored jobs: create a hand-curated vacancy and edit it. Authenticated
	// by cookie or API key (the CLI uses a key), then gated on the moderator role. The
	// public job reads above stay unauthenticated; a non-moderator gets 403.
	api.Post("/jobs", mw.key, mw.moderator, h.CreateJob)
	api.Patch("/jobs/:slug", mw.key, mw.moderator, h.UpdateJob)
}

// ListJobs returns a page of jobs using limit/offset pagination. Jobs are
// served in the shared jobview wire shape (public_slug, no internal id) — the
// same shape the detail and search endpoints use. The page rides the partial
// index jobs_open_created_idx (no full-table sort) and meta.total is an
// approximate planner estimate (EstimateOpenJobs), so neither query scans the
// whole open-job set at catalogue scale.
func (h *jobsHandlers) ListJobs(c *fiber.Ctx) error {
	limit, offset := pageParams(c)

	jobs, err := h.queries.ListJobs(c.Context(), db.ListJobsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return err
	}

	total, err := h.queries.EstimateOpenJobs(c.Context())
	if err != nil {
		return err
	}

	views, err := jobview.FromRows(jobs)
	if err != nil {
		return err
	}

	return listResponse(c, views, total, limit, offset)
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

	// Attach the job-reality signal (see internal/jobreality): the badge on the detail
	// page. A count-query failure degrades to a unique role (counts 1) rather than
	// dropping the whole response.
	repost, mass := int64(1), int64(1)
	if cnt, err := h.queries.RoleClusterCount(c.Context(), db.RoleClusterCountParams{
		CompanySlug:     job.CompanySlug,
		RoleFingerprint: job.RoleFingerprint,
	}); err == nil {
		repost, mass = cnt.RepostCount, cnt.MassCount
	}
	reality := jobview.ClassifyReality(job, time.Now(), int(repost), int(mass))
	view.Reality = &reality

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

	return c.JSON(fiber.Map{"data": view})
}
