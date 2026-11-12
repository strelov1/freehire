package handler

import (
	"context"
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

const (
	// supportMaxSteps is the support agent's tool-calling ceiling per turn. At the ceiling
	// the runner asks once more with no tools offered, and the prompt tells the model to
	// suggest a human then.
	supportMaxSteps = 3
	// supportMaxPrompt bounds one support message; a support question is a few sentences.
	supportMaxPrompt = 1500
	supportEmail     = "hello@freehire.me"
)

// supportDeps is what the support agent runs on, held apart from the paid assistant's model
// so the two are tagged, timed and billed separately. The zero value is a deployment with
// no support model, which answers 503.
type supportDeps struct {
	llm    *llm.Client
	runner *assistant.Runner
	mail   supportMail

	// prices reads what each plan costs now, from the same provider cache as the pricing
	// page. Without billing (or nil, in tests) my_account carries no prices.
	prices func(context.Context) []billing.PublicPrice

	// pool opens the transaction that makes a blank session's get-or-create atomic.
	pool *pgxpool.Pool
}

// registerSupport mounts the support agent. It is open to every signed-in account, paid or
// not, so none of these routes sits behind the paid-AI gate; what bounds it instead is the
// flat allowance, the per-session ceiling and the day's global budget (meterSupport).
// Cookie only: an API key is a programmatic caller, and a free surface must not be one.
func (h *assistantHandlers) registerSupport(api fiber.Router, mw middleware) {
	api.Post("/support/sessions", mw.cookie, h.CreateSupportSession)
	api.Post("/support/sessions/:id/messages", mw.cookie, h.PostSupportMessage)
	api.Post("/support/sessions/:id/cancel", mw.cookie, h.CancelSupportTurn)
	api.Post("/support/sessions/:id/rating", mw.cookie, h.RateSupportSession)
}

func (h *assistantHandlers) CreateSupportSession(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	// An owner cannot delete a support session, and this route is not metered, so it hands
	// back the blank session it already made rather than adding one per call. That bounds
	// the rows by the messages actually sent, which the daily allowance already bounds. The
	// read and the insert share one transaction under a per-user lock: two parallel calls
	// would otherwise both find none and both insert.
	tx, err := h.support.pool.Begin(c.Context())
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(c.Context()) }()
	q := h.queries.WithTx(tx)
	if err := q.LockAssistantSessionCreation(c.Context(), userID); err != nil {
		return err
	}
	store := assistant.NewStore(q)
	if sess, ok, err := store.EmptySession(c.Context(), userID, assistant.PresetSupport); err != nil {
		return err
	} else if ok {
		return c.JSON(fiber.Map{"data": sessionView(sess)})
	}
	sess, err := store.CreateSession(c.Context(), userID, assistant.PresetSupport, nil, nil)
	if err != nil {
		return err
	}
	if err := tx.Commit(c.Context()); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": sessionView(sess)})
}

// ownedSupportSession is ownedSession's mirror: only a support session answers here, so a
// chat id cannot be driven through the free allowance.
func (h *assistantHandlers) ownedSupportSession(c *fiber.Ctx) (assistant.Session, error) {
	userID, err := requireUserID(c)
	if err != nil {
		return assistant.Session{}, err
	}
	id, err := assistantSessionID(c)
	if err != nil {
		return assistant.Session{}, err
	}
	sess, err := h.store.Session(c.Context(), id, userID)
	if err != nil {
		return assistant.Session{}, mapAssistantError(err)
	}
	if sess.Preset != assistant.PresetSupport {
		return assistant.Session{}, errSessionNotFound
	}
	return sess, nil
}

func (h *assistantHandlers) PostSupportMessage(c *fiber.Ctx) error {
	sess, err := h.ownedSupportSession(c)
	if err != nil {
		return err
	}
	if h.support.runner == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "support is not available")
	}
	var in assistantTurnRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if len(in.Attachments) > 0 {
		return fiber.NewError(fiber.StatusBadRequest, "attachments are not accepted here")
	}
	prompt, _, err := parseTurnRequest(in, supportMaxPrompt)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	c.SetUserContext(plan.WithFreeAI(c.UserContext()))
	return h.streamSSE(c, sess, h.meterSupport, func(ctx context.Context, runner *assistant.Runner, reg *assistant.Registry, system string, emit func(assistant.Event)) error {
		return runner.Run(ctx, sess, reg, system, prompt, turnBounds(sess, ""), emit)
	})
}

