// Package fitanalysis is the fit-analysis use cases: reading the cached analysis for a
// (candidate, job), running the three-stage chain over it, the allowance rule that decides
// which runs are charged, and the coalescing that keeps two concurrent callers for one pair
// from paying for two chains.
//
// It exists because the chain has four entry points and only one of them speaks HTTP: the
// cached read and the on-demand run behind the API, the tailoring workspace's streamed run,
// and the autopilot's two invisible halves — a cold-start fill and a post-run refresh — that
// execute after the request, from a detached goroutine, with no fiber ctx at all. A rule
// written where one of those callers happens to reach it is a rule the other three never
// meet; that is how the metering, the staleness stamp and the coalescing came to live on a
// Fiber handler, reachable only through a *fiber.Ctx and testable only through the tagged
// handler tests.
//
// The prompt chain itself, the Analysis type and its sanitize ceilings stay in
// internal/candidate/matchanalysis — this package orchestrates that domain, it does not
// replace it. What stays with the caller is transport: resolving a slug to a job, binding
// the candidate's gateway credential, framing SSE, and rendering the 402 that
// RefusedError describes.
package fitanalysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// Store reads and writes the per-(candidate, job) cached fit analysis. *db.Queries satisfies
// it; a fake backs the DB-less tests.
//
// It trades the generated row types deliberately, the same way internal/ai/embed's ports do:
// the rows are wide projections this package only ever reads fields off, and restating them
// as domain structs would buy a mapping layer nobody reads.
type Store interface {
	GetUserJobAnalysis(ctx context.Context, arg db.GetUserJobAnalysisParams) (db.GetUserJobAnalysisRow, error)
	UpsertUserJobAnalysis(ctx context.Context, arg db.UpsertUserJobAnalysisParams) error
	ListUserJobAnalyses(ctx context.Context, userID int64) ([]db.ListUserJobAnalysesRow, error)
}

// Meter is the plan allowance as this package needs it. *plan.Store satisfies it.
//
// A nil Meter is a working no-op — standing unknown, nothing consumed — so a caller
// assembled without one (a minimal test app) runs the chain rather than panicking.
// Production always wires one.
type Meter interface {
	Standing(ctx context.Context, userID int64, f plan.Feature) (plan.Standing, error)
	Consume(ctx context.Context, userID int64, f plan.Feature, ref string) (plan.Decision, error)
	// Release gives back an allowance reserved for work that produced nothing.
	Release(ctx context.Context, userID int64, f plan.Feature, ref string) error
}

// RefusedError refuses a run the candidate's plan does not allow right now. It carries the
// decision because the refusal has to be renderable — the SPA shows what is left, when the
// day resets, and where to upgrade — and carrying it here is what lets the decision live
// outside the transport that formats it.
type RefusedError struct {
	Decision plan.Decision
}

func (e *RefusedError) Error() string {
	return fmt.Sprintf("fitanalysis: today's fit-analysis allowance is spent (%d of %d used)",
		e.Decision.Used, e.Decision.Limit)
}

// Request is one fit-analysis compute: who it is for, what it analyses, and whether this
// caller has already paid for it.
type Request struct {
	UserID int64
	Job    db.Job

	// Analyzer spends under the candidate's own gateway credential. It is bound by the
	// caller rather than here because minting a credential is a network call, and a
	// streaming caller must make it BEFORE its headers go out — afterwards it would stall a
	// stream the client is already reading.
	Analyzer *matchanalysis.Analyzer

	// Input is the assembled chain input. Its Language is also the language the cache is
	// stamped with, so the two can never disagree.
	Input matchanalysis.Input

	// CVUploadedAt stamps the cache with the CV that was actually analysed. Captured by the
	// caller up front: the chain takes seconds, and re-reading it afterwards would risk
	// stamping a newer CV's time on an older CV's analysis.
	CVUploadedAt *time.Time

	// Reserved says THIS caller has already paid for the run (Reserve's answer), not the
	// leader's. Two callers racing for one never-analysed job each reserve, and consumption
	// is idempotent per (candidate, feature, job), so both collapse into a single ledger row
	// — two tabs on the same job is neither a double charge nor a discount.
	//
	// A run that produces nothing releases what it took, so a failed analysis still costs
	// nothing.
	Reserved bool

	// Claim is this caller's role in the coalesced compute, from Claim. Nil means the caller
	// is not coalescing at all (the plain POST run), which races nothing today because the
	// two paths that can collide — the visible stream and the autopilot's fill — both claim.
	Claim *Claim
}

