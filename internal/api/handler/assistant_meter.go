package handler

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/ai/plan"
)

// A turn is the unit charged, not the model call.
//
// One turn ran 7.1 model calls on average in production, and that number is invisible to
// the person who asked the question — it depends on how many tools the model decided to
// reach for. Charging per call would make two identical-looking requests cost differently
// for reasons the candidate can neither see nor influence.
//
// Which allowance a turn draws on depends on the preset, and the split is not cosmetic.
// The tailoring workspace is metered by its OWN two bounds (a daily session count and a
// per-session turn ceiling), so a turn inside it consumes no assistant allowance: charging
// it twice would let the daily assistant allowance decide how deep one CV may be edited.
// Every other preset draws on the shared assistant allowance, chat and profile together —
// they are the same conversation surface pointed at different things, and a per-preset
// allowance would only teach a candidate which name to type.

// turnCharge is what a metered turn took, so the caller can give it back if the turn
// produces nothing. A zero value means nothing was charged and a release is a no-op.
type turnCharge struct {
	feature plan.Feature
	ref     string
}

// meterTurn decides whether a turn may run and takes what it costs, BEFORE the stream
// opens. That ordering is the point: a 402 written after the headers are out is an event
// inside a 200, invisible to anything that checks status codes, and the SPA would render
// an empty answer instead of an upgrade prompt.
//
// It returns what was charged, whether it refused, and the error from writing that refusal.
//
// The middle value is load-bearing and cannot be inferred from the error: writing a 402
// through Fiber SUCCEEDS, so a refusal that has been written returns a nil error. Deciding
// by the error alone would let a refused turn fall through and open its stream anyway —
// which is exactly what happened once, and what the test asserting no event stream on a
// 402 now catches.
//
// Metering fails OPEN. A counter that cannot be read logs and lets the turn through
// uncharged: bookkeeping must never be able to refuse a legitimate question, and an
// uncharged turn is a smaller wrong than a candidate stopped by our accounting.
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
		// The refusal reports the tailoring STANDING, not the turn count. What the
		// candidate has to act on is whether they can spend another of today's sessions to
		// continue — and that answer, with its reset instant, only the standing carries.
		st, err := h.plans.Standing(c.Context(), sess.UserID, plan.FeatureTailor)
		if err != nil {
			log.Printf("plan: tailoring standing for user %d: %v", sess.UserID, err)
			return turnCharge{}, false, nil
		}
		return turnCharge{}, true, refuseSessionCeiling(c, sess.ID.String(), d, st)
	}
	// The turn about to be recorded is the next one. Charging it under that number is what
	// lets the retry below land on the same reference and consume nothing further.
	return h.chargeTurn(c, sess, turns+1)
}

// meterRetry is meterTurn for a resumed turn: the user message already exists, so the turn
// being paid for is the one already counted rather than the next one.
func (h *assistantHandlers) meterRetry(c *fiber.Ctx, sess assistant.Session) (turnCharge, bool, error) {
	turns, ok := h.sessionTurns(c, sess)
	if !ok || turns == 0 {
		return turnCharge{}, false, nil
	}
	if sess.Preset == assistant.PresetTailor {
		// A retry of a tailoring turn re-runs work inside a ceiling that was already
		// checked when the message was first sent. Checking it again against a turn count
		// that now includes that message would refuse the retry of the very turn the
		// ceiling admitted.
		return turnCharge{}, false, nil
	}
	return h.chargeTurn(c, sess, turns)
}

// sessionTurns is how many turns the session has run, and whether metering can proceed at
// all. A deployment with no meter and a count that cannot be read both answer false, and
// the turn then runs uncharged — the fail-open rule above.
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

// chargeTurn takes one assistant allowance for turn number n of this session. The number
// is what makes a retry idempotent: a resumed turn appends no second user message, so it
// charges under the reference the original charge was filed with and takes nothing more.
//
// A charge handle comes back ONLY when this request really took an allowance. Consume also
// answers yes for a reference already paid for, and handing back a handle then would arm a
// release over somebody else's charge — a turn refused the session's slot computes the
// reference of the turn QUEUED ahead of it, because that one has not appended its message
// yet, and would refund a turn that is still going to run.
func (h *assistantHandlers) chargeTurn(c *fiber.Ctx, sess assistant.Session, n int) (turnCharge, bool, error) {
	ref := sess.ID.String() + "#turn-" + strconv.Itoa(n)
	d, err := h.plans.Consume(c.Context(), sess.UserID, plan.FeatureAssistant, ref)
	switch {
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

// turnReleaseTimeout bounds the detached cleanup. Generous for two small statements, and
// short enough that a wedged database cannot pile up goroutines behind it.
const turnReleaseTimeout = 5 * time.Second

// releaseTurn gives back what a turn took when it produced nothing the candidate can use.
// Safe to call blind: a zero charge releases nothing, and a release of something never
// charged is a no-op in the store.
//
// It runs on a detached context on purpose. A client that disconnects mid-turn cancels the
// request context, the turn fails with it, and a release on that same context could not
// even open its transaction — leaving the candidate charged for an answer they never got,
// in exactly the case this exists for.
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

// refuseSessionCeiling writes the 402 for a session that has run as far as it paid for.
//
// It names the TAILORING allowance rather than the assistant one, because spending another
// of today's sessions is what the candidate does about it — pointing them at the assistant
// allowance would send them to look at a number that is not the one stopping them.
//
// The session and its turn ceiling ride along, so the workspace can say which conversation
// stopped and offer to extend that one rather than guessing.
func refuseSessionCeiling(c *fiber.Ctx, sessionID string, d plan.TurnDecision, st plan.Standing) error {
	body := fiber.Map{
		"error":     "This CV editing session has run as far as today's session covers.",
		"allowance": viewStanding(st),
		"session":   fiber.Map{"id": sessionID, "turns": d.Turns, "ceiling": d.Ceiling},
	}
	// Extending spends another of today's sessions, so it is only offered when spending one
	// would actually go through. Offering it otherwise is a button that answers 402 — and
	// asking Exhausted instead would withhold the offer while the allowance is only being
	// counted, which is a refusal shadow mode exists to prevent.
	body["can_extend"] = !st.Refuses()
	return c.Status(fiber.StatusPaymentRequired).JSON(body)
}
