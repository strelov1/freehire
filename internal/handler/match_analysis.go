package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/experience"
	"github.com/strelov1/freehire/internal/hardconstraint"
	"github.com/strelov1/freehire/internal/jobmatch"
	"github.com/strelov1/freehire/internal/matchanalysis"
	"github.com/strelov1/freehire/internal/pgconv"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/resumeextract"
	"github.com/strelov1/freehire/internal/userprofile"
)

// matchHandlers serves the per-job match surfaces: the read-only skill match, the
// on-demand three-stage LLM fit analysis (GET cached / POST run / SSE stream), and
// the caller's list of analyzed jobs. The LLM analyzer is nil-safe (a nil client
// degrades Analyze to a no-op); the cache and the AI-points meter ride along.
type matchHandlers struct {
	queries *db.Queries
	// userProfile loads the caller's profile (skills for the match bar, location
	// preferences for the hard-constraint blockers).
	userProfile *userprofile.Service
	// resume provides the stored-CV text and the structured résumé the analysis
	// grounds on.
	resume *resume.Store
	// matchAnalysis runs the on-demand three-stage LLM fit analysis for one
	// (candidate, job). Its client is nil when the LLM is unconfigured; Analyze then
	// degrades to a no-op.
	matchAnalysis *matchanalysis.Analyzer
	// llm binds the model client and the credential resolver, so an analysis is spent
	// under the account that asked for it.
	llm llmBinding
	// matchAnalysisCache reads/writes the per-(user, job) cached fit analysis
	// (backed by *db.Queries).
	matchAnalysisCache matchAnalysisStore
	// credits meters the per-user AI-points balance the analysis debits.
	credits *credits.Store
	// bank supplies the candidate's work history. Nil when there are no queries to build
	// it over, which reads as an empty bank — and an empty bank means no analysis.
	bank candidateProfiler
	// coalesce ensures at most one fit-analysis compute runs at a time for a given (user, job)
	// — see matchAnalysisCoordinator for why the cold-start autopilot's invisible pre-run and
	// the workspace's visible stream can both reach autopilotAnalysis.ensure/StreamMatchAnalysis
	// for the identical pair. Billing is decided per caller, not by who leads — see
	// StreamMatchAnalysis's own comment.
	coalesce matchAnalysisCoordinator
}

func newMatchHandlers(queries *db.Queries, userProfile *userprofile.Service, resumeStore *resume.Store, analyzer *matchanalysis.Analyzer, creditsStore *credits.Store) *matchHandlers {
	return &matchHandlers{
		queries:            queries,
		userProfile:        userProfile,
		resume:             resumeStore,
		matchAnalysis:      analyzer,
		matchAnalysisCache: queries,
		bank:               newCandidateProfiler(queries),
		credits:            creditsStore,
	}
}

func (h *matchHandlers) register(api fiber.Router, mw middleware) {
	// Read-only per-job skill match against the caller's profile (no writes).
	api.Get("/jobs/:slug/match", mw.key, h.JobMatch)
	// Ad-hoc skill match for a job posting scraped off any page (title + text),
	// no catalog job required — powers the browser extension's on-any-page card.
	api.Post("/me/match-text", mw.key, matchTextLimiter(mw.throttler), h.MatchText)
	// The on-demand LLM match analysis (GET cached / POST run / SSE stream). The two routes
	// that actually drive the prompt chain share ONE limiter instance, so the budget is per
	// user and not per route — a limiter per mount would hand the stream, the POST, and each
	// deprecated alias a fresh allowance of the same user's quota.
	runLimit := matchAnalysisLimiter(mw.throttler)
	api.Get("/jobs/:slug/match-analysis", mw.key, h.GetMatchAnalysis)
	api.Post("/jobs/:slug/match-analysis", mw.key, runLimit, h.PostMatchAnalysis)
	api.Get("/jobs/:slug/match-analysis/stream", mw.key, runLimit, h.StreamMatchAnalysis)
	// Deprecated pre-rename aliases (was "fit") — kept so existing API-key clients and the
	// CLI don't break; they hit the same handlers. Remove once callers have migrated.
	api.Get("/jobs/:slug/fit", mw.key, h.GetMatchAnalysis)
	api.Post("/jobs/:slug/fit", mw.key, runLimit, h.PostMatchAnalysis)
	api.Get("/jobs/:slug/fit/stream", mw.key, runLimit, h.StreamMatchAnalysis)
	// analyses lists the jobs the caller has run the AI fit analysis on.
	api.Get("/me/tracking/analyses", mw.key, h.ListMyAnalyses)
}

