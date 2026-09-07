package handler

import (
	"bufio"
	"time"

	"github.com/getsentry/sentry-go"
	sentryfiber "github.com/getsentry/sentry-go/fiber"
	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/job/recentfeed"
)

// recentFeedConnectsPerMinute bounds how often one IP may OPEN a new connection to
// the feed. It does not cap total concurrently-open connections from an IP that
// opens them slowly — this endpoint has no session/credential to key a stricter
// limit on — but it blocks the cheap abuse shape (a reconnect loop or a burst of
// parallel opens) the same way every other IP-scoped limiter in this package does.
const recentFeedConnectsPerMinute = 20

// recentFeedLimiter bounds new connections to StreamRecentJobs — see
// recentFeedConnectsPerMinute. This is the first public, unauthenticated,
// indefinitely-held connection endpoint in the API: unlike a plain request it
// costs a live goroutine, ticker, and Broadcaster subscription for as long as it
// stays open.
func recentFeedLimiter(throttler ratelimit.Throttler) fiber.Handler {
	return ratelimit.Middleware(throttler, ratelimit.KeyByIP("recent-feed"), recentFeedConnectsPerMinute, time.Minute)
}

// recentFeedHandlers serves the homepage's live "recently added jobs" feed —
// a cosmetic, unauthenticated signal that the catalogue is actively growing, fed
// by the internal/job/recentfeed Poller running elsewhere in this same process.
// See openspec/changes/add-homepage-recent-jobs-feed.
type recentFeedHandlers struct {
	// broadcaster is nil when the deployment has not wired one up (e.g. a fixture
	// exercising unrelated routes). The stream degrades to 503 rather than serving
	// a feed that will never receive anything, matching how Config's other
	// optional dependencies (Suggest, LLM, Blob) degrade elsewhere in this package.
	broadcaster *recentfeed.Broadcaster
	// pingInterval is how often an idle connection gets a "ping" event — both the
	// production cadence and how a dead connection is noticed (see
	// StreamRecentJobs). A field rather than the sseKeepalive constant directly so
	// a test can shrink it, the same reason sseStream.keepalive takes its interval
	// as an argument instead of using the constant inline.
	pingInterval time.Duration
	// throttler backs the per-IP connect-rate limit on this route. Nil in a
	// fixture that never exercises it; register still works (Middleware handles a
	// nil throttler no differently than any other IP-limited route in this file).
	throttler ratelimit.Throttler
}

func newRecentFeedHandlers(broadcaster *recentfeed.Broadcaster, throttler ratelimit.Throttler) *recentFeedHandlers {
	return &recentFeedHandlers{broadcaster: broadcaster, pingInterval: sseKeepalive, throttler: throttler}
}

func (h *recentFeedHandlers) register(api fiber.Router) {
	// Public, unauthenticated: the homepage is shown to signed-out visitors, and the
	// feed carries nothing user-specific — the same posture as the other public
	// reads statsHandlers serves.
	api.Get("/feed/recent", recentFeedLimiter(h.throttler), h.StreamRecentJobs)
}

// StreamRecentJobs streams the homepage's live feed over Server-Sent Events. On
// connect it replays the Broadcaster's current backlog so a new connection never
// starts on an empty feed, then streams every entry the Poller publishes for as
// long as the client stays connected.
//
// Unlike the bounded chains match_analysis_stream.go and assistant.go stream (which
// run to completion regardless of whether the reader is still there), this
// subscription is indefinite: it must notice a dead connection itself instead of
// relying on the underlying work finishing. A periodic "ping" event doubles as
// that check — its write failing is how a client that walked away is noticed and
// its Broadcaster subscription released, exactly as a real "job" event's failed
// write would be.
func (h *recentFeedHandlers) StreamRecentJobs(c *fiber.Ctx) error {
	if h.broadcaster == nil {
		return fiber.ErrServiceUnavailable
	}
	backlog, entries, cancel := h.broadcaster.Subscribe()

	sseHeaders(c)
	conn := c.Context().Conn()

	var hub *sentry.Hub
	if reqHub := sentryfiber.GetHubFromContext(c); reqHub != nil {
		hub = reqHub.Clone()
	}

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer cancel()
		stream := newSSEStream(w, conn, sseWriteTimeout, hub)
		defer recoverStream(hub, "recentfeed: stream", nil)

		for _, e := range backlog {
			if !stream.event("job", e) {
				return
			}
		}

		ping := time.NewTicker(h.pingInterval)
		defer ping.Stop()
		for {
			select {
			case e, ok := <-entries:
				if !ok {
					return
				}
				if !stream.event("job", e) {
					return
				}
			case <-ping.C:
				if !stream.event("ping", struct{}{}) {
					return
				}
			}
		}
	}))
	return nil
}
