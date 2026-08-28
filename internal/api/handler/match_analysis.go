package handler

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/credits"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/hardconstraint"
	"github.com/strelov1/freehire/internal/candidate/jobmatch"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// matchHandlers serves the per-job match surfaces: the read-only skill match, the
// on-demand three-stage LLM fit analysis (GET cached / POST run / SSE stream), and
// the caller's list of analyzed jobs.
//
// The use cases behind those routes — the cache, the staleness stamp, the credit rule and
// the coalescing — live in internal/candidate/fitanalysis, because the autopilot's two
// invisible halves reach them with no fiber ctx at all and the assistant's tools will too.
// What stays here is transport: slug → job, binding the caller's gateway credential, SSE
// framing, and rendering the 402 that fitanalysis refuses with.
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
	// degrades to a no-op. Held here to bind it per caller (fit.Analyzer wants the bound
	// one) — the unbound analyzer the service holds only answers ModelID.
	matchAnalysis *matchanalysis.Analyzer
	// llm binds the model client and the credential resolver, so an analysis is spent
	// under the account that asked for it.
	llm llmBinding
	// fit is the fit-analysis use case: the cache, the staleness rule, the credit decision
	// and the coalescing. It also answers the points balance every fit surface reports, so
	// this handler holds no ledger of its own. Nil in a fixture that exercises only the
	// skill-match or candidate-profile paths.
	fit *fitanalysis.Service
	// bank supplies the candidate's work history. Nil when there are no queries to build
	// it over, which reads as an empty bank — and an empty bank means no analysis.
	bank candidateProfiler
}

func newMatchHandlers(queries *db.Queries, userProfile *userprofile.Service, resumeStore *resume.Store, analyzer *matchanalysis.Analyzer, creditsStore *credits.Store) *matchHandlers {
	return &matchHandlers{
		queries:       queries,
		userProfile:   userProfile,
		resume:        resumeStore,
		matchAnalysis: analyzer,
		fit:           fitanalysis.New(queries, meterOrNil(creditsStore), analyzer),
		bank:          newCandidateProfiler(queries),
	}
}