func (h *assistantHandlers) CancelSupportTurn(c *fiber.Ctx) error {
	sess, err := h.ownedSupportSession(c)
	if err != nil {
		return err
	}
	h.turns.cancel(sess.ID)
	return c.SendStatus(fiber.StatusNoContent)
}

// RateSupportSession records whether the conversation helped. A pointer, so a body without
// the field is refused rather than read as "did not help".
func (h *assistantHandlers) RateSupportSession(c *fiber.Ctx) error {
	sess, err := h.ownedSupportSession(c)
	if err != nil {
		return err
	}
	var in struct {
		Helpful *bool `json:"helpful"`
	}
	if err := c.BodyParser(&in); err != nil || in.Helpful == nil {
		return fiber.NewError(fiber.StatusBadRequest, "helpful must be true or false")
	}
	if _, err := h.queries.RateSupportSession(c.Context(), db.RateSupportSessionParams{
		ID: sess.ID, UserID: sess.UserID, Helpful: pgtype.Bool{Bool: *in.Helpful, Valid: true},
	}); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *assistantHandlers) boundSupportRunner(ctx context.Context, sess assistant.Session) (*assistant.Runner, error) {
	bound, err := llmBinding{client: h.support.llm, keys: h.keys}.bind(ctx, sess.UserID, llm.Feature(tagSupport))
	if err != nil {
		return nil, err
	}
	if bound == nil {
		return h.support.runner, nil
	}
	return h.support.runner.With(bound), nil
}

// meterSupport decides whether a support message may run and charges it, before the stream
// opens. Unlike the paid assistant it fails CLOSED: this surface is free to every account, so
// a counter that cannot be read must not become unbounded model spend.
//
// Its refusals deliberately carry no upgrade link: a person asking support about a payment
// problem must never be answered with a sales prompt.
func (h *assistantHandlers) meterSupport(c *fiber.Ctx, sess assistant.Session) (turnCharge, bool, error) {
	if h.plans == nil {
		return turnCharge{}, true, fiber.NewError(fiber.StatusServiceUnavailable, "support is not available")
	}
	turns, err := h.queries.CountAssistantUserTurns(c.Context(), sess.ID)
	if err != nil {
		log.Printf("support: counting turns for session %s: %v", sess.ID, err)
		return turnCharge{}, true, fiber.NewError(fiber.StatusServiceUnavailable, "support is not available")
	}
	if int(turns) >= h.plans.SupportTurnsPerSession() {
		return turnCharge{}, true, refuseSupport(c, fiber.StatusTooManyRequests,
			"This conversation has reached its length limit. Please start a new one or email "+supportEmail+".")
	}
	exhausted, err := h.plans.SupportGloballyExhausted(c.Context())
	if err != nil {
		log.Printf("support: reading the day's budget: %v", err)
		return turnCharge{}, true, fiber.NewError(fiber.StatusServiceUnavailable, "support is not available")
	}
	if exhausted {
		return turnCharge{}, true, refuseSupport(c, fiber.StatusTooManyRequests,
			"The support assistant is very busy today. Please email "+supportEmail+" and a person will reply.")
	}
	// Every message is a new one here (there is no retry route), so it always takes a
	// reference no other reservation holds; see chargeTurn for why the persisted count alone
	// is not enough while a turn is running or queued.
	ref, d, err := reserveTurnRef(sess.ID, int(turns)+1, func(ref string) (plan.Decision, error) {
		return h.plans.ConsumeSupport(c.Context(), sess.UserID, ref)
	})
	switch {
	case err == nil && d.Charge == 0:
		return turnCharge{}, true, fiber.NewError(fiber.StatusConflict, errTurnQueueFull.Error())
	case err == nil:
		return turnCharge{feature: plan.FeatureSupport, ref: ref}, false, nil
	case errors.Is(err, plan.ErrRefused):
		return turnCharge{}, true, refuseSupport(c, fiber.StatusPaymentRequired,
			"You've used today's support messages. Please email "+supportEmail+" and a person will reply.")
	default:
		log.Printf("support: charging a message for user %d: %v", sess.UserID, err)
		return turnCharge{}, true, fiber.NewError(fiber.StatusServiceUnavailable, "support is not available")
	}
}

func refuseSupport(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(fiber.Map{"error": message, "support_email": supportEmail})
}
