package handler

import (
	"bufio"
	"time"

	"github.com/getsentry/sentry-go"
	sentryfiber "github.com/getsentry/sentry-go/fiber"
	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/search/embedprogress"
)

// embedProgressConnectsPerMinute bounds how often one IP may OPEN the progress stream,
// the same bound and the same reasoning as recentFeedConnectsPerMinute: the route is
// public and unauthenticated, and EventSource reconnects on its own, so a client stuck
// in a reconnect loop is the case to bound, not a reader holding one connection open.
const embedProgressConnectsPerMinute = 20

// embedProgressSource is what the routes read. *embedprogress.Sampler satisfies it.
type embedProgressSource interface {
	Current() embedprogress.Progress
	Subscribe() (embedprogress.Progress, <-chan embedprogress.Progress, func())
}

// embedProgressHandlers serves the public /embeddings page's figures — see
// openspec/changes/embedding-progress-page. Neither route touches the database: the
// JSON answer is the sampler's latest document and the stream is its subscription, so
// a request costs a mutex and nothing the host's database would notice.
type embedProgressHandlers struct {
	source       embedProgressSource
	pingInterval time.Duration
	throttler    ratelimit.Throttler
}

func newEmbedProgressHandlers(source embedProgressSource, throttler ratelimit.Throttler) *embedProgressHandlers {
	return &embedProgressHandlers{source: source, pingInterval: sseKeepalive, throttler: throttler}
}

func (h *embedProgressHandlers) register(api fiber.Router) {
	api.Get("/stats/embeddings", h.Progress)
	api.Get("/stats/embeddings/stream",
		ratelimit.Middleware(h.throttler, ratelimit.KeyByIP("embed-progress"), embedProgressConnectsPerMinute, time.Minute),
		h.Stream)
}

// Progress answers the current progress document. Unmeasured figures are null.
func (h *embedProgressHandlers) Progress(c *fiber.Ctx) error {
	if h.source == nil {
		return fiber.ErrServiceUnavailable
	}
	return c.JSON(fiber.Map{"data": h.source.Current()})
}

// Stream sends the current document at once (once one has been sampled) and then one
// "progress" event per sample,
// with a keepalive ping between. Each event is the whole document, so a client that
// misses one is current again at the next.
func (h *embedProgressHandlers) Stream(c *fiber.Ctx) error {
	if h.source == nil {
		return fiber.ErrServiceUnavailable
	}
	current, updates, cancel := h.source.Subscribe()

	sseHeaders(c)
	conn := c.Context().Conn()
	// fasthttp closes Done when the server shuts down, not when a client leaves.
	serverDone := c.Context().Done()

	var hub *sentry.Hub
	if reqHub := sentryfiber.GetHubFromContext(c); reqHub != nil {
		hub = reqHub.Clone()
	}

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer cancel()
		stream := newSSEStream(w, conn, sseWriteTimeout, hub)
		defer recoverStream(hub, "embedprogress: stream", nil)

		// Before the sampler's first reading every figure is null; sending that would
		// replace what the page already rendered with "measuring…". The first real
		// sample arrives on the channel below.
		if current.MeasuredAt != nil && !stream.event("progress", current) {
			return
		}
		ping := time.NewTicker(h.pingInterval)
		defer ping.Stop()
		for {
			select {
			case <-serverDone:
				return
			case p, ok := <-updates:
				if !ok {
					return
				}
				if !stream.event("progress", p) {
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