// meterOrNil keeps a nil *credits.Store out of the service as a NON-nil interface holding a
// nil pointer, which would defeat fitanalysis's own nil-meter check and panic on the first
// balance read. Only fixtures assemble without a ledger; production always has one.
func meterOrNil(s *credits.Store) fitanalysis.Meter {
	if s == nil {
		return nil
	}
	return s
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

// renderCreditsRefusal turns fitanalysis's transport-agnostic refusal into the 402 body, and
// reports whether err was one. Every fit route gates through fitanalysis.Authorize, so this is
// the single place the refusal becomes a status.
func renderCreditsRefusal(c *fiber.Ctx, err error) (error, bool) {
	var refused *fitanalysis.InsufficientCreditsError
	if !errors.As(err, &refused) {
		return nil, false
	}
	return creditsError(c, refused.Balance), true
}

// liveStamps assembles what a cached analysis is judged fresh against right now. cvUploadedAt
// is passed in rather than re-read: the caller captured it up front, and a second read could
// date an analysis by a CV that replaced the one it was computed from.
func (h *matchHandlers) liveStamps(ctx context.Context, userID int64, job db.Job, cvUploadedAt *time.Time) fitanalysis.Stamps {
	return fitanalysis.Stamps{
		CVUploadedAt:   cvUploadedAt,
		JobContentHash: job.ContentHash,
		Model:          h.fit.ModelID(),
		Language:       h.callerLanguage(ctx, userID),
	}
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
	bal := h.fit.Balance(c.Context(), userID)
	analysis, stored, err := h.fit.Cached(c.Context(), userID, job.ID)
	if err != nil {
		return err
	}
	if analysis == nil {
		return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: true, Credits: bal}})
	}
	// Recompute the hard-constraint ceiling from the current job/résumé/dictionary and
	// apply it to the cached analysis on read — the cap is never stored, so a dictionary
	// change takes effect without marking the cache stale.
	h.capServedAnalysis(c.Context(), userID, job, analysis)
	stale := !h.liveStamps(c.Context(), userID, job, cvUploadedAt).Fresh(stored)
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
	// Take the credit before touching the LLM: the atomic debit IS the gate, so two
	// concurrent runs cannot both pass a balance only one of them could afford. A recompute
	// is free, and a run that produces nothing gives the credit back. The rule and the
	// refusal are the service's; only the status code is ours.
	reserved, err := h.fit.Reserve(c.Context(), userID, job.ID)
	if err != nil {
		if refusal, refused := renderCreditsRefusal(c, err); refused {
			return refusal
		}
		return err
	}

	// The caller's profile drives both the deterministic skills anchor and the location
	// dimension; a missing profile is tolerated (zero value → empty skills/preferences).
	profile, _ := h.userProfile.Get(c.Context(), userID)

	// Compute the hard-constraint blockers once: the unmet ones ground the prompt
	// (below) and the same list caps the served score (applyBlockers, after caching).
	blockers := h.jobBlockers(c.Context(), userID, job, profile)

	analysis, err := h.fit.Run(c.Context(), fitanalysis.Request{
		UserID:       userID,
		Job:          job,
		Analyzer:     h.boundAnalyzer(c.Context(), userID),
		Input:        h.buildAnalysisInput(c, job, userID, profile, blockers, language),
		CVUploadedAt: cvUploadedAt,
		Reserved:     reserved,
	}, nil)
	if err != nil {
		// Best-effort: log (never the CV/job text) and serve no analysis.
		log.Printf("matchanalysis: analyze failed for user %d job %d: %v", userID, job.ID, err)
		return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: true}})
	}
	if analysis == nil {
		// LLM unconfigured — nothing was cached.
		return c.JSON(fiber.Map{"data": matchAnalysisResponse{HasCV: true}})
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

// boundAnalyzer returns the analyzer that spends under this caller's own gateway credential.
// Binding is a network call, so a streaming caller makes it before its headers go out —
// afterwards it would stall a stream the client is already reading.
func (h *matchHandlers) boundAnalyzer(ctx context.Context, userID int64) *matchanalysis.Analyzer {
	return h.matchAnalysis.As(h.llm.bind(ctx, userID, llm.Feature(tagMatchAnalysis)))
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
//
// Neither half is chargeable, and the request it carries says so by leaving Reserved false:
// the cold-start pre-run is unmetered by design, tracked only by the LLM spend attribution
// every call already carries — so it reserves nothing and has nothing to release.
type autopilotAnalysis struct {
	fit *fitanalysis.Service
	req fitanalysis.Request
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
		fit: h.fit,
		req: fitanalysis.Request{
			UserID:       userID,
			Job:          job,
			Analyzer:     h.boundAnalyzer(c.Context(), userID),
			Input:        h.buildAnalysisInput(c, job, userID, profile, blockers, language),
			CVUploadedAt: cvUploadedAt,
		},
	}
}

// ensure fills the cache before a cold-start autopilot run, whose first tool call reads it
// and errors without one. Coalesced, best-effort, and never charged — the rules are
// fitanalysis.Service.Ensure's; the claim is taken here because it is per attempt, not per
// prepared run.
func (a *autopilotAnalysis) ensure(ctx context.Context) {
	if a == nil {
		return
	}
	req := a.req
	req.Claim = a.fit.Claim(req.UserID, req.Job.ID)
	a.fit.Ensure(ctx, req)
}

// refresh recomputes the analysis after the run, unconditionally — see
// fitanalysis.Service.Refresh. It claims nothing: by the time it runs the turn is over, so
// there is no concurrent caller for this pair to coalesce with.
func (a *autopilotAnalysis) refresh(ctx context.Context) {
	if a == nil {
		return
	}
	a.fit.Refresh(ctx, a.req)
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
