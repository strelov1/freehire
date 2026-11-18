package handler

import (
	"bufio"
	"context"
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/valyala/fasthttp"
)

// streamTurn runs one turn and writes it to the response as SSE. Every entry point that starts
// a turn shares it, so the stream's shape (headers, cleared write deadline, keepalive) is
// written once.
//
// The prompt and the turn's bounds are arguments, not the request's body: a ceiling a client
// can set is not a bound.
func (h *assistantHandlers) streamTurn(c *fiber.Ctx, sess assistant.Session, prompt string, docs []assistant.Attachment, turn assistant.TurnConfig) error {
	return h.streamSSE(c, sess, h.meterTurn, func(ctx context.Context, runner *assistant.Runner, reg *assistant.Registry, system string, emit func(assistant.Event)) error {
		return runner.RunWithAttachments(ctx, sess, reg, system, prompt, docs, turn, emit)
	})
}

// streamContinue is streamTurn for a retry: same SSE machinery, no new prompt.
func (h *assistantHandlers) streamContinue(c *fiber.Ctx, sess assistant.Session, turn assistant.TurnConfig) error {
	return h.streamSSE(c, sess, h.meterRetry, func(ctx context.Context, runner *assistant.Runner, reg *assistant.Registry, system string, emit func(assistant.Event)) error {
		if sess.SearchContext != nil {
			transcript, err := h.store.Transcript(ctx, sess.ID)
			if err != nil {
				emit(assistant.Event{Kind: assistant.EventResult, IsError: true, StopReason: assistant.StopError})
				return err
			}
			shown, err := shortlistShown(transcript)
			if err != nil {
				emit(assistant.Event{Kind: assistant.EventResult, IsError: true, StopReason: assistant.StopError})
				return err
			}
			reg = assistant.NewRegistry(h.shortlistDiscoveryTools(sess, shown)...)
			reg.MaxResultBytes = assistantResultCap
		}
		return runner.Continue(ctx, sess, reg, system, turn, emit)
	})
}