// callerLanguage reads the caller's preferred interface language for the fit-analysis
// prompt chain (see matchanalysis.Input.Language) and for comparing against a cached
// row's stamp. Best-effort: a lookup failure falls back to "en" rather than failing the
// request.
func (h *matchHandlers) callerLanguage(ctx context.Context, userID int64) string {
	lang, err := h.queries.GetUserLanguage(ctx, userID)
	if err != nil {
		log.Printf("matchanalysis: language for user %d: %v", userID, err)
		return "en"
	}
	return lang
}

// creditsError writes the 402 Payment Required body when a metered action can't be
// afforded: a message plus the caller's remaining points and the date the monthly grant
// resets, so the SPA can render an out-of-credits state.
func creditsError(c *fiber.Ctx, bal credits.Balance) error {
	return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
		"error":     "You're out of AI credits for this month.",
		"remaining": bal.Remaining,
		"resets_at": bal.ResetsAt,
	})
}

// matchAnalysisStore reads/writes the per-(user, job) cached fit analysis. *db.Queries satisfies
// it; a fake backs the DB-less handler tests.
type matchAnalysisStore interface {
	GetUserJobAnalysis(ctx context.Context, arg db.GetUserJobAnalysisParams) (db.GetUserJobAnalysisRow, error)
	UpsertUserJobAnalysis(ctx context.Context, arg db.UpsertUserJobAnalysisParams) error
	ListUserJobAnalyses(ctx context.Context, userID int64) ([]db.ListUserJobAnalysesRow, error)
}

// matchAnalysisResponse is the wire shape for the LLM fit analysis. HasCV is false when the
// caller has no stored CV — the SPA then prompts an upload instead of an empty report.
// Stale marks a cached analysis whose CV or job changed since (the SPA offers a
// recompute); Analysis is nil when none is cached or the LLM is unconfigured. Credits is
// set on reads (GET) so the SPA can show the points balance and pre-block a new-job
// analysis; it is omitted on the compute responses.
type matchAnalysisResponse struct {
	HasCV    bool                    `json:"has_cv"`
	Stale    bool                    `json:"stale"`
	Analysis *matchanalysis.Analysis `json:"analysis"`
	Credits  *credits.Balance        `json:"credits,omitempty"`
}

// GetMatchAnalysis serves the cached fit analysis for one of the caller's jobs, never calling
// the LLM. It returns the cached analysis (flagged stale when the CV or job changed
// since it was computed), or a null analysis when none is cached. Cookie or API key;
// an unknown slug is a 404. has_cv=false (no LLM ever) when no CV is stored.
func (h *matchHandlers) GetMatchAnalysis(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	job, err := h.queries.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err
	}
	cvUploadedAt, hasCV := h.cvUploadedAt(c, userID)
	if !hasCV {
		// No CV means no analysis is possible, so usage is moot — skip the count query.
		return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: false}})
	}
	bal := h.creditsBalance(c.Context(), userID)
	row, err := h.matchAnalysisCache.GetUserJobAnalysis(c.Context(), db.GetUserJobAnalysisParams{UserID: userID, JobID: job.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: true, Credits: bal}})
	}
	if err != nil {
		return err
	}
	analysis := decodeAnalysis(row.Analysis)
	if analysis == nil {
		return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: true, Credits: bal}})
	}
	// Recompute the hard-constraint ceiling from the current job/résumé/dictionary and
	// apply it to the cached analysis on read — the cap is never stored, so a dictionary
	// change takes effect without marking the cache stale.
	h.capServedAnalysis(c.Context(), userID, job, analysis)
	stale := !stampsFresh(row, cvUploadedAt, job.ContentHash, h.matchAnalysis.ModelID(), h.callerLanguage(c.Context(), userID))
	return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: true, Stale: stale, Analysis: analysis, Credits: bal}})
}

