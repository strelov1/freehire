package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	sentryfiber "github.com/getsentry/sentry-go/fiber"
	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/hardconstraint"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
)

// StreamMatchAnalysis runs the three-stage fit chain over Server-Sent Events, emitting stage
// progress, best-effort thinking tokens, and each section as it resolves, then caching
// the final analysis exactly as PostMatchAnalysis does. Cookie or API key; unknown slug 404.
// The stream always opens with a `meta` event (has_cv); when no CV is stored it closes
// after that. Everything the stream needs is captured before the body writer starts,
// because the fiber ctx is released once this handler returns.
//
// Coalesced through fitanalysis.Claim: this is also what the tailoring workspace's cold start
// opens to animate the fit analysis, at the same moment the autopilot's own invisible
// autopilotAnalysis.ensure may be racing for the identical (user, job). Whichever claims
// leadership runs the chain for real; the other becomes a follower and replays the result via
// followMatchAnalysis instead of paying for a second chain.
func (h *matchHandlers) StreamMatchAnalysis(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	job, err := h.queries.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err
	}
	cvUploadedAt, hasCV := h.cvUploadedAt(c, userID)
	// Reserve the credit before opening the stream (the fiber ctx is still valid here, so an
	// out-of-credits new job returns a real 402 instead of an SSE error). Only a CV-backed
	// request would run the LLM; without one the stream just reports has_cv. A recompute is
	// free, and a run that produces nothing gives the credit back.
	//
	// Every caller reserves for ITSELF, whether it ends up leading or following the compute
	// below — an out-of-credits caller must still 402 even when someone else is already
	// computing the same analysis for free reasons (autopilotAnalysis.ensure never reaches
	// this gate at all — see prepareAutopilotRun). Reserving twice for one job is safe: the
	// debit is idempotent per (user, feature, job), so a leader and a follower collapse into
	// a single ledger row, whichever reaches it first.
	reserved := false
	if hasCV {
		var err error
		if reserved, err = h.fit.Reserve(c.Context(), userID, job.ID); err != nil {
			if refusal, refused := renderCreditsRefusal(c, err); refused {
				return refusal
			}
			return err
		}
	}

	// Leadership for (user, job) is claimed after the gate above, not before: nothing here
	// can fail, so there is no path where a caller becomes leader and then has to immediately
	// hand leadership back. A follower (almost always the cold-start autopilot's own invisible
	// autopilotAnalysis.ensure, racing for the same pair) runs neither the LLM nor the reads
	// below — see fitanalysis.Claim and followMatchAnalysis.
	var claim *fitanalysis.Claim
	if hasCV {
		claim = h.fit.Claim(userID, job.ID)
	}
	profile, _ := h.userProfile.Get(c.Context(), userID)

	// Compute the hard-constraint blockers exactly as the POST path does: the unmet
	// ones ground the prompt, and the same list caps the `final` event below. Unlike
	// GET, a stream reader never comes back for a recompute-on-read — this event IS
	// the served response — so the ceiling has to be applied here, not skipped on the
	// assumption a later GET will do it. The cache still holds the uncapped analysis,
	// exactly as the POST path leaves it, so a dictionary change still takes effect
	// on a later GET with no cache invalidation. Needed by both roles: a follower caps
	// the same way before replaying the leader's cached analysis.
	blockers := h.jobBlockers(c.Context(), userID, job, profile)

	// One request value carries everything both roles need. A follower fills no Analyzer and
	// no Input: it runs neither the LLM nor the reads that assemble one, and reaches its own
	// path (followMatchAnalysis) once the body-writer opens.
	req := fitanalysis.Request{
		UserID:       userID,
		Job:          job,
		CVUploadedAt: cvUploadedAt,
		Reserved:     reserved,
		Claim:        claim,
	}
	if claim.IsLeader() {
		// Bound before the stream opens: minting a credential is a network call, and making
		// it after the headers are out would stall a stream the client is already reading.
		req.Analyzer = h.boundAnalyzer(c.Context(), userID)

		// The language is captured up front, same as cvUploadedAt above: the cache write
		// happens after the stream's own goroutine outlives this request, so the language it
		// stamps must be the one the chain actually ran under, not whatever a profile edit
		// changes it to mid-stream.
		req.Input = h.buildAnalysisInput(c, job, userID, profile, blockers, h.callerLanguage(c.Context(), userID))
	}

	sseHeaders(c)

	// The server's 10s WriteTimeout would kill this long-lived stream mid-analysis, so the
	// SSE response sets its own, per-write deadline instead (see sseStream). Captured here
	// while the ctx is valid; used inside the writer, which runs after this handler returns.
	conn := c.Context().Conn()

	// Same reason as conn: the sentryfiber hub is request-scoped and the ctx is released
	// once this handler returns, so take a clone now. A clone rather than the hub itself
	// because the writer outlives the request that owns its scope.
	var hub *sentry.Hub
	if reqHub := sentryfiber.GetHubFromContext(c); reqHub != nil {
		hub = reqHub.Clone()
	}

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		stream := newSSEStream(w, conn, sseWriteTimeout)
		start := time.Now()
		log.Printf("matchanalysis: stream start user=%d job=%d has_cv=%v leader=%v", userID, job.ID, hasCV, claim.IsLeader())
		stream.event("meta", map[string]bool{"has_cv": hasCV})
		if !hasCV {
			return
		}
		// A background context: the fiber request ctx is gone by now, and each LLM call
		// is already bounded by the client's per-call timeout.
		//
		// A client that disconnects therefore does NOT abort the chain, and that is on
		// purpose. The run is nearly always one an AI credit was just spent on, so finishing
		// it puts the analysis in the cache for when the reader comes back, where aborting
		// would charge them for nothing. What bounds the spend is the per-user rate limit on
		// the routes that reach here (matchAnalysisLimiter), not the lifetime of a TCP
		// connection — a limit a reload could reset would bound nothing at all.
		ctx := context.Background()

		if !claim.IsLeader() {
			// Someone else — almost always the cold-start autopilot's own invisible
			// autopilotAnalysis.ensure — already claimed this pair. Wait for it and replay its
			// result rather than running a second chain.
			h.followMatchAnalysis(ctx, stream, req, blockers)
			return
		}

		events := 0
		// A long stage (a silent LLM call with no thinking tokens) would let the
		// connection go quiet long enough for nginx's proxy_read_timeout to sever it
		// mid-analysis — the client sees a bare "Connection lost". A periodic SSE comment
		// keeps bytes flowing so the stream survives silent stages. The ticker goroutine
		// and the stage callback both write, which sseStream serializes.
		stopHeartbeat := stream.keepalive(sseKeepalive)

		// The claim is released inside Run, from a defer that also covers this callback — so a
		// panic anywhere in the chain, the emit, the cache write or the debit still wakes a
		// waiting follower instead of stranding that (user, job) pair behind a leader that
		// never finishes.
		analysis, err := h.fit.Run(ctx, req, func(e matchanalysis.Event) {
			events++
			stream.event(string(e.Kind), capFinalEvent(e, blockers))
		})
		stopHeartbeat()
		if err != nil {
			log.Printf("matchanalysis: stream FAILED user=%d job=%d dur=%s events=%d: %v", userID, job.ID, time.Since(start).Round(time.Millisecond), events, err)
			reportStreamFault(hub, err)
			stream.event("stream_error", map[string]string{"message": "analysis failed"})
			return
		}
		if analysis == nil {
			stream.event("stream_error", map[string]string{"message": "analysis unavailable"})
			return
		}
		log.Printf("matchanalysis: stream DONE user=%d job=%d dur=%s events=%d overall=%d", userID, job.ID, time.Since(start).Round(time.Millisecond), events, analysis.OverallScore)
	}))
	return nil
}