// streamSSE owns the SSE headers, turn claim/queue, keepalive and writer. start is the
// one difference between a fresh message and a retry, and meter is the difference between
// paying for a new turn and resuming one already paid for.
func (h *assistantHandlers) streamSSE(
	c *fiber.Ctx,
	sess assistant.Session,
	meter func(*fiber.Ctx, assistant.Session) (turnCharge, bool, error),
	start func(context.Context, *assistant.Runner, *assistant.Registry, string, func(assistant.Event)) error,
) error {
	asExtension := auth.IsExtensionBearer(c)
	// Read here, while the request is ours: the turn runs on after the handler returns.
	inBrowser := auth.ViaCookie(c)
	turnSess := sess
	turnSess.Preset = effectivePreset(sess.Preset, asExtension)

	// One batch per turn: every CV edit the agent makes is filed under it, so history can group
	// them and "undo the run" undoes a batch.
	registry := h.registry(turnSess, uuid.New())
	language := h.userLanguage(c.Context(), sess.UserID)
	system := assistant.SystemPrompt(turnSess.Preset, language)
	if turnSess.SearchContext != nil {
		run := start
		// Called after acquiring the session's turn slot: a preceding turn may have saved new
		// preferences while this request was queued.
		start = func(ctx context.Context, runner *assistant.Runner, reg *assistant.Registry, _ string, emit func(assistant.Event)) error {
			fresh, err := h.store.Session(ctx, sess.ID, sess.UserID)
			if err != nil {
				emit(assistant.Event{Kind: assistant.EventResult, IsError: true, StopReason: assistant.StopError})
				return err
			}
			if fresh.SearchContext == nil {
				emit(assistant.Event{Kind: assistant.EventResult, IsError: true, StopReason: assistant.StopError})
				return errors.New("search context is no longer available")
			}
			return run(ctx, runner, reg, assistant.ShortlistPrompt(language, *fresh.SearchContext), emit)
		}
	}

	// Tagged from turnSess, not sess: a demoted browse turn spends like a chat turn, so billing it
	// "browse" would pollute that preset's cost bucket.
	turnRunner, err := h.boundRunner(c.UserContext(), turnSess)
	if err != nil {
		return err
	}

	// Metered BEFORE the headers go out: a refusal has to be a real status, since an error frame
	// inside a stream that already answered 200 is invisible to status checks. Charged against
	// turnSess, not sess, for the same reason as the spend tag.
	//
	// A session that cannot take another message is refused before it is metered, so a busy
	// conversation is not reported as an exhausted allowance. claim below remains the guard.
	if h.turns.full(sess.ID) {
		return fiber.NewError(fiber.StatusConflict, errTurnQueueFull.Error())
	}
	charge, refused, err := meter(c, turnSess)
	if refused {
		return err
	}

	sseHeaders(c)

	// The server's write timeout would kill this long-lived stream, so the SSE response carries its
	// own bounded deadline (captured while the request ctx is valid; used inside the writer).
	conn := c.Context().Conn()

	// The turn runs on its own cancellable context: the request's is released when this handler
	// returns. Created HERE, not in the writer, so the session's slot is taken while the request
	// can still be refused.
	//
	// Detached via WithoutCancel (not context.Background) so the paid-AI gate's mark travels into
	// the turn: a tool making its own model call (cover-letter draft) hits the same fail-closed check.
	ctx, cancel := context.WithCancel(context.WithoutCancel(c.UserContext()))
	slot, waiter, err := h.turns.claim(sess.ID, cancel)
	if err != nil {
		cancel()
		// The charge was taken before the headers (the only place a refusal can still be a status),
		// but this turn never reaches the runner, so it owes the allowance back.
		h.releaseTurn(turnSess, charge)
		return fiber.NewError(fiber.StatusConflict, err.Error())
	}

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		// sseStream owns the write protocol both SSE endpoints need: it serializes the heartbeat
		// goroutine against the event callback (bufio.Writer is not safe for concurrent use) and
		// re-arms the write deadline before EVERY write. Per-write because fasthttp runs this writer
		// on its own goroutine while the serving goroutine arms the server's WriteTimeout, so a single
		// set races and the turn dies at ten seconds. Bounded, not cleared: a reader that stopped
		// reading would block this goroutine forever.
		stream := newSSEStream(w, conn, sseWriteTimeout)

		// Registered first so it unwinds last: the slot, allowance and heartbeat are released by the
		// defers below, and the client must not be told the turn is over while the session holds it.
		//
		// The frame says StopError, as Runner.fail emits: a panic is a failure, not a cancellation, and
		// the client's error branch offers the retry route (POST /assistant/sessions/:id/retry).
		defer recoverStream("assistant: turn", func() {
			stream.event(string(assistant.EventResult),
				assistant.Event{Kind: assistant.EventResult, StopReason: assistant.StopError, IsError: true})
		})

		defer cancel()

		// One byte immediately, before anything this turn might wait on: until something is written,
		// fasthttp buffers the response line and headers, and a proxy times out an upstream that has
		// sent nothing. The heartbeat cannot cover this (its first comment is an interval away). A
		// comment, not an event, because EventSource ignores it.
		stream.comment("open")

		// The heartbeat starts BEFORE the queue wait: a queued message can wait up to a minute, and a
		// proxy collects an idle connection.
		stopHeartbeat := stream.keepalive(sseKeepalive)
		defer stopHeartbeat()

		// A message that arrived while the session was busy waits here rather than running beside the
		// turn in flight (two turns of one tailoring session would edit one CV from two conversations).
		// The client is told it is waiting, since a minute of silence looks like a broken stream.
		if waiter != nil {
			stream.event(string(assistant.EventQueued), assistant.Event{Kind: assistant.EventQueued})

			waitCtx, giveUp := context.WithTimeout(ctx, turnQueueWait)
			slot, err = waiter.enter(waitCtx, cancel)
			giveUp()
			if err != nil {
				// The wait is over and this turn never started, so it gives back what it took.
				h.releaseTurn(turnSess, charge)

				// Every stream owes its client one terminal event, or it waits for a turn that is not coming.
				stream.event(string(assistant.EventResult),
					assistant.Event{Kind: assistant.EventResult, StopReason: assistant.StopCancelled})
				return
			}
		}
		defer h.turns.release(sess.ID, slot)

		turnCtx := ctx
		if inBrowser {
			turnCtx = withPersonInBrowser(ctx)
		}
		err = start(turnCtx, turnRunner, registry, system, func(e assistant.Event) {
			// A failed write means THIS reader is not listening (frozen tab, dropped tunnel), not that
			// the work should stop: stopping threw away live runs. The event is not delivered, the turn
			// runs to its own end under its bounds (step cap, model timeout), the transcript is persisted
			// either way, and a returning client reads it from the session. Stopping a turn is something a
			// caller ASKS for, through the cancel route.
			stream.event(string(e.Kind), e)
		})
		if err != nil {
			// The turn produced nothing usable, so it gives back what it took: a model or transport
			// failure is ours, and charging for an answer that never arrived is what the reservation avoids.
			h.releaseTurn(turnSess, charge)

			// Nothing to continue is a client mistake (empty session), not a turn fault: surface it as a
			// conflict on the open stream rather than error-report noise.
			if errors.Is(err, assistant.ErrNothingToContinue) {
				return
			}
			// The loop already emitted its terminal error event; this is for the exception log, which
			// would never learn of it: this handler returned nil long ago, so RenderError never sees the
			// failure and the access log keeps the 200.
			log.Printf("assistant: turn failed session=%s: %v", sess.ID, err)
			reportStreamFault(err)
		}
	}))
	return nil
}
