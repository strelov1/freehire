package handler

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
)

// A turn is the unit charged, not the model call: the call count depends on how many tools the model
// reaches for, and charging per call would make identical-looking requests cost differently.
// Which allowance a turn draws on depends on the preset. The tailoring workspace is metered by its OWN
// two bounds (daily sessions, per-session turns), so a turn inside it takes no assistant allowance;
// charging both would let the daily assistant allowance decide how deep one CV may be edited. Every
// other preset draws on the shared assistant allowance, chat and profile together.

// turnCharge is what a metered turn took, so the caller can give it back if the turn
// produces nothing. A zero value means nothing was charged and a release is a no-op.
type turnCharge struct {
	feature plan.Feature
	ref     string
}

// meterTurn decides whether a turn may run and takes what it costs, BEFORE the stream opens: a 402
// written after the headers is an event inside a 200, and the SPA would render an empty answer instead of
// an upgrade prompt. It returns what was charged, whether it refused, and the error from writing the
// refusal. The middle value is load-bearing: writing a 402 through Fiber SUCCEEDS, so a written refusal
// returns a nil error, and deciding by the error alone would open the stream anyway (a test asserts no
// event stream on a 402). Metering fails OPEN: a counter that cannot be read logs and lets the turn
// through uncharged, since bookkeeping must never refuse a legitimate question.
func (h *assistantHandlers) meterTurn(c *fiber.Ctx, sess assistant.Session) (turnCharge, bool, error) {
	turns, ok := h.sessionTurns(c, sess)
	if !ok {
		return turnCharge{}, false, nil
	}
	if sess.Preset == assistant.PresetTailor {
		d, err := h.plans.AllowTurn(c.Context(), sess.UserID, sess.ID.String(), turns)
		if err != nil {
			log.Printf("plan: tailoring turn allowance for session %s: %v", sess.ID, err)
			return turnCharge{}, false, nil
		}
		if d.Allowed {
			return turnCharge{}, false, nil
		}
		// The refusal reports the tailoring STANDING, not the turn count: the candidate acts on whether they can
		// spend another of today's sessions, and only the standing carries that and its reset instant.
		st, err := h.plans.Standing(c.Context(), sess.UserID, plan.FeatureTailor)
		if err != nil {
			log.Printf("plan: tailoring standing for user %d: %v", sess.UserID, err)
			return turnCharge{}, false, nil
		}
		return turnCharge{}, true, refuseSessionCeiling(c, sess.ID.String(), d, st)
	}
	// The turn about to be recorded is the next one; charging under that number lets the retry below land
	// on the same reference and consume nothing further.
	return h.chargeTurn(c, sess, turns+1, true)
}

// meterRetry is meterTurn for a resumed turn: the message already exists, so the turn paid for is the
// one already counted, not the next.
func (h *assistantHandlers) meterRetry(c *fiber.Ctx, sess assistant.Session) (turnCharge, bool, error) {
	turns, ok := h.sessionTurns(c, sess)
	if !ok || turns == 0 {
		return turnCharge{}, false, nil
	}
	if sess.Preset == assistant.PresetTailor {
		// A retry re-runs work inside a ceiling checked when the message was first sent; checking it again
		// against a count that includes that message would refuse the very turn the ceiling admitted.
		return turnCharge{}, false, nil
	}
	return h.chargeTurn(c, sess, turns, false)
}

// sessionTurns is how many turns the session has run, and whether metering can proceed. No meter or an
// unreadable count both answer false and the turn runs uncharged (the fail-open rule above).
func (h *assistantHandlers) sessionTurns(c *fiber.Ctx, sess assistant.Session) (int, bool) {
	if h.plans == nil {
		return 0, false
	}
	turns, err := h.queries.CountAssistantUserTurns(c.Context(), sess.ID)
	if err != nil {
		log.Printf("plan: counting turns for session %s: %v", sess.ID, err)
		return 0, false
	}
	return int(turns), true
}