// followMatchAnalysis is the graceful degrade for the rare race a visible stream loses: some
// concurrent caller for the same (user, job) — almost always the cold-start autopilot's own
// invisible autopilotAnalysis.ensure — claimed the compute first. It waits on run.done, then
// replays whatever landed in the cache as one synthesized burst — stage_done for all three
// stages (there is nothing to show progressing through) followed by final — so this reader's
// stepper still resolves instead of hanging on "pending" forever.
//
// Waiting for the leader, deciding whether its result can be trusted, and charging this
// caller's own credit are fitanalysis.Service.Follow's — including why a failed leader must
// report the same failure rather than serve the older row it left behind. What is left here is
// the replay: three stage_done events and a final, because this reader's stepper must resolve
// instead of hanging on "pending" forever. It never emits
// stage_start/thinking/requirements/dimensions — those describe progress this caller never
// watched.
//
// The wait runs on ctx (StreamMatchAnalysis's background context, taken after the fiber ctx is
// gone): the same disconnect-proof posture the leader's own compute already runs under, so a
// client that leaves during the wait costs nothing extra.
func (h *matchHandlers) followMatchAnalysis(ctx context.Context, stream *sseStream, req fitanalysis.Request, blockers []hardconstraint.Blocker) {
	stopHeartbeat := stream.keepalive(sseKeepalive) // the wait can run as long as a full chain
	analysis, err := h.fit.Follow(ctx, req)
	stopHeartbeat()

	if err != nil {
		stream.event("stream_error", map[string]string{"message": "analysis unavailable"})
		return
	}
	served := *analysis
	applyBlockers(&served, blockers)
	for n := 1; n <= 3; n++ {
		stream.event(string(matchanalysis.EventStageDone), matchanalysis.Event{
			Kind: matchanalysis.EventStageDone, Stage: n, Label: matchanalysis.StageLabel(n),
		})
	}
	stream.event(string(matchanalysis.EventFinal), matchanalysis.Event{Kind: matchanalysis.EventFinal, Analysis: &served})
}

