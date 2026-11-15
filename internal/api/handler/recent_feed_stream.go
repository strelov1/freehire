package handler

import (
	"bufio"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/job/recentfeed"
)

// recentFeedHandlers serves the homepage's live "recently added jobs" feed, fed by the
// internal/job/recentfeed Poller running in this process.
type recentFeedHandlers struct {
	// broadcaster is nil when not wired (e.g. a fixture); the stream then answers 503.
	broadcaster *recentfeed.Broadcaster
	// pingInterval is how often an idle connection gets a "ping" event; a field so a test can shrink it.
	pingInterval time.Duration
}

func newRecentFeedHandlers(broadcaster *recentfeed.Broadcaster) *recentFeedHandlers {
	return &recentFeedHandlers{broadcaster: broadcaster, pingInterval: sseKeepalive}
}

func (h *recentFeedHandlers) register(api fiber.Router) {
	// Public: nothing in the feed is user-specific.
	api.Get("/feed/recent", h.StreamRecentJobs)
}

// StreamRecentJobs streams the homepage's live feed over Server-Sent Events: the
// Broadcaster's backlog first, then every entry the Poller publishes.
//
// The subscription is indefinite, so it must notice a dead client itself: a periodic "ping"
// event's failed write releases the Broadcaster subscription.
func (h *recentFeedHandlers) StreamRecentJobs(c *fiber.Ctx) error {
	if h.broadcaster == nil {
		return fiber.ErrServiceUnavailable
	}
	backlog, entries, cancel := h.broadcaster.Subscribe()

	sseHeaders(c)
	conn := c.Context().Conn()
	// fasthttp closes Done when the server shuts down, not when a client leaves.
	serverDone := c.Context().Done()

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer cancel()
		stream := newSSEStream(w, conn, sseWriteTimeout)
		defer recoverStream("recentfeed: stream", nil)

		for _, e := range backlog {
			if !stream.event("job", e) {
				return
			}
		}

		ping := time.NewTicker(h.pingInterval)
		defer ping.Stop()
		for {
			select {
			case <-serverDone:
				return
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
