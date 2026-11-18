package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/strelov1/freehire/internal/platform/observability"
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/hardconstraint"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// headerSecFetchSite is the Fetch Metadata request header naming the relationship between
// the origin that started a request and the origin it is sent to. fasthttp defines no
// constant for it (v1.76.0), so the canonical casing is spelled out here.
const headerSecFetchSite = "Sec-Fetch-Site"

// The Sec-Fetch-Site values Fetch Metadata defines.
const (
	secFetchSiteCrossSite  = "cross-site"
	secFetchSiteSameSite   = "same-site"
	secFetchSiteSameOrigin = "same-origin"
	secFetchSiteNone       = "none"
)

// secFetchSiteAllows reports whether a Sec-Fetch-Site value states the request is the
// site's own doing: same-origin, same-site (every origin under the site's control is trusted,
// by policy), or none (the browser's own address bar). cross-site and every unknown or empty
// value refuse: an empty header carries no signal, and an unknown future value is not trusted.
func secFetchSiteAllows(value string) bool {
	switch value {
	case secFetchSiteSameOrigin, secFetchSiteNone, secFetchSiteSameSite:
		return true
	default:
		return false
	}
}

// cookieOnlyStreamAllowed is the gate a cookie-authenticated entry into a route that
// mutates state on GET (a credit reservation, an LLM run) must pass. The session cookie is
// SameSite=Lax, which attaches it to cross-site top-level GET navigations, so any page could
// carry a signed-in victim's credential onto a GET that spends something.
//
// The check is Fetch Metadata: same-origin, same-site and none pass; cross-site, unknown and
// absent values refuse, because absence proves nothing and must not spend a credit. A scripted
// caller uses a Bearer API key, which cannot ride a navigation and is never gated here.
//
// Mount it only behind an auth gate that leaves the credential in locals (RequireAuthOrKey):
// ViaCookie is how the request proved who it was. It runs at the top of the handler, so a
// denied request has paid the limiter and entitlement read but never reaches the job read,
// the reservation or the model chain.
func cookieOnlyStreamAllowed(c *fiber.Ctx) bool {
	if !auth.ViaCookie(c) {
		return true
	}
	return secFetchSiteAllows(c.Get(headerSecFetchSite))
}

