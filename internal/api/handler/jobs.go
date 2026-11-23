package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-faster/jx"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/ingest/catalogstats"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/ghost"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// jobsHandlers serves the public job catalogue reads (list, detail, repost
// copies) and the moderator-authored job writes (create/edit a hand-curated
// vacancy). The write use cases live in moderation.Service; the handlers
// translate wire ↔ domain and delegate to it.
type jobsHandlers struct {
	queries    *db.Queries
	moderation *moderation.Service
	// postings canonicalises the URL /jobs/find is asked about. Its zero value is the
	// offline rewrite only.
	postings sources.PostingURLResolver
	// cache and estimator resolve the list's meta.total: the published
	// catalogue-scale snapshot when there is one, the approximate estimate when there
	// is not. See openJobTotal.
	cache     cache.Cache
	estimator catalogstats.Estimator
	// recruiterContacts says the contact finder is configured, so the detail read may offer it.
	recruiterContacts bool
}

func newJobsHandlers(queries *db.Queries, moderation *moderation.Service, postings sources.PostingURLResolver, c cache.Cache) *jobsHandlers {
	return &jobsHandlers{queries: queries, moderation: moderation, postings: postings, cache: c, estimator: queries}
}

func (h *jobsHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// The literal /jobs/* routes (search, facets, sitemap) are registered before
	// this param route (see Register) so they are not read as slugs. optionalAuth
	// attaches the caller when signed in but never rejects, so the detail read can
	// overlay the caller's own vote (my_vote) while staying open to anonymous
	// visitors.
	api.Get("/jobs", contract)
	// Static route registered before /jobs/:slug so it isn't captured as a slug.
	api.Get("/jobs/find", contract)
	api.Get("/jobs/changes", contract)
	api.Get("/jobs/:slug", mw.optional, contract)
	api.Get("/jobs/:slug/copies", contract)
	api.Get("/jobs/:slug/apply-form", contract)
	api.Get("/jobs/:slug/skill-fit", contract)
	registerInterviewPrep(api, mw, contract)

	// Moderator-authored jobs: create a hand-curated vacancy and edit it. Authenticated
	// by cookie or API key (the CLI uses a key), then gated on the moderator role. The
	// public job reads above stay unauthenticated; a non-moderator gets 403.
	api.Post("/jobs", mw.key, mw.moderator, contract)
	api.Patch("/jobs/:slug", mw.key, mw.moderator, contract)
}

// ListJobs returns a page of jobs using limit/offset pagination. Jobs are
// served in the shared jobview wire shape (public_slug, no internal id). The page rides
// the partial index jobs_open_created_idx and meta.total comes from the precomputed
// snapshot (see openJobTotal), so neither query scans the open-job set.
//
// It reads no filter at all, so it reports ignored params: `?countries=de` would
// otherwise read as a real answer about Germany (see ignoredListJobsParams).
func (h *jobsHandlers) ListJobs(ctx context.Context, _ apigen.ListJobsParams) (*apigen.ListJobsOK, error) {
	limit, offset, err := contractPageWindowed(ctx, defaultLimit, maxLimit)
	if err != nil {
		return nil, err
	}

	jobs, err := h.queries.ListJobs(ctx, db.ListJobsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, err
	}

	total := h.openJobTotal(ctx)

	views, err := jobview.FromRows(jobs)
	if err != nil {
		return nil, err
	}
	h.attachGhostToRows(ctx, jobs, views)

	data, err := marshalRaws(views)
	if err != nil {
		return nil, err
	}
	return &apigen.ListJobsOK{Data: data, Meta: apigen.ListMeta{
		Total: total, Limit: int64(limit), Offset: int64(offset),
		IgnoredParams: contractIgnoredParams(ignoredListJobsParams(contractQuery(ctx))),
	}}, nil
}

// listJobsParams are the only query params GET /jobs reads. Its siblings declare
// their own transport params and let search.UnknownParams own the filter vocabulary;
// here the two are the same list, because this endpoint has no filter.
var listJobsParams = []string{"limit", "offset"}

// ignoredListJobsParams reports the query params this listing did not read.
//
// It deliberately does not go through ignoredParams, whose SEARCH-filter vocabulary would
// call `countries=de` legitimate on an endpoint that never reads a facet. No suggestions
// either: every facet name is equally ignored here. Reported, not refused, so old links
// with a retired param keep working.
func ignoredListJobsParams(query url.Values) []search.UnknownParam {
	var out []search.UnknownParam
	for param := range query {
		// "?=value" parses to an empty name: nothing to report, nothing to correct.
		if param == "" || slices.Contains(listJobsParams, param) {
			continue
		}
		out = append(out, search.UnknownParam{Param: param})
	}
	return search.SortAndCap(out)
}