// Service is the fit-analysis use cases.
type Service struct {
	store    Store
	meter    Meter
	analyzer *matchanalysis.Analyzer
	coalesce coordinator
}

// New builds the service over the analysis cache, the plan meter, and the analyzer whose
// model identifies a cached row. The analyzer here is the UNBOUND one: it answers ModelID for
// the stamps, while every actual compute runs under the per-candidate analyzer the caller
// puts in Request.Analyzer.
func New(store Store, meter Meter, analyzer *matchanalysis.Analyzer) *Service {
	return &Service{store: store, meter: meter, analyzer: analyzer}
}

// ModelID is the model a cached row is stamped with and judged fresh against.
func (s *Service) ModelID() string { return s.analyzer.ModelID() }

// Claim claims the compute for (candidate, job); see Claim's own documentation for the
// leader/follower contract.
func (s *Service) Claim(userID, jobID int64) *Claim { return s.coalesce.Claim(userID, jobID) }

// Standing reports where the candidate stands on their fit-analysis allowance today, or nil
// on a DB error (logged), with no meter wired, or on a nil service. Best-effort: a transient
// hiccup must neither block a legitimate analysis nor refuse the caller — the atomic Consume
// remains the real ceiling, and a caller assembled without a fit service (a fixture
// exercising an adjacent surface) reads as "standing unknown" rather than panicking.
func (s *Service) Standing(ctx context.Context, userID int64) *plan.Standing {
	if s == nil || s.meter == nil {
		return nil
	}
	st, err := s.meter.Standing(ctx, userID, plan.FeatureFit)
	if err != nil {
		log.Printf("plan: standing for user %d: %v", userID, err)
		return nil
	}
	return &st
}

// Reserve decides whether this run is chargeable and, if it is, TAKES the allowance before
// the chain starts. The atomic consumption is the gate.
//
// It used to check what was left and charge afterwards, which is a check-then-act: two
// concurrent runs for two never-analysed jobs could both pass a check only one of them could
// afford, and the loser's charge then failed silently — after its analysis had been computed,
// cached and served. Consuming first makes the counter's own row lock the arbiter, so the
// second run is refused rather than given away.
//
// A run is chargeable only when it would be the candidate's FIRST analysis of that job, so a
// recompute is always free and an analysis cached before metering shipped re-runs for nothing.
// A run the plan does not allow is refused with RefusedError, before the LLM is touched and
// before a streaming caller opens its response — so the refusal can still be a status rather
// than an event on a stream that already returned 200.
//
// Metering fails OPEN. An unreachable counter logs and lets the run through unreserved,
// exactly as the balance read always did: bookkeeping must never be able to refuse a
// legitimate analysis, and an uncharged run is a smaller wrong than a candidate blocked by
// our accounting.
func (s *Service) Reserve(ctx context.Context, userID, jobID int64) (reserved bool, err error) {
	isNew, err := s.isNew(ctx, userID, jobID)
	if err != nil || !isNew {
		return false, err
	}
	if s.meter == nil {
		return false, nil
	}
	d, err := s.meter.Consume(ctx, userID, plan.FeatureFit, debitRef(jobID))
	if errors.Is(err, plan.ErrRefused) {
		return false, &RefusedError{Decision: d}
	}
	if err != nil {
		log.Printf("plan: reserving a fit analysis for user %d job %d: %v", userID, jobID, err)
		return false, nil
	}
	// True even when the consumption no-opped on an already-charged ref. isNew said there is
	// no analysis for this job, so a charge standing against it has bought the candidate
	// nothing — and releasing it if this run also fails is the right answer, not a refund of
	// somebody else's work.
	return true, nil
}