// capFinalEvent applies the caller's hard-constraint blockers to the audited `final`
// event's analysis before it goes out over the wire — the same ceiling GetMatchAnalysis
// (capServedAnalysis) and PostMatchAnalysis (applyBlockers) apply, so a stream reader
// never sees an uncapped, blocker-free score just because a GET recompute never runs for
// them. Every other event kind passes through unchanged.
//
// It caps a COPY of the analysis rather than the one e.Analysis points to: that pointer
// is the same object AnalyzeStream returns to the caller, which still feeds
// h.cacheAnalysis right after and must stay uncapped there, exactly as the POST path's
// cache write does — so a later dictionary change still takes effect on a GET with no
// cache invalidation.
func capFinalEvent(e matchanalysis.Event, blockers []hardconstraint.Blocker) matchanalysis.Event {
	if e.Kind != matchanalysis.EventFinal || e.Analysis == nil {
		return e
	}
	served := *e.Analysis
	applyBlockers(&served, blockers)
	e.Analysis = &served
	return e
}

// reportStreamFault sends a fault that surfaced AFTER the response body began streaming
// to Sentry. It exists because RenderError — the single reporting point for the whole API
// — only ever sees errors a handler RETURNS, and this handler returns nil before the body
// writer runs. Without this seam every failed analysis is invisible: the reader gets a
// `stream_error` event, the access log records the 200 the stream opened with, and the
// error inbox stays empty.
//
// hub is a clone captured while the request context was still alive; it is nil when Sentry
// is unconfigured, which must report nothing rather than panic on the SSE goroutine.
//
// What counts as a fault is classify's decision, not a second policy invented here: the
// streaming path must not disagree with the returned-error path about whether a reader
// who walked away is an error. Only classify's status mapping is dropped — the response
// status was fixed when the stream opened, long before this failure existed.
func reportStreamFault(hub *sentry.Hub, err error) {
	if hub == nil {
		return
	}
	if _, _, report := classify(err); !report {
		return
	}
	hub.CaptureException(err)
}