// PostMatchAnalysis runs the three-stage fit prompt-chain over the caller's stored CV and the
// job, caches the result per (user, job), and returns it fresh. Best-effort: an
// unconfigured or failing LLM returns has_cv with a null analysis (200) and caches
// nothing. Cookie or API key; unknown slug 404; has_cv=false when no CV is stored.
func (h *matchHandlers) PostMatchAnalysis(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	job, err := h.queries.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err
	}
	cvUploadedAt, hasCV := h.cvUploadedAt(c, userID)
	if !hasCV {
		return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: false}})
	}
	// The upload time above doubles as the cache stamp, captured up front so the cache
	// is stamped with the CV that was actually analyzed even if the user re-uploads
	// mid-analysis (the three-stage chain takes seconds); re-reading it afterwards would
	// risk stamping a newer CV's time on an older CV's analysis. language is captured the
	// same way, for the same reason.
	language := h.callerLanguage(c.Context(), userID)
	// Gate on points before touching the LLM: a new job needs at least the match cost, a
	// recompute of an already-analyzed job is always free. Only new analyses are charged,
	// and only after they persist (below), so a legacy cached job re-runs for free.
	isNew, err := h.matchIsNew(c.Context(), userID, job.ID)
	if err != nil {
		return err
	}
	if isNew {
		bal := h.creditsBalance(c.Context(), userID)
		if bal != nil && bal.Remaining < h.credits.Cost(credits.FeatureMatch) {
			return creditsError(c, *bal)
		}
	}

	// The caller's profile drives both the deterministic skills anchor and the location
	// dimension; a missing profile is tolerated (zero value → empty skills/preferences).
	profile, _ := h.userProfile.Get(c.Context(), userID)

	// Compute the hard-constraint blockers once: the unmet ones ground the prompt
	// (below) and the same list caps the served score (applyBlockers, after caching).
	blockers := h.jobBlockers(c.Context(), userID, job, profile)

	analysis, err := h.runAnalysis(c, userID, job, profile, blockers, language)
	if err != nil {
		// Best-effort: log (never the CV/job text) and serve no analysis.
		log.Printf("matchanalysis: analyze failed for user %d job %d: %v", userID, job.ID, err)
		return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: true}})
	}
	if analysis == nil {
		// LLM unconfigured — nothing to cache.
		return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: true}})
	}
	h.cacheAnalysis(c.Context(), userID, job, cvUploadedAt, language, analysis)
	if isNew {
		h.debitMatch(c.Context(), userID, job.ID)
	}
	// Cache holds the uncapped LLM analysis; cap the served copy from the blockers we
	// already computed for the prompt (same recompute-on-read the GET path does).
	applyBlockers(analysis, blockers)
	return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: true, Stale: false, Analysis: analysis}})
}

// buildAnalysisInput assembles the fit chain's input from the candidate's profile and the
// vacancy. Split out of runAnalysis so a caller that must run the chain from a plain
// context — the autopilot's post-run refresh, which runs from the SSE writer's detached
// goroutine after the fiber ctx is gone — can build the Input once, while c is still
// valid, and carry only the plain value into that goroutine.
func (h *matchHandlers) buildAnalysisInput(c *fiber.Ctx, job db.Job, userID int64, profile userprofile.Profile, blockers []hardconstraint.Blocker, language string) matchanalysis.Input {
	return matchanalysis.Input{
		JobTitle:            job.Title,
		JobDescription:      job.Description,
		CompanyInfo:         h.companyInfo(c, job.CompanySlug),
		StructuredResume:    h.candidateProfile(c, userID),
		Match:               jobmatch.Compute(job.Skills, profile.Skills),
		JobWorkMode:         job.WorkMode,
		JobRemote:           job.Remote,
		JobLocation:         job.Location,
		JobRegions:          job.Regions,
		JobCountries:        job.Countries,
		LocationPreferences: string(profile.LocationPreferences),
		Blockers:            blockers,
		Language:            language,
	}
}

// runAnalysis runs the three-stage fit chain under the caller's own attribution, over an
// input built by buildAnalysisInput, for a caller that has a live fiber ctx: the on-demand
// endpoint. The autopilot's two halves run after the request and build that same input
// through the same function, from plain values — see autopilotAnalysis.
func (h *matchHandlers) runAnalysis(c *fiber.Ctx, userID int64, job db.Job, profile userprofile.Profile, blockers []hardconstraint.Blocker, language string) (*matchanalysis.Analysis, error) {
	analyzer := h.matchAnalysis.As(h.llm.bind(c.Context(), userID, tagMatchAnalysis))
	return analyzer.Analyze(c.Context(), h.buildAnalysisInput(c, job, userID, profile, blockers, language))
}