// isNew reports whether analysing (userID, jobID) would be the candidate's first analysis of
// that job — i.e. no cached row exists.
func (s *Service) isNew(ctx context.Context, userID, jobID int64) (bool, error) {
	_, err := s.store.GetUserJobAnalysis(ctx, db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID})
	if err == nil {
		return false, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	return false, err
}

// Cached serves the analysis already computed for (candidate, job), never calling the LLM,
// together with the stamps it was computed under. It answers a nil analysis when none is
// cached or the stored blob is unreadable — the caller then re-offers a compute.
//
// The stamps come back rather than a ready-made staleness verdict so the caller assembles the
// live ones only when there is something to judge: most jobs a candidate opens were never
// analysed, and reading a profile language to date an analysis that does not exist would put
// a query on the commonest read there is. The rule those stamps are compared BY still lives
// here — Stamps.Fresh — so no caller restates which fields matter.
//
// The returned analysis is the UNCAPPED one the chain produced: the hard-constraint ceiling is
// recomputed and applied by the caller on every read, so a dictionary change takes effect
// without any cache invalidation.
func (s *Service) Cached(ctx context.Context, userID, jobID int64) (*matchanalysis.Analysis, Stamps, error) {
	row, err := s.store.GetUserJobAnalysis(ctx, db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, Stamps{}, nil
	}
	if err != nil {
		return nil, Stamps{}, err
	}
	analysis := DecodeAnalysis(row.Analysis)
	if analysis == nil {
		return nil, Stamps{}, nil
	}
	return analysis, StoredStamps(row.Model, row.CvUploadedAt, row.JobContentHash, row.Language), nil
}

// List returns the jobs the candidate has run the analysis on, newest first.
func (s *Service) List(ctx context.Context, userID int64) ([]db.ListUserJobAnalysesRow, error) {
	return s.store.ListUserJobAnalyses(ctx, userID)
}

// Run executes the chain for r and leaves the result in the cache, charging the candidate
// when r.Chargeable says this caller owes one.
//
// emit receives each progress event as it resolves; a nil emit runs the chain without
// streaming, which is the only difference between the on-demand endpoint and the streamed one.
//
// A nil analysis with a nil error means the LLM is unconfigured — nothing was computed and
// nothing cached, which every caller degrades to "no analysis" rather than an error.
//
// When r.Claim leads, the claim is released here, from a defer: a panic anywhere between the
// chain, the cache write and the debit still wakes a waiting follower instead of stranding
// that pair behind a leader that never finishes.
func (s *Service) Run(ctx context.Context, r Request, emit func(matchanalysis.Event)) (*matchanalysis.Analysis, error) {
	succeeded := false
	if r.Claim.IsLeader() {
		defer func() { r.Claim.Release(succeeded) }()
	}
	// Nothing produced means nothing owed — and whoever RAN THE CHAIN says so for everyone.
	// Only a leader or a caller that is not coalescing at all reaches Run (a follower goes to
	// Follow), so this is exactly the caller that knows the outcome. It releases the ref
	// rather than just its own reservation: when the leader is the free autopilot pre-run, the
	// charge standing against that ref belongs to a follower about to be told there is nothing.
	//
	// Deferred so a panic anywhere below — in the chain, the emit callback or the cache write
	// — gives the credit back too.
	defer func() {
		if !succeeded {
			s.release(ctx, r.UserID, r.Job.ID)
		}
	}()

	var analysis *matchanalysis.Analysis
	var err error
	if emit == nil {
		analysis, err = r.Analyzer.Analyze(ctx, r.Input)
	} else {
		analysis, err = r.Analyzer.AnalyzeStream(ctx, r.Input, emit)
	}
	if err != nil {
		return nil, err
	}
	if analysis == nil {
		return nil, nil // LLM unconfigured — nothing to cache
	}
	s.Cache(ctx, r.UserID, r.Job, r.CVUploadedAt, r.Input.Language, analysis)
	succeeded = true
	return analysis, nil
}