// StreamMatchAnalysis runs the three-stage fit chain over Server-Sent Events, emitting stage
// progress, best-effort thinking tokens, and each section as it resolves, then caching the
// final analysis as PostMatchAnalysis does. Cookie or API key; unknown slug 404. The stream
// always opens with a `meta` event (has_cv) and closes after it when no CV is stored.
// Everything the stream needs is captured before the body writer starts, because the fiber
// ctx is released once this handler returns.
//
// Coalesced through fitanalysis.Claim: the leader runs the chain; a follower (typically the
// autopilot's autopilotAnalysis.ensure racing for the same (user, job)) replays the result via
// followMatchAnalysis instead of paying for a second chain.
func (h *matchHandlers) StreamMatchAnalysis(c *fiber.Ctx) error {
	if !cookieOnlyStreamAllowed(c) {
		// A cross-site top-level navigation is not an SSE client (EventSource cannot produce
		// one), so a 403 never reaches a legitimate reader. This saves the job read, the credit
		// reservation, the credential mint and the model chain.
		return fiber.NewError(fiber.StatusForbidden, "cross-site initiation is not allowed")
	}
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	job, err := h.queries.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err
	}
	if err := jobVisibleToCaller(job, userID, true); err != nil {
		return err
	}
	cvUploadedAt, hasCV := h.cvUploadedAt(c.Context(), userID)
	// Reserve the credit before opening the stream, while the fiber ctx is valid, so an
	// out-of-credits new job returns a real 402 instead of an SSE error. Only a CV-backed
	// request runs the LLM. A recompute is free, and a run that produces nothing gives the
	// credit back.
	//
	// Every caller reserves for itself, leader or follower, so an out-of-credits caller still
	// 402s. Reserving twice is safe: the debit is idempotent per (user, feature, job).
	//
	// The analyzer is bound first, so a refusing binding (route mounted without the paid gate)
	// leaves nothing reserved and no leadership claimed. A follower never uses it.
	var analyzer *matchanalysis.Analyzer
	reserved := false
	if hasCV {
		var err error
		if analyzer, err = h.boundAnalyzer(c.UserContext(), userID); err != nil {
			return err
		}
		if reserved, err = h.fit.Reserve(c.Context(), userID, job.ID); err != nil {
			if refusal, refused := renderRefusal(c, err); refused {
				return refusal
			}
			return err
		}
	}

	// Leadership for (user, job) is claimed after the gate above, so a leader never has to
	// hand leadership back. A follower runs neither the LLM nor the reads below; see
	// fitanalysis.Claim and followMatchAnalysis.
	var claim *fitanalysis.Claim
	if hasCV {
		claim = h.fit.Claim(userID, job.ID)
	}
	profile, _ := h.userProfile.Get(c.Context(), userID)

	// Compute the hard-constraint blockers as the POST path does: the unmet ones ground the
	// prompt, and the same list caps the `final` event. A stream reader never comes back for a
	// recompute-on-read, so the ceiling is applied here. The cache keeps the uncapped analysis.
	// Both roles need it: a follower caps the same way before replaying.
	blockers := h.jobBlockers(c.Context(), userID, job, profile)

	// One request value carries everything both roles need. A follower fills no Analyzer and
	// no Input; it reaches followMatchAnalysis once the body-writer opens.
	req := fitanalysis.Request{
		UserID:       userID,
		Job:          job,
		CVUploadedAt: cvUploadedAt,
		Reserved:     reserved,
		Claim:        claim,
	}
	if claim.IsLeader() {
		// Bound above, before the stream opens: minting a credential is a network call that
		// would stall a stream already being read.
		req.Analyzer = analyzer

		// Captured up front: the cache write happens after this request is gone, and must stamp
		// the language the chain ran under, not a mid-stream profile edit.
		req.Input = h.buildAnalysisInput(c.Context(), job, userID, profile, blockers, h.callerLanguage(c.Context(), userID))
	}

	sseHeaders(c)

	// The server's 10s WriteTimeout would kill this stream mid-analysis, so the SSE response
	// sets its own per-write deadline (see sseStream). Captured while the ctx is valid.
	conn := c.Context().Conn()

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		stream := newSSEStream(w, conn, sseWriteTimeout)
		// Registered first so it unwinds last: the claim release and heartbeat stop run before the
		// reader is told the analysis failed.
		defer recoverStream("matchanalysis: stream", func() {
			stream.event("stream_error", map[string]string{"message": "analysis failed"})
		})
		start := time.Now()
		log.Printf("matchanalysis: stream start user=%d job=%d has_cv=%v leader=%v", userID, job.ID, hasCV, claim.IsLeader())
		stream.event("meta", map[string]bool{"has_cv": hasCV})
		if !hasCV {
			return
		}
		// A background context: the fiber request ctx is gone, and each LLM call is bounded by
		// the client's per-call timeout.
		//
		// A client disconnect deliberately does not abort the chain: a credit was just spent, so
		// finishing caches the analysis for when the reader returns.
		ctx := context.Background()

		if !claim.IsLeader() {
			// Someone else (usually the autopilot's ensure) already claimed this pair: wait for it
			// and replay its result instead of running a second chain.
			h.followMatchAnalysis(ctx, stream, req, blockers)
			return
		}

		events := 0
		// A silent LLM stage would let nginx's proxy_read_timeout sever the connection, so a
		// periodic SSE comment keeps bytes flowing. The ticker and the stage callback both write;
		// sseStream serializes them.
		stopHeartbeat := stream.keepalive(sseKeepalive)
		// Deferred, not stopped when Run returns: a panic in the chain would otherwise leave the
		// ticker writing into a bufio.Writer fasthttp already recycled onto another connection.
		defer stopHeartbeat()

		// The claim is released inside Run from a defer that also covers this callback, so a panic
		// anywhere still wakes a waiting follower.
		analysis, err := h.fit.Run(ctx, req, func(e matchanalysis.Event) {
			events++
			stream.event(string(e.Kind), capFinalEvent(e, blockers))
		})
		if err != nil {
			log.Printf("matchanalysis: stream FAILED user=%d job=%d dur=%s events=%d: %v", userID, job.ID, time.Since(start).Round(time.Millisecond), events, err)
			reportStreamFault(err)
			stream.event("stream_error", map[string]string{"message": "analysis failed"})
			return
		}
		if analysis == nil {
			// Two situations produce the same silence, and only one is the candidate's to act on: a
			// deployment with no gateway, versus a CV whose background parse failed and left no banked
			// experience, which only a re-upload fixes. The stream already said has_cv=true, so the
			// message must not read as a fault on our side.
			msg := "analysis unavailable"
			reason := "no analysis produced"
			if !matchanalysis.HasCandidateContext(req.Input.StructuredResume) {
				msg = "We couldn't read your CV, so there's nothing to compare this job against. Try uploading it again."
				reason = "no banked experience — the CV's background parse produced none"
			}
			log.Printf("matchanalysis: stream EMPTY user=%d job=%d dur=%s events=%d: %s", userID, job.ID, time.Since(start).Round(time.Millisecond), events, reason)
			stream.event("stream_error", map[string]string{"message": msg})
			return
		}
		log.Printf("matchanalysis: stream DONE user=%d job=%d dur=%s events=%d overall=%d", userID, job.ID, time.Since(start).Round(time.Millisecond), events, analysis.OverallScore)
	}))
	return nil
}