// autopilotAnalysis is one autopilot run's fit analysis: the cold-start fill the run's first
// tool call (cv_context) depends on, and the refresh that must follow the run. Both run the
// same three-stage chain over the same Input, so both are assembled once, here, rather than
// each reading the candidate's profile, blockers and bank for itself.
//
// Everything it holds is a plain value because NEITHER half runs in the request. The refresh
// never could — it fires after the turn, from the SSE writer's detached goroutine. The fill
// now joins it there: run in the handler ahead of the stream, it left the response silent for
// the whole chain, which on 2026-08-21 was 2m6s against nginx's 60s default. The proxy cut
// five runs that way, and each one told the candidate their run had failed to start while it
// was in fact running.
//
// A nil *autopilotAnalysis is a working no-op — the caller has no match surface wired — so
// PostAssistantAutopilot needs no branch of its own.
type autopilotAnalysis struct {
	h            *matchHandlers
	userID       int64
	job          db.Job
	analyzer     *matchanalysis.Analyzer
	input        matchanalysis.Input
	cvUploadedAt *time.Time
}

// prepareAutopilotRun assembles the run's analysis while the fiber ctx is still valid. It
// reads — profile, blockers, language, the credential binding, the Input, the CV's upload
// stamp — and computes nothing; both halves are the caller's to run from the stream.
func (h *matchHandlers) prepareAutopilotRun(c *fiber.Ctx, userID int64, job db.Job) *autopilotAnalysis {
	profile, _ := h.userProfile.Get(c.Context(), userID)
	blockers := h.jobBlockers(c.Context(), userID, job, profile)
	language := h.callerLanguage(c.Context(), userID)
	cvUploadedAt, _ := h.cvUploadedAt(c, userID)
	return &autopilotAnalysis{
		h:            h,
		userID:       userID,
		job:          job,
		analyzer:     h.matchAnalysis.As(h.llm.bind(c.Context(), userID, tagMatchAnalysis)),
		input:        h.buildAnalysisInput(c, job, userID, profile, blockers, language),
		cvUploadedAt: cvUploadedAt,
	}
}

// ensure computes and caches the analysis when none is cached yet. It exists for the
// cold-start autopilot run, whose first tool call reads the cache and errors without one —
// this is what lets that run start without requiring the candidate to have produced an
// analysis first.
//
// Best-effort and silent: a lookup or compute failure (no LLM configured, an analyzer error)
// is logged and left uncached, exactly as PostMatchAnalysis already degrades. No credits
// debit — this path is unmetered, tracked only by the same LLM spend attribution every call
// already carries (see the tailor-coldstart-autopilot design's "no new metering" decision).
//
// Coalesced through h.coalesce: the tailoring workspace's visible match-analysis stream
// starts at the same cold start this runs at, so both routinely race for the identical
// (user, job) — see matchAnalysisCoordinator for why only one of them ever actually computes.
func (a *autopilotAnalysis) ensure(ctx context.Context) {
	if a == nil {
		return
	}
	done, run, isLeader := a.h.coalesce.lead(a.userID, a.job.ID)
	if !isLeader {
		// The visible stream (or another concurrent call for this pair) already claimed the
		// compute — wait for it rather than spending a second three-stage chain on the same
		// input. Best-effort either way, exactly as before this coalescing existed: whether
		// the leader actually cached anything is not this function's concern.
		<-run.done
		return
	}
	succeeded := false
	defer func() { done(succeeded) }()

	if _, err := a.h.matchAnalysisCache.GetUserJobAnalysis(ctx,
		db.GetUserJobAnalysisParams{UserID: a.userID, JobID: a.job.ID}); err == nil {
		succeeded = true // already cached — nothing to compute, and it's a good row for a follower to read
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("matchanalysis: checking cache before an autopilot run, user %d job %d: %v",
			a.userID, a.job.ID, err)
		return
	}
	succeeded = a.compute(ctx, "inline compute before an autopilot run")
}

// refresh UNCONDITIONALLY recomputes the chain and overwrites the (user, job) cache, even
// when nothing was cached or an analysis was already there. This is what repeals the
// fit-analysis-post-autopilot-verify design's predecessor rule that the fit analysis is a
// frozen snapshot of the base profile.
func (a *autopilotAnalysis) refresh(ctx context.Context) {
	if a == nil {
		return
	}
	a.compute(ctx, "post-autopilot refresh")
}

