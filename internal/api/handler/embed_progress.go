package handler

import (
	"bufio"
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/search/embedprogress"
)

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
}

func newEmbedProgressHandlers(source embedProgressSource) *embedProgressHandlers {
	return &embedProgressHandlers{source: source, pingInterval: sseKeepalive}
}

func (h *embedProgressHandlers) register(api fiber.Router, contract fiber.Handler) {
	api.Get("/stats/embeddings", contract)
	api.Get("/stats/embeddings/stream",
		h.Stream)
}

// GetEmbeddingProgress answers the current progress document. Unmeasured figures are null.
func (h *embedProgressHandlers) GetEmbeddingProgress(context.Context) (*apigen.GetEmbeddingProgressOK, error) {
	if h.source == nil {
		return nil, fiber.ErrServiceUnavailable
	}
	p := h.source.Current()
	out := apigen.EmbeddingProgress{
		Eligible: nilInt64(p.Eligible), Covered: nilInt64(p.Covered), SnapshotCovered: nilInt64(p.SnapshotCovered),
		Queued: nilInt64(p.Queued), EtaSeconds: nilInt64(p.ETASeconds),
		SnapshotAt: nilDateTime(p.SnapshotAt), MeasuredAt: nilDateTime(p.MeasuredAt),
		RatePerMinute: apigen.NilFloat64{Null: true},
	}
	if p.RatePerMinute != nil {
		out.RatePerMinute = apigen.NewNilFloat64(*p.RatePerMinute)
	}
	return &apigen.GetEmbeddingProgressOK{Data: out}, nil
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

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer cancel()
		stream := newSSEStream(w, conn, sseWriteTimeout)
		defer recoverStream("embedprogress: stream", nil)

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
