package handler

import (
	"bufio"
	"context"
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/strelov1/freehire/internal/candidate/coverletter"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// letterStreamTimeout bounds the whole chain once the request context is gone. Three model
// calls in series, each bounded at the client's own per-call timeout, need room for the worst
// case without letting a wedged gateway hold a goroutine forever.
const letterStreamTimeout = 6 * time.Minute

// StreamCVCoverLetter drafts the letter over Server-Sent Events.
//
// This exists because the POST cannot work. The chain is three model calls in series and takes
// minutes; a proxy holding a silent response gives up at sixty seconds, so every draft reached
// the candidate as a 504 from the edge while the server was still working. The fit chain
// streams for exactly this reason, and a letter is the same shape of work.
//
// Events: `stage` as each step opens and closes, then either `letter` with the finished draft
// and its resolved evidence, or `error` with a sentence to render. The stream always closes
// with one or the other, so a reader never has to guess.
func (h *cvHandlers) StreamCVCoverLetter(c *fiber.Ctx) error {
	drafter := h.letterDrafter()
	if !drafter.ready() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "cover letters are not enabled on this deployment")
	}
	userID, jobID, err := h.coverLetterTarget(c)
	if err != nil {
		return err
	}

	// Charged BEFORE the stream opens, while the fiber ctx is still valid — a refused caller
	// gets a real 402 rather than an error event inside a 200 nobody status-checks.
	attempt := letterAttempt(c.Context(), h.letter.letters, userID, jobID)
	charge, refused, decision := chargeLetter(c.Context(), h.plans, userID, jobID, attempt)
	if refused {
		return refuse(c, decision)
	}

	// Everything the writer needs is captured here: the fiber ctx is released the moment this
	// handler returns, so reading it inside the stream would race a recycled request.
	band := coverLetterBand(c)
	client := h.llm.bind(c.Context(), userID, llm.Feature(tagCoverLetter))
	conn := c.Context().Conn()

	sseHeaders(c)
	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		stream := newSSEStream(w, conn, sseWriteTimeout)

		// A background context on purpose, the same rule the fit stream states: the request
		// ctx is gone by now, and a client that navigates away must not abort a chain their
		// allowance was just charged for. Finishing puts the letter in the store for when
		// they come back; aborting would charge them for nothing.
		ctx, cancel := context.WithTimeout(context.Background(), letterStreamTimeout)
		defer cancel()

		// The first event goes out before any model call, which is the whole point: it is what
		// keeps the proxy from closing a response that has said nothing for a minute.
		stream.event("stage", letterStageEvent{Stage: coverletter.StageSelect})

		letter, err := drafter.draftStream(ctx, client, userID, jobID, band, func(stage string, done bool) {
			stream.event("stage", letterStageEvent{Stage: stage, Done: done})
		})
		switch {
		case errors.Is(err, coverletter.ErrNoPublishableEvidence):
			releaseLetterCharge(h.plans, userID, charge)
			stream.event("error", letterErrorEvent{
				Error: "nothing in your experience bank is yours to cite yet: confirm an achievement first",
			})
		case err != nil:
			releaseLetterCharge(h.plans, userID, charge)
			log.Printf("coverletter: streaming for user %d job %d: %v", userID, jobID, err)
			// The message is the mapper's, so a streamed failure reads the same as the one the
			// POST path renders — an un-analysed vacancy says "run the fit analysis first"
			// here too, rather than becoming a bare "something went wrong".
			_, msg, _ := classify(err)
			stream.event("error", letterErrorEvent{Error: msg})
		case letter == nil:
			releaseLetterCharge(h.plans, userID, charge)
			stream.event("error", letterErrorEvent{Error: "drafting is unavailable on this deployment"})
		default:
			stream.event("letter", coverLetterResponse{
				Present: true,
				Letter:  letter,
				Cited:   citedAtomsOf(ctx, drafter.bank, userID, letter.Cited),
				Model:   modelIDOf(client),
			})
		}
	}))
	return nil
}

// letterStageEvent reports one step of the chain opening or closing.
type letterStageEvent struct {
	Stage string `json:"stage"`
	Done  bool   `json:"done"`
}

// letterErrorEvent carries a sentence the surface renders. It rides inside a 200 because the
// status was written before the first model call — which is why the stream must always close
// with either this or a letter, and never with silence.
type letterErrorEvent struct {
	Error string `json:"error"`
}