// sseWriteTimeout bounds a single SSE write. It is generous — a live reader never comes
// close, and the deadline is refreshed per write, so a slow-but-alive client is never cut
// off mid-analysis. Its job is only to put a ceiling on a write that would otherwise never
// return, which is what strands the analysis goroutine.
const sseWriteTimeout = 30 * time.Second

// sseStream owns one SSE response body: it serializes the writes (the heartbeat goroutine
// and the analysis callback both write, and bufio.Writer is not safe for concurrent use)
// and bounds each of them.
//
// The bounding is the point. The handler clears the connection's write deadline so the
// server's 10s WriteTimeout cannot kill a long analysis — but a cleared deadline is
// forever, so a reader that stopped reading blocks Flush indefinitely while holding the
// lock, stranding the analysis goroutine for the life of the process. A per-write
// deadline, refreshed on every write, keeps the long stream alive without ever letting a
// single write block without limit.
type sseStream struct {
	mu      sync.Mutex
	w       *bufio.Writer
	conn    net.Conn
	timeout time.Duration
}

func newSSEStream(w *bufio.Writer, conn net.Conn, timeout time.Duration) *sseStream {
	return &sseStream{w: w, conn: conn, timeout: timeout}
}

// event writes one named SSE event with a JSON data payload and flushes it, reporting
// whether it reached the client. A marshal failure reports TRUE: an unencodable frame is
// our bug, not a dead reader, and a caller that stops the work on false must not stop it
// over our own encoding mistake.
func (s *sseStream) event(name string, data any) bool {
	blob, err := json.Marshal(data)
	if err != nil {
		return true
	}
	return s.write(fmt.Sprintf("event: %s\ndata: %s\n\n", name, blob))
}

// comment writes an SSE comment line — ignored by EventSource — as a heartbeat that keeps
// the connection producing bytes through long, silent stages. A failure is nothing to act
// on: the next event will report it.
func (s *sseStream) comment(text string) {
	_ = s.write(fmt.Sprintf(": %s\n\n", text))
}

// sseKeepalive is how often a silent stream emits a comment. Both SSE endpoints go quiet
// for the same reason — a model thinking — and would be severed by the same thing: nginx's
// proxy_read_timeout, which shows the client a bare "connection lost" mid-answer. One
// number because it answers one constraint.
const sseKeepalive = 15 * time.Second

// keepalive starts the heartbeat and returns the function that stops it. Stopping BLOCKS
// until the goroutine has finished, so no comment can be written after the caller believes
// the stream is done — both endpoints hand-rolled this ticker plus a WaitGroup, and getting
// the ordering wrong writes into a closed body.
//
// The interval is an argument rather than the constant so that property can be tested at a
// millisecond instead of the fifteen seconds production waits.
func (s *sseStream) keepalive(every time.Duration) (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				s.comment("keepalive")
			}
		}
	}()
	return func() {
		close(done)
		wg.Wait()
	}
}

// write emits one frame under the lock, with a fresh deadline, and reports whether it
// reached the client. The deadline is what guarantees the call RETURNS, so a reader that
// stopped reading cannot pin this goroutine.
func (s *sseStream) write(frame string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		_ = s.conn.SetWriteDeadline(time.Now().Add(s.timeout))
	}
	if _, err := s.w.WriteString(frame); err != nil {
		return false
	}
	return s.w.Flush() == nil
}

// sseHeaders sets the response headers every SSE endpoint needs, so a new one cannot ship
// with three of the four.
func sseHeaders(c *fiber.Ctx) {
	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Set("X-Accel-Buffering", "no") // stop nginx buffering so events reach the browser promptly
}
