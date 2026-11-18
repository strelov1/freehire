// Package fitanalysis holds the fit-analysis use cases: the cached read, the three-stage chain
// run, the allowance rule that decides which runs are charged, and the coalescing that keeps two
// concurrent callers for one pair from paying twice.
//
// The chain has four entry points and only one speaks HTTP (the API run, the tailoring stream,
// and the autopilot's cold-start fill and post-run refresh, which run detached with no fiber
// ctx), so these rules live here rather than on a handler.
//
// The prompt chain, the Analysis type and its sanitize ceilings stay in
// internal/candidate/matchanalysis. Transport (slug to job, gateway credential, SSE framing,
// rendering the 402 from RefusedError) stays with the caller.
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
// it; a fake backs the DB-less tests. It uses the generated row types directly: they are
// read-only projections, so domain structs would add a mapping layer for nothing.
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
// Decision so the caller can render what is left, when the day resets and where to upgrade.
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

	// Analyzer spends under the candidate's own gateway credential. The caller binds it: minting
	// is a network call a streaming caller must make BEFORE its headers go out.
	Analyzer *matchanalysis.Analyzer

	// Input is the assembled chain input. Its Language is also the language the cache is
	// stamped with, so the two can never disagree.
	Input matchanalysis.Input

	// CVUploadedAt stamps the cache with the CV actually analysed. The caller captures it up
	// front: the chain takes seconds, and a later re-read could stamp a newer CV's time on an
	// older analysis.
	CVUploadedAt *time.Time

	// Reserved says THIS caller already paid for the run (Reserve's answer). Consumption is
	// idempotent per (candidate, feature, job), so two racing callers collapse into one ledger
	// row. A run that produces nothing releases what it took.
	Reserved bool

	// Claim is this caller's role in the coalesced compute, from Claim. Nil means not
	// coalescing (the plain POST run); the two paths that can collide, the visible stream and
	// the autopilot fill, both claim.
	Claim *Claim
}

// Service is the fit-analysis use cases.
type Service struct {
	store    Store
	meter    Meter
	analyzer *matchanalysis.Analyzer
	coalesce coordinator
}

// New builds the service. The analyzer is the UNBOUND one: it answers ModelID for the stamps,
// while every compute runs under the per-candidate analyzer in Request.Analyzer.
func New(store Store, meter Meter, analyzer *matchanalysis.Analyzer) *Service {
	return &Service{store: store, meter: meter, analyzer: analyzer}
}

// ModelID is the model a cached row is stamped with and judged fresh against.
func (s *Service) ModelID() string { return s.analyzer.ModelID() }

// Claim claims the compute for (candidate, job); see Claim's own documentation for the
// leader/follower contract.
func (s *Service) Claim(userID, jobID int64) *Claim { return s.coalesce.Claim(userID, jobID) }

// Standing reports where the candidate stands on today's fit-analysis allowance, or nil on a DB
// error (logged), with no meter wired, or on a nil service. Best-effort: the atomic Consume is
// the real ceiling.
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

// Reserve decides whether this run is chargeable and, if so, TAKES the allowance before the
// chain starts; the atomic consumption is the gate (its row lock arbitrates concurrent runs).
//
// A run is chargeable only when it would be the candidate's FIRST analysis of that job, so
// recomputes are free. A refused run returns RefusedError before the LLM is touched and before
// a streaming caller opens its response, so the refusal can still be a status.
//
// Metering fails OPEN: an unreachable counter logs and lets the run through unreserved. An
// uncharged run is a smaller wrong than a candidate blocked by our accounting.
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
	// True even when the consumption no-opped on an already-charged ref: no analysis exists, so
	// that charge bought nothing and releasing it if this run fails is correct.
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