// ErrUnavailable reports that no analysis can be served: the concurrent leader of this pair
// failed, or what it left behind is unreadable. It is deliberately one error for both — the
// caller's answer is the same and neither case tells the candidate anything actionable.
var ErrUnavailable = errors.New("fitanalysis: analysis unavailable")

// Follow is the graceful degrade for the caller that lost the race for (candidate, job): it
// waits for the leader and returns the analysis that landed in the cache, so this caller
// serves a real result instead of paying for a second identical chain.
//
// The leader's success is checked before the cache is trusted at all. A leader whose attempt
// failed leaves an OLDER (or absent) row behind, not a fresh one, and reading it
// unconditionally would serve a stale analysis dressed up as this run's live result — a
// follower must report the same failure the leader saw, not paper over it.
//
// It still charges when r.Chargeable says this caller owes a credit. Following the free
// autopilot pre-run is exactly the case that must still charge a genuinely new analysis.
func (s *Service) Follow(ctx context.Context, r Request) (*matchanalysis.Analysis, error) {
	if !r.Claim.wait() {
		// No release here, deliberately. The debit is per (candidate, job) and this caller
		// shares it with the leader, so giving it back could void a charge the leader's own
		// result earned — a leader whose CACHE WRITE failed still returned its analysis to
		// whoever paid for it. The leader releases on failure for everyone; see Run.
		return nil, ErrUnavailable
	}
	row, err := s.store.GetUserJobAnalysis(ctx, db.GetUserJobAnalysisParams{UserID: r.UserID, JobID: r.Job.ID})
	analysis := DecodeAnalysis(row.Analysis)
	if err != nil || analysis == nil {
		return nil, ErrUnavailable
	}
	return analysis, nil
}

// Ensure computes and caches the analysis when none is cached yet. It exists for the
// cold-start autopilot run, whose first tool call reads the cache and errors without one —
// this is what lets that run start without requiring the candidate to have produced an
// analysis first.
//
// Best-effort and silent: a lookup or compute failure (no LLM configured, an analyzer error)
// is logged and left uncached, exactly as the on-demand run already degrades. It never
// charges — this path is unmetered, tracked only by the same LLM spend attribution every call
// already carries (see the tailor-coldstart-autopilot design's "no new metering" decision) —
// so r.Chargeable is ignored here rather than trusted.
//
// r.Claim decides the coalescing: the tailoring workspace's visible stream starts at the same
// cold start this runs at, so both routinely race for the identical pair. A follower waits and
// returns without touching the LLM; whether the leader actually cached anything is not this
// function's concern, exactly as before the coalescing existed.
func (s *Service) Ensure(ctx context.Context, r Request) {
	if !r.Claim.IsLeader() {
		r.Claim.wait()
		return
	}
	succeeded := false
	defer func() {
		r.Claim.Release(succeeded)
		// This half never charges, but it can LEAD a paying caller. When it produces nothing,
		// the follower waiting on it gets nothing either, and the charge standing against this
		// ref is that follower's — so the leader gives it back on their behalf.
		if !succeeded {
			s.release(ctx, r.UserID, r.Job.ID)
		}
	}()

	if _, err := s.store.GetUserJobAnalysis(ctx,
		db.GetUserJobAnalysisParams{UserID: r.UserID, JobID: r.Job.ID}); err == nil {
		succeeded = true // already cached — nothing to compute, and a good row for a follower to read
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("matchanalysis: checking cache before an autopilot run, user %d job %d: %v",
			r.UserID, r.Job.ID, err)
		return
	}
	succeeded = s.compute(ctx, r, "inline compute before an autopilot run")
}

// Refresh UNCONDITIONALLY recomputes the chain and overwrites the cache, even when nothing was
// cached or an analysis was already there. This is what repeals the
// fit-analysis-post-autopilot-verify design's predecessor rule that the fit analysis is a
// frozen snapshot of the base profile. It never charges, for the same reason Ensure does not.
func (s *Service) Refresh(ctx context.Context, r Request) {
	s.compute(ctx, r, "post-autopilot refresh")
}