// compute runs the chain and caches what it produced, reporting whether the cache is left
// holding this call's own result — which is what a coalescing follower waits to learn.
func (a *autopilotAnalysis) compute(ctx context.Context, stage string) bool {
	analysis, err := a.analyzer.Analyze(ctx, a.input)
	if err != nil {
		log.Printf("matchanalysis: %s, user %d job %d: %v", stage, a.userID, a.job.ID, err)
		return false
	}
	if analysis == nil {
		return false // LLM unconfigured — nothing to cache
	}
	a.h.cacheAnalysis(ctx, a.userID, a.job, a.cvUploadedAt, a.input.Language, analysis)
	return true
}

// creditsBalance reports the caller's current points, or nil on a DB error (logged).
// Best-effort: a transient hiccup must neither block a legitimate analysis nor 402 the
// caller — the atomic Debit remains the real ceiling.
func (h *matchHandlers) creditsBalance(ctx context.Context, userID int64) *credits.Balance {
	bal, err := h.credits.Balance(ctx, userID)
	if err != nil {
		log.Printf("credits: balance for user %d: %v", userID, err)
		return nil
	}
	return &bal
}

// matchIsNew reports whether analysing (userID, jobID) would be the caller's FIRST
// analysis of that job — i.e. no cached row exists. A recompute (row present) is free and
// never charged, so a legacy analysis cached before credits shipped re-runs for free.
func (h *matchHandlers) matchIsNew(ctx context.Context, userID, jobID int64) (bool, error) {
	_, err := h.matchAnalysisCache.GetUserJobAnalysis(ctx, db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID})
	if err == nil {
		return false, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	return false, err
}

// debitMatch charges one match against the caller's points after a fresh analysis has
// persisted, idempotent by job id. Best-effort: the analysis is already computed and
// cached, so a debit error (including a rare insufficient-balance race the pre-check let
// through) is logged, not surfaced.
func (h *matchHandlers) debitMatch(ctx context.Context, userID, jobID int64) {
	if _, err := h.credits.Debit(ctx, userID, credits.FeatureMatch, strconv.FormatInt(jobID, 10)); err != nil {
		log.Printf("credits: match debit user=%d job=%d: %v", userID, jobID, err)
	}
}

// cacheAnalysis upserts the analysis stamped with the analyzed CV's upload time, the job
// content hash, the language it was written in, and the model that produced it. It takes
// a plain context (not the fiber ctx) so the SSE stream can cache after the request
// handler has returned. Best-effort: a cache failure is logged, not surfaced.
func (h *matchHandlers) cacheAnalysis(ctx context.Context, userID int64, job db.Job, cvUploadedAt *time.Time, language string, analysis *matchanalysis.Analysis) {
	blob, err := json.Marshal(analysis)
	if err != nil {
		return
	}
	if err := h.matchAnalysisCache.UpsertUserJobAnalysis(ctx, db.UpsertUserJobAnalysisParams{
		UserID:         userID,
		JobID:          job.ID,
		Analysis:       blob,
		Model:          h.matchAnalysis.ModelID(),
		CvUploadedAt:   pgconv.Timestamptz(cvUploadedAt),
		JobContentHash: job.ContentHash,
		Language:       language,
	}); err != nil {
		log.Printf("matchanalysis: cache analysis for user %d job %d: %v", userID, job.ID, err)
	}
}

// cvUploadedAt reports the caller's stored-CV upload time; ok=false (no error) when CV
// storage is disabled or the caller has none stored.
func (h *matchHandlers) cvUploadedAt(c *fiber.Ctx, userID int64) (*time.Time, bool) {
	if !h.resume.Enabled() {
		return nil, false
	}
	meta, err := h.resume.Status(c.Context(), userID)
	if err != nil || !meta.Present {
		return nil, false
	}
	return meta.UploadedAt, true
}