// openJobTotal resolves the list's meta.total: the exact count from the published
// snapshot, or the approximate estimate when none is available.
//
// It cannot fail: the page of jobs is the payload, and a missing count must not cost it.
func (h *jobsHandlers) openJobTotal(ctx context.Context) int64 {
	return catalogstats.Load(ctx, h.cache, h.estimator).OpenJobs
}

// GetJob returns a single job addressed by its public slug. A slug that no longer
// exists still answers with its posting when its shortcode names exactly one, so the
// response's public_slug may differ from the one asked for — the page redirects on that.
func (h *jobsHandlers) GetJob(ctx context.Context, params apigen.GetJobParams) (*apigen.GetJobOK, error) {
	job, err := jobBySlugOrCode(ctx, h.queries, params.Slug)
	if err != nil {
		// RenderError maps pgx.ErrNoRows to 404, anything else to 500.
		return nil, err
	}
	callerID, signedIn := auth.UserIDFromContext(ctx)
	if err := jobVisibleToCaller(job, callerID, signedIn); err != nil {
		return nil, err
	}

	view, err := jobview.FromRow(job)
	if err != nil {
		return nil, err
	}

	// Widen the geography with the reposts this posting stands for, the same union its
	// search document carries, so a role found under many countries does not name one on
	// its own page. Best-effort: a failed lookup leaves the row's own geography.
	if rows, err := h.queries.DuplicateClosureGeoFor(ctx, []int64{job.ID}); err == nil {
		for _, g := range rows {
			view.MergeClosureGeography(jobview.ClosureGeo{Countries: g.Countries, Regions: g.Regions, Cities: g.Cities, Metros: g.Metros})
		}
	}

	// Attach the job-reality signal (see internal/job/jobreality): the badge on the detail
	// page. A count-query failure degrades to a unique role (counts 1) rather than
	// dropping the whole response.
	repost, mass := int64(1), int64(1)
	if cnt, err := h.queries.RoleClusterCount(ctx, db.RoleClusterCountParams{
		CompanySlug:     job.CompanySlug,
		RoleFingerprint: job.RoleFingerprint,
	}); err == nil {
		repost, mass = cnt.RepostCount, cnt.MassCount
	}
	now := time.Now()
	reality := jobview.ClassifyReality(job, now, int(repost), int(mass))
	view.Reality = &reality

	// The ghost signal takes the reality class as a criterion, so it is derived here rather
	// than recomputed. Best-effort: an evidence-lookup failure leaves the signal off.
	view.Ghost = jobview.ClassifyGhost(jobview.GhostInput{
		Now:          now,
		Closed:       job.ClosedAt.Valid,
		RealityClass: reality.Class,
		ATSAbsentAt:  job.AtsAbsentAt.Time,
		HasATSAbsent: job.AtsAbsentAt.Valid,
		Evidence:     h.ghostEvidence(ctx, job.ID),
	})

	// The application form's required-question count, the same figure the posting's search
	// document carries. Best-effort: a failed read leaves the key absent, which says "not
	// known" — never a count of zero.
	if counts, err := applyform.QuestionCounts(ctx, h.queries, []int64{job.ID}); err == nil {
		if n, ok := counts[job.ID]; ok {
			view.ApplyQuestions = &n
		}
	}

	// Referral availability: true when the company has an approved referrer, so the detail
	// page can show the "ask for a referral" block. Best-effort — a lookup error degrades
	// to false (block hidden), never failing the job read.
	if avail, err := h.queries.CompanyHasApprovedReferrer(ctx, job.CompanySlug); err == nil {
		view.ReferralAvailable = avail
	}

	// Recruiter contacts: offered only where a lookup can run, so nobody pays a click for a
	// 422. Best-effort like the referral flag.
	if h.recruiterContacts {
		if d, err := h.queries.GetJobRecruiterDomain(ctx, job.PublicSlug); err == nil {
			view.RecruiterContactsAvailable = d.Domain != ""
		}
	}

	// Caller's own thumbs vote, overlaid only when signed in (OptionalAuth attaches
	// the id on this public read). Best-effort: a lookup error leaves my_vote 0.
	if userID, ok := auth.UserIDFromContext(ctx); ok {
		if mv, err := h.queries.GetJobVote(ctx, db.GetJobVoteParams{UserID: userID, JobID: job.ID}); err == nil {
			view.MyVote = int32(mv)
		}
	}

	// Caller's own auto-apply status, a signed-in-only overlay like MyVote above. No row is
	// the common case (nil, not logged); a lookup failure also degrades to nil. Scoped to
	// autoApplyEnqueueSources so other jobs skip a query that could only come back empty.
	if userID, ok := auth.UserIDFromContext(ctx); ok && autoApplyEnqueueSources[job.Source] {
		if entry, err := h.queries.GetAutoApplyQueueEntryForJob(ctx, db.GetAutoApplyQueueEntryForJobParams{
			UserID: userID, JobID: job.ID,
		}); err == nil {
			status := autoApplyEntryStatus(entry.ReviewDecision, entry.FailedAt, entry.BlockedAt)
			view.AutoApplyStatus = &status
		}
	}

	raw, err := json.Marshal(view)
	if err != nil {
		return nil, err
	}
	return &apigen.GetJobOK{Data: jx.Raw(raw)}, nil
}