// Cached serves the analysis already computed for (candidate, job), never calling the LLM, with
// the stamps it was computed under. It answers a nil analysis when none is cached or the stored
// blob is unreadable.
//
// Stamps come back instead of a staleness verdict so the caller builds the live ones only when
// there is something to judge (most opened jobs were never analysed). The comparison rule stays
// in Stamps.Fresh.
//
// The analysis is the UNCAPPED one; the caller recomputes and applies the hard-constraint
// ceiling on every read, so dictionary changes need no cache invalidation.
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
// when r.Reserved says this caller owes one.
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
//
// The two gates below answer different questions, so they are two flags: a failed cache write
// must not be published to followers as success, or Follow would serve an older row as this
// run's result.
func (s *Service) Run(ctx context.Context, r Request, emit func(matchanalysis.Event)) (*matchanalysis.Analysis, error) {
	// cached: did the CACHE end up holding this run's result? That is what the claim publishes.
	cached := false
	if r.Claim.IsLeader() {
		defer func() { r.Claim.Release(cached) }()
	}
	// produced: did the chain yield an analysis this caller will be handed? If not, nothing is
	// owed, and the caller that ran the chain (leader or non-coalescing; followers go to Follow)
	// releases the ref for everyone: when the leader is the free autopilot pre-run, the standing
	// charge belongs to a follower about to be told there is nothing.
	//
	// Deliberately NOT `cached`: a caller that received an analysis got what it paid for even if
	// the cache write failed. Deferred so a panic also gives the credit back.
	produced := false
	defer func() {
		if !produced {
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
	produced = true
	cached = s.Cache(ctx, r.UserID, r.Job, r.CVUploadedAt, r.Input.Language, analysis) == nil
	return analysis, nil
}

// ErrUnavailable reports that no analysis can be served: the leader failed or left an
// unreadable row. One error for both; the caller's answer is the same.
var ErrUnavailable = errors.New("fitanalysis: analysis unavailable")

// Follow is the degrade for the caller that lost the race for (candidate, job): it waits for
// the leader and returns the analysis that landed in the cache instead of paying for a second
// chain.
//
// The leader's success is checked before the cache is trusted: a failed leader leaves an OLDER
// or absent row, which would be served as this run's live result.
//
// It still charges when r.Reserved says this caller owes a credit (e.g. following the free
// autopilot pre-run).
func (s *Service) Follow(ctx context.Context, r Request) (*matchanalysis.Analysis, error) {
	if !r.Claim.wait() {
		// No release: the debit is shared with the leader, and a leader whose cache write failed
		// still returned its analysis to whoever paid. The leader releases on failure for
		// everyone; see Run.
		return nil, ErrUnavailable
	}
	row, err := s.store.GetUserJobAnalysis(ctx, db.GetUserJobAnalysisParams{UserID: r.UserID, JobID: r.Job.ID})
	analysis := DecodeAnalysis(row.Analysis)
	if err != nil || analysis == nil {
		return nil, ErrUnavailable
	}
	return analysis, nil
}

// Ensure computes and caches the analysis when none is cached yet, for the cold-start autopilot
// run whose first tool call reads the cache and errors without one.
//
// It REPORTS whether the caller ended up with an analysis: a silently failed fill sent
// autopilot runs out with no requirement list, editing a CV against nothing.
//
// It never charges (this path is unmetered), so r.Reserved is ignored.
//
// r.Claim decides the coalescing: the tailoring workspace's visible stream starts at the same
// cold start, so both routinely race for one pair. A follower waits, then reads the cache; the
// leader's outcome matters, since a follower reporting success on a failed leader would hand
// the run the same empty plan.
//
// The answer is taken from the CACHE rather than from which branch ran, so leader, follower,
// cache hit and unconfigured analyzer are all answered by one question: is there a row to walk?
func (s *Service) Ensure(ctx context.Context, r Request) error {
	if !r.Claim.IsLeader() {
		r.Claim.wait()
		return s.cachedAnalysisPresent(ctx, r)
	}
	succeeded := false
	defer func() {
		r.Claim.Release(succeeded)
		// This half never charges, but it can LEAD a paying caller. When it produces nothing, the
		// charge standing against this ref is the waiting follower's, so the leader gives it back
		// on their behalf.
		if !succeeded {
			s.release(ctx, r.UserID, r.Job.ID)
		}
	}()

	if _, err := s.store.GetUserJobAnalysis(ctx,
		db.GetUserJobAnalysisParams{UserID: r.UserID, JobID: r.Job.ID}); err == nil {
		succeeded = true // already cached: a good row for a follower to read
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("matchanalysis: checking cache before an autopilot run, user %d job %d: %v",
			r.UserID, r.Job.ID, err)
		return fmt.Errorf("%w: reading the cache: %w", ErrNoAnalysis, err)
	}
	succeeded = s.compute(ctx, r, "inline compute before an autopilot run")
	if !succeeded {
		return ErrNoAnalysis
	}
	return nil
}

// cachedAnalysisPresent answers whether a row a run can walk is in the cache now. A follower
// asks it after its leader finishes, and it asks the STORE: the leader reports only that it is
// done.
func (s *Service) cachedAnalysisPresent(ctx context.Context, r Request) error {
	_, err := s.store.GetUserJobAnalysis(ctx,
		db.GetUserJobAnalysisParams{UserID: r.UserID, JobID: r.Job.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoAnalysis
	}
	if err != nil {
		return fmt.Errorf("%w: reading the cache: %w", ErrNoAnalysis, err)
	}
	return nil
}

// Refresh UNCONDITIONALLY recomputes the chain and overwrites the cache, even when an analysis
// was already there. It never charges, for the same reason Ensure does not.
func (s *Service) Refresh(ctx context.Context, r Request) {
	s.compute(ctx, r, "post-autopilot refresh")
}

// compute runs the chain and caches what it produced, reporting whether the cache is left
// holding this call's own result, which a coalescing follower waits to learn. It never charges:
// both callers are the autopilot's unmetered halves.
//
// Unlike Run, one flag is the whole answer: neither caller is handed the analysis, so if the
// write failed the follower gets nothing and Ensure's defer gives back the charge on their
// behalf.
func (s *Service) compute(ctx context.Context, r Request, stage string) bool {
	analysis, err := r.Analyzer.Analyze(ctx, r.Input)
	if err != nil {
		log.Printf("matchanalysis: %s, user %d job %d: %v", stage, r.UserID, r.Job.ID, err)
		return false
	}
	if analysis == nil {
		return false // LLM unconfigured — nothing to cache
	}
	return s.Cache(ctx, r.UserID, r.Job, r.CVUploadedAt, r.Input.Language, analysis) == nil
}

// Cache upserts the analysis stamped with the analysed CV's upload time, the job content hash,
// the language it was written in, and the model that produced it. It takes a plain context
// (never a request's) so a caller streaming over SSE can cache after its handler has returned.
// Best-effort towards the CALLER, which logs and keeps its own analysis.
//
// It still returns the failure because a coalescing FOLLOWER reads what this wrote: a failed
// write leaves an older or absent row that Follow would otherwise serve as this run's result.
//
// It stores the UNCAPPED analysis; the caller applies the hard-constraint ceiling to the served
// copy only, so dictionary changes need no cache invalidation.
func (s *Service) Cache(ctx context.Context, userID int64, job db.Job, cvUploadedAt *time.Time, language string, analysis *matchanalysis.Analysis) error {
	blob, err := json.Marshal(analysis)
	if err != nil {
		log.Printf("matchanalysis: encode analysis for user %d job %d: %v", userID, job.ID, err)
		return err
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
		return err
	}
	return nil
}

// release gives back a credit reserved for a run that produced nothing. Best-effort (the
// candidate is already being told the analysis is unavailable) and idempotent, so every failure
// path may call it without working out whether it owes one.
func (s *Service) release(ctx context.Context, userID, jobID int64) {
	if s.meter == nil {
		return
	}
	// The cleanup must survive the cancellation that caused it: a client disconnect cancels the
	// request context, and a release on that context could not open its transaction.
	detached := context.WithoutCancel(ctx)

	// An allowance buys HAVING the analysis, not the attempt that produced it. A failed later
	// run (recompute, autopilot refresh) must not give back what an EARLIER run paid, so a
	// release stops when an analysis exists for the pair.
	//
	// A service with no store simply releases (a tool runs in an SSE writer's goroutine where a
	// panic reaches no recover). A read that FAILS releases too: leaving a candidate charged is
	// the worse error.
	if s.store != nil {
		read, cancelRead := context.WithTimeout(detached, releaseTimeout)
		analysis, _, err := s.Cached(read, userID, jobID)
		cancelRead()
		if err == nil && analysis != nil {
			return
		}
	}

	// Its own budget, not the read's leftovers: a slow read must not starve the refund.
	write, cancel := context.WithTimeout(detached, releaseTimeout)
	defer cancel()
	if err := s.meter.Release(write, userID, plan.FeatureFit, debitRef(jobID)); err != nil {
		log.Printf("plan: releasing a fit-analysis reservation for user %d job %d: %v", userID, jobID, err)
	}
}

// releaseTimeout bounds the detached cleanup: generous for two small statements, short enough
// that a wedged database cannot pile up goroutines.
const releaseTimeout = 5 * time.Second

// debitRef identifies what a match credit was spent on. One charge per job, ever.
func debitRef(jobID int64) string { return strconv.FormatInt(jobID, 10) }