// chargeTurn takes one assistant allowance for turn number n of this session. The number makes a retry
// idempotent: a resumed turn appends no second user message, so it charges under the original reference.
// fresh says this request brings a NEW message. n is read from the persisted count, and a running or
// queued turn has not appended its message yet, so a second new message would compute the first one's
// reference, find it paid for and run free. A fresh charge therefore moves to the next number while its
// reference is taken (reserveTurnRef); a resume must not.
// A charge handle comes back ONLY when this request really took an allowance. Consume also answers yes
// for a reference already paid for, and a handle then would arm a release over somebody else's charge.
func (h *assistantHandlers) chargeTurn(c *fiber.Ctx, sess assistant.Session, n int, fresh bool) (turnCharge, bool, error) {
	consume := func(ref string) (plan.Decision, error) {
		return h.plans.Consume(c.Context(), sess.UserID, plan.FeatureAssistant, ref)
	}
	var (
		ref string
		d   plan.Decision
		err error
	)
	if fresh {
		ref, d, err = reserveTurnRef(sess.ID, n, consume)
	} else {
		ref = turnRef(sess.ID, n)
		d, err = consume(ref)
	}
	switch {
	case err == nil && d.Charge == 0 && fresh:
		return turnCharge{}, true, fiber.NewError(fiber.StatusConflict, errTurnQueueFull.Error())
	case err == nil && d.Charge == 0:
		return turnCharge{}, false, nil // already paid for — this request owes nothing back
	case err == nil:
		return turnCharge{feature: plan.FeatureAssistant, ref: ref}, false, nil
	case isRefusal(err):
		return turnCharge{}, true, refuse(c, d)
	default:
		log.Printf("plan: charging an assistant turn for user %d: %v", sess.UserID, err)
		return turnCharge{}, false, nil
	}
}

func turnRef(session uuid.UUID, n int) string {
	return session.String() + "#turn-" + strconv.Itoa(n)
}

// turnRefAttempts is how many consecutive turn numbers a new message may try: a session runs one turn
// and queues one (turnRegistry.claim), so at most two reservations can be ahead of a third request.
const turnRefAttempts = 3

// reserveTurnRef consumes the first reference from turn number first onward that no other reservation
// holds. The ledger's uniqueness decides, under the usage row's lock, so requests computing the same
// number cannot both take it. If every attempt charged nothing, each was already paid for and the
// caller refuses rather than letting the turn run free.
func reserveTurnRef(session uuid.UUID, first int, consume func(ref string) (plan.Decision, error)) (string, plan.Decision, error) {
	var (
		ref string
		d   plan.Decision
		err error
	)
	for i := 0; i < turnRefAttempts; i++ {
		ref = turnRef(session, first+i)
		d, err = consume(ref)
		if err != nil || d.Charge > 0 {
			return ref, d, err
		}
	}
	return ref, d, nil
}

// turnReleaseTimeout bounds the detached cleanup: short enough that a wedged database cannot pile up goroutines.
const turnReleaseTimeout = 5 * time.Second

// releaseTurn gives back what a turn took when it produced nothing usable. Safe to call blind: a zero
// charge releases nothing. It runs on a detached context on purpose: a client disconnect cancels the
// request context, and a release on it could not even open its transaction, leaving the candidate
// charged for an answer they never got.
func (h *assistantHandlers) releaseTurn(sess assistant.Session, charge turnCharge) {
	if h.plans == nil || charge.ref == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), turnReleaseTimeout)
	defer cancel()
	if err := h.plans.Release(ctx, sess.UserID, charge.feature, charge.ref); err != nil {
		log.Printf("plan: releasing an assistant turn for user %d: %v", sess.UserID, err)
	}
}

// refuseSessionCeiling writes the 402 for a session that has run as far as it paid for. It names the
// TAILORING allowance, not the assistant one, because spending another of today's sessions is what the
// candidate does about it. The session and its turn ceiling ride along so the workspace can offer to
// extend that conversation.
func refuseSessionCeiling(c *fiber.Ctx, sessionID string, d plan.TurnDecision, st plan.Standing) error {
	body := fiber.Map{
		"error":     "This CV editing session has run as far as today's session covers.",
		"allowance": viewStanding(st),
		"session":   fiber.Map{"id": sessionID, "turns": d.Turns, "ceiling": d.Ceiling},
	}
	// Extending spends another of today's sessions, so it is offered only when that would go through
	// (otherwise a button that answers 402). Asking Exhausted instead would withhold the offer while the
	// allowance is only being counted, a refusal shadow mode exists to prevent.
	body["can_extend"] = !st.Refuses()
	return c.Status(fiber.StatusPaymentRequired).JSON(body)
}