// candidateProfile returns the caller's contact-free résumé projection for the fit input, or
// the zero value when the caller has none current (no résumé, unconfigured LLM, not yet
// extracted, or stale) — the fit chain then produces no analysis (the raw CV is never sent as
// a fallback). Best-effort: a read error degrades to the zero value.
func (h *matchHandlers) candidateProfile(c *fiber.Ctx, userID int64) resumeextract.Professional {
	// The structured résumé still owns education, languages, the summary and the years
	// estimate, and is read best-effort: a stale or absent structure now costs those
	// sections, not the whole analysis.
	var st resumeextract.Structured
	if h.resume.Enabled() {
		if stored, ok, err := h.resume.Structured(c.Context(), userID); err == nil && ok {
			st = stored
		}
	}

	bank := h.candidateBank()
	if bank == nil {
		return resumeextract.Professional{}
	}
	profile, err := bank.Professional(c.Context(), userID, st)
	if err != nil {
		log.Printf("candidate profile: user %d: %v", userID, err)
		return resumeextract.Professional{}
	}
	// Returned as-is, empty work history included — the chain is what refuses a candidate
	// with no experience. There is deliberately no fallback here to the structure's own copy
	// of that history: scoring somebody against experience nothing owns is worse than not
	// scoring them, and a silent fallback would hide a failed backfill for months.
	return profile
}

// candidateBank returns the bank the fit chain reads, building it from the handler's
// queries when the field was not wired. A nil field must not be read as "this candidate
// has no experience": those are different statements, and collapsing them would deny
// someone their fit analysis over an assembly detail.
func (h *matchHandlers) candidateBank() candidateProfiler {
	if h.bank != nil {
		return h.bank
	}
	if h.queries == nil {
		return nil
	}
	return experience.NewStore(experience.NewQueriesRepository(h.queries))
}

// candidateProfiler is the one operation the fit chain needs from the experience bank.
type candidateProfiler interface {
	Professional(ctx context.Context, userID int64, st resumeextract.Structured) (resumeextract.Professional, error)
}

// newCandidateProfiler builds the bank the fit chain reads, or nil when there are no
// queries to build it over.
func newCandidateProfiler(queries *db.Queries) candidateProfiler {
	if queries == nil {
		return nil
	}
	return experience.NewStore(experience.NewQueriesRepository(queries))
}

// companyInfo returns the raw company_info JSON for the job's company, or "" when the
// company is unknown or has none — the analysis then grounds on the job text alone.
func (h *matchHandlers) companyInfo(c *fiber.Ctx, companySlug string) string {
	if companySlug == "" {
		return ""
	}
	company, err := h.queries.GetCompany(c.Context(), companySlug)
	if err != nil {
		return ""
	}
	return string(company.CompanyInfo)
}

// stampsFresh reports whether a cached row still matches the live CV upload time, job
// content hash, profile language, and current model. A model change (LLM_MODEL upgrade)
// invalidates the cache so the improved model re-analyzes — the analogue of the
// enrichment version and semantic-embedder staleness guards. A language change
// (freehire#1837) invalidates it the same way, so switching the profile language re-runs
// the free-text commentary rather than leaving it in the old language until the CV or
// job text happens to change too. Absent-on-both-sides hash stamps count as unchanged
// (a non-board job with no content_hash is never re-crawled, so its text is stable and a
// NULL stamp must not force an endless recompute); a stamp appearing on one side only is
// a change.
func stampsFresh(row db.GetUserJobAnalysisRow, cvUploadedAt *time.Time, jobHash pgtype.Text, model, language string) bool {
	return stampsMatch(row.Model, row.CvUploadedAt, row.JobContentHash, row.Language, cvUploadedAt, jobHash, model, language)
}

// stampsMatch is stampsFresh over the raw stored stamps, so callers holding a different
// row type (e.g. the analysed-jobs list) can reuse the same freshness rule.
func stampsMatch(storedModel string, storedCV pgtype.Timestamptz, storedHash pgtype.Text, storedLanguage string, liveCV *time.Time, liveHash pgtype.Text, liveModel, liveLanguage string) bool {
	return storedModel == liveModel &&
		storedLanguage == liveLanguage &&
		sameTime(storedCV, liveCV) &&
		sameText(storedHash, liveHash)
}

func sameTime(stored pgtype.Timestamptz, live *time.Time) bool {
	if stored.Valid != (live != nil) {
		return false
	}
	return !stored.Valid || stored.Time.Equal(*live)
}

func sameText(stored, live pgtype.Text) bool {
	if stored.Valid != live.Valid {
		return false
	}
	return !stored.Valid || stored.String == live.String
}

// decodeAnalysis unmarshals a cached analysis blob, returning nil on empty/corrupt data
// (treated as "no analysis" — the caller re-offers a compute).
func decodeAnalysis(blob []byte) *matchanalysis.Analysis {
	if len(blob) == 0 {
		return nil
	}
	var a matchanalysis.Analysis
	if err := json.Unmarshal(blob, &a); err != nil {
		return nil
	}
	return &a
}