// followMatchAnalysis is the graceful degrade for the race a visible stream loses: a
// concurrent caller for the same (user, job), usually the autopilot's ensure, claimed the
// compute first. It waits on run.done, then replays whatever landed in the cache as
// stage_done for all three stages followed by final, so this reader's stepper resolves.
// It never emits stage_start/thinking/requirements/dimensions: this caller watched no progress.
//
// Waiting for the leader, trusting its result, and charging this caller's own credit are
// fitanalysis.Service.Follow's, including why a failed leader must report the same failure
// rather than serve the older row it left behind.
//
// The wait runs on ctx (StreamMatchAnalysis's background context), so a client that leaves
// during the wait costs nothing extra.
func (h *matchHandlers) followMatchAnalysis(ctx context.Context, stream *sseStream, req fitanalysis.Request, blockers []hardconstraint.Blocker) {
	stopHeartbeat := stream.keepalive(sseKeepalive) // the wait can run as long as a full chain
	// Deferred, not stopped when Follow returns: a panic below would otherwise leave the
	// ticker writing into a recycled bufio.Writer. A comment between replay events is harmless.
	defer stopHeartbeat()

	analysis, err := h.fit.Follow(ctx, req)
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

// capFinalEvent applies the caller's hard-constraint blockers to the `final` event's
// analysis before it goes out, the same ceiling GetMatchAnalysis (capServedAnalysis) and
// PostMatchAnalysis (applyBlockers) apply. Other event kinds pass through unchanged.
//
// It caps a copy: e.Analysis points to the object AnalyzeStream returns, which still feeds
// h.cacheAnalysis and must stay uncapped, so a later dictionary change takes effect on GET.
func capFinalEvent(e matchanalysis.Event, blockers []hardconstraint.Blocker) matchanalysis.Event {
	if e.Kind != matchanalysis.EventFinal || e.Analysis == nil {
		return e
	}
	served := *e.Analysis
	applyBlockers(&served, blockers)
	e.Analysis = &served
	return e
}

// reportStreamFault sends a fault that surfaced after the response body began streaming to
// the exception log. RenderError only sees errors a handler returns, and this handler returns
// nil before the body writer runs, so without this seam failed analyses are invisible.
//
// What counts as a fault is classify's decision, so the streaming and returned-error paths
// agree. Only its status mapping is dropped: the status was fixed when the stream opened.
func reportStreamFault(err error) {
	if _, _, report := classify(err); !report {
		return
	}
	observability.CaptureError(context.Background(), err)
}

// recoverStream catches a panic on a goroutine this package hands to fasthttp, reports it,
// and lets the stream close itself through terminal.
//
// None of those goroutines is covered by the server's recover middleware:
// SetBodyStreamWriter runs the closure in a bare `go sw(bw)` after the handler chain has
// returned, so a panic there would kill the process. A test pins the same promise for
// ordinary handlers.
//
// terminal is the one frame this goroutine still owes its client, or nil when it owes none
// (the heartbeat). It runs last, after the defers that give a charge back or release a
// claim, because those make the terminal frame true.
func recoverStream(what string, terminal func()) {
	r := recover()
	if r == nil {
		return
	}
	observability.CapturePanic(context.Background(), r)
	log.Printf("%s: panic: %v\n%s", what, r, debug.Stack())
	if terminal != nil {
		terminal()
	}
}

// sseWriteTimeout bounds a single SSE write. It is generous and refreshed per write, so a
// slow-but-alive client is never cut off; it only caps a write that would never return.
const sseWriteTimeout = 30 * time.Second

// sseStream owns one SSE response body: it serializes the writes (the heartbeat and the
// analysis callback both write, and bufio.Writer is not concurrency-safe) and bounds each.
//
// The handler clears the connection's write deadline so the server's 10s WriteTimeout cannot
// kill a long analysis, but a cleared deadline is forever: a reader that stopped reading would
// block Flush while holding the lock. A per-write deadline avoids both.
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
// whether it reached the client. A marshal failure reports true: an unencodable frame is our
// bug, not a dead reader, and must not stop the work.
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

// sseKeepalive is how often a silent stream emits a comment, so nginx's proxy_read_timeout
// does not sever it while a model thinks. Shared by both SSE endpoints.
const sseKeepalive = 15 * time.Second

// keepalive starts the heartbeat and returns the function that stops it. Stopping blocks
// until the goroutine has finished, so no comment is written after the caller believes the
// stream is done. The interval is an argument so tests can run it at a millisecond.
func (s *sseStream) keepalive(every time.Duration) (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Its own goroutine, so its own recover; it owes no terminal frame, the writer that
		// started it closes the stream.
		defer recoverStream("sse keepalive", nil)
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

// write emits one frame under the lock with a fresh deadline and reports whether it
// reached the client; the deadline guarantees the call returns.
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