// compute runs the chain and caches what it produced, reporting whether the cache is left
// holding this call's own result — which is what a coalescing follower waits to learn. It
// never charges: both callers are the autopilot's own unmetered halves.
func (s *Service) compute(ctx context.Context, r Request, stage string) bool {
	analysis, err := r.Analyzer.Analyze(ctx, r.Input)
	if err != nil {
		log.Printf("matchanalysis: %s, user %d job %d: %v", stage, r.UserID, r.Job.ID, err)
		return false
	}
	if analysis == nil {
		return false // LLM unconfigured — nothing to cache
	}
	s.Cache(ctx, r.UserID, r.Job, r.CVUploadedAt, r.Input.Language, analysis)
	return true
}

// Cache upserts the analysis stamped with the analysed CV's upload time, the job content
// hash, the language it was written in, and the model that produced it. It takes a plain
// context (never a request's) so a caller streaming over SSE can cache after its handler has
// returned. Best-effort: a cache failure is logged, not surfaced.
//
// What it stores is the UNCAPPED analysis. The hard-constraint ceiling is applied to the
// served copy only, by the caller, so a dictionary change takes effect on the next read with
// no cache invalidation.
func (s *Service) Cache(ctx context.Context, userID int64, job db.Job, cvUploadedAt *time.Time, language string, analysis *matchanalysis.Analysis) {
	blob, err := json.Marshal(analysis)
	if err != nil {
		return
	}
	if err := s.store.UpsertUserJobAnalysis(ctx, db.UpsertUserJobAnalysisParams{
		UserID:         userID,
		JobID:          job.ID,
		Analysis:       blob,
		Model:          s.ModelID(),
		CvUploadedAt:   pgconv.Timestamptz(cvUploadedAt),
		JobContentHash: job.ContentHash,
		Language:       language,
	}); err != nil {
		log.Printf("matchanalysis: cache analysis for user %d job %d: %v", userID, job.ID, err)
	}
}

// release gives back a credit reserved for a run that produced nothing, and is best-effort:
// the candidate is already being told the analysis is unavailable, and a failure to return
// their point is not something to fail them a second time over. It is idempotent, so every
// failure path may call it without first working out whether it owes one.
func (s *Service) release(ctx context.Context, userID, jobID int64) {
	if s.meter == nil {
		return
	}
	// The cleanup has to survive the cancellation that caused it. A client that disconnects
	// mid-run cancels the request context, the chain fails with it, and a release on that same
	// context cannot even open its transaction — leaving the candidate charged for an analysis
	// they never received, in exactly the case this exists for.
	detached := context.WithoutCancel(ctx)

	// An allowance buys HAVING the analysis, not the attempt that produced it. A later run
	// that fails — a recompute, the autopilot's refresh — must not give back what an EARLIER
	// run paid, so a release stops when an analysis exists for the pair.
	//
	// A service with no store cannot answer that question and simply releases: it is the
	// nil-degrades-rather-than-panics rule the reads already follow, and a tool runs inside an
	// SSE writer's goroutine where a panic reaches no recover. A read that FAILS releases too —
	// an unanswered question is not a yes, and leaving a candidate charged is the worse error.
	if s.store != nil {
		read, cancelRead := context.WithTimeout(detached, releaseTimeout)
		analysis, _, err := s.Cached(read, userID, jobID)
		cancelRead()
		if err == nil && analysis != nil {
			return
		}
	}

	// Its own budget, not the read's leftovers: a slow cache read must not spend the deadline
	// the refund needs and leave the reservation standing.
	write, cancel := context.WithTimeout(detached, releaseTimeout)
	defer cancel()
	if err := s.meter.Release(write, userID, plan.FeatureFit, debitRef(jobID)); err != nil {
		log.Printf("plan: releasing a fit-analysis reservation for user %d job %d: %v", userID, jobID, err)
	}
}

// releaseTimeout bounds the detached cleanup. Generous for two small statements, and short
// enough that a wedged database cannot pile up goroutines behind it.
const releaseTimeout = 5 * time.Second

// debitRef identifies what a match credit was spent on. One charge per job, ever.
func debitRef(jobID int64) string { return strconv.FormatInt(jobID, 10) }