// ghostEvidence gathers the outcome evidence for one job.
func (h *jobsHandlers) ghostEvidence(ctx context.Context, jobID int64) ghost.Evidence {
	return ghostEvidenceFor(ctx, h.queries, []int64{jobID})[jobID]
}

// attachGhostToRows attaches the ghost signal to a page served from Postgres: the closed
// state and absence stamp come off the rows, only the outcome evidence needs a lookup.
//
// Best-effort: a failed lookup leaves the signal off, never failing the list.
func (h *jobsHandlers) attachGhostToRows(ctx context.Context, rows []db.Job, views []jobview.Job) {
	if len(rows) == 0 {
		return
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	evidence := ghostEvidenceFor(ctx, h.queries, ids)
	clusters := h.roleClusterCounts(ctx, rows)

	now := time.Now()
	for i, r := range rows {
		// A miss is a singleton cluster (counts 1), the per-row query's default; the bulk
		// query omits it deliberately.
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
// query rather than one per card; it is the reindex's lookup scoped to the page.
func (h *jobsHandlers) roleClusterCounts(ctx context.Context, rows []db.Job) map[clusterKey]clusterCount {
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

	rowsOut, err := h.queries.RoleClusterCountsFor(ctx, db.RoleClusterCountsForParams{
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

// jobSlugReader is what resolving a job slug reads. *db.Queries satisfies it.
type jobSlugReader interface {
	GetJobBySlug(ctx context.Context, publicSlug string) (db.Job, error)
	ListJobsBySlugCode(ctx context.Context, code string) ([]db.Job, error)
}

// slugFromLink reduces a pasted job link to its slug: the last path segment, without a
// query, fragment or trailing slash. Agents pass whole URLs, and the shortcode match reads
// the END of its input, so "…/jobs/a-x?ref=b-t35nijto" would otherwise pick job t35nijto.
// A bare slug passes unchanged: it never holds '/', '?' or '#'.
func slugFromLink(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimRight(s, "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// jobBySlugOrCode resolves stale links by identity. Collision recovery URLs carry
// the full hash; legacy links resolve only among original short-format URLs.
// Genuine shortcode ambiguity still answers pgx.ErrNoRows rather than guessing.
func jobBySlugOrCode(ctx context.Context, q jobSlugReader, slug string) (db.Job, error) {
	slug = slugFromLink(slug)
	job, err := q.GetJobBySlug(ctx, slug)
	if !errors.Is(err, pgx.ErrNoRows) {
		return job, err
	}
	code, ok := normalize.JobSlugCode(slug)
	if !ok {
		return job, err
	}
	jobs, err := q.ListJobsBySlugCode(ctx, code)
	if err != nil {
		return db.Job{}, err
	}
	identity, _ := normalize.JobSlugIdentityCode(slug)
	var matched db.Job
	matches := 0
	for _, candidate := range jobs {
		fullIdentity := normalize.JobSlugWithFullHash("", "", candidate.Source, candidate.ExternalID)
		storedIdentity, _ := normalize.JobSlugIdentityCode(candidate.PublicSlug)
		if identity == fullIdentity {
			return candidate, nil
		}
		// A long legacy title or company can have the same shape as a hash.
		// Without an exact full-hash match it still uses the ordinary shortcode.
		if storedIdentity == fullIdentity {
			continue
		}
		matched = candidate
		matches++
	}
	if matches != 1 {
		return db.Job{}, pgx.ErrNoRows
	}
	return matched, nil
}

func jobVisibleToCaller(job db.Job, callerID int64, signedIn bool) error {
	if !job.IsPrivate {
		return nil
	}
	if signedIn && job.CreatedBy.Valid && job.CreatedBy.Int64 == callerID {
		return nil
	}
	return pgx.ErrNoRows
}
