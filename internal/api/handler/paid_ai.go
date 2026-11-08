package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/plan"
)

// paidEntitlement is what the paid-AI gate asks of the plan: whether the caller is on a paid
// tier at all, and — only to word a refusal — where they stand on the feature they asked
// for — and to remember a refusal, which is the one write it makes. *plan.Store in production.
type paidEntitlement interface {
	RequirePaid(ctx context.Context, userID int64) (plan.Tier, error)
	Standing(ctx context.Context, userID int64, f plan.Feature) (plan.Standing, error)
	RecordPaywallHit(ctx context.Context, userID int64, feature string, job plan.PaywallJob) error
}

// entitlementOf adapts a plan store to the gate, keeping a nil store nil. A nil *plan.Store
// held in the interface would read as configured and dereference on the first request; the
// gate treats a genuinely absent one as "could not tell", which is a 503.
func entitlementOf(s *plan.Store) paidEntitlement {
	if s == nil {
		return nil
	}
	return s
}

// unmeteredAI is the feature an AI route names when it draws on no daily allowance of its
// own — the ATS review, mail recall, autofill, a re-run of extraction. The
// paid gate still stands in front of it; only the refusal has no allowance to quote.
const unmeteredAI plan.Feature = ""

// paidAIGate builds the route middleware every user-initiated model call sits behind. It is
// a function of the feature rather than one handler, so a refusal on a metered route names
// what the plan does not include in the same body every other plan refusal carries; a route
// with no allowance of its own (the ATS review, mail recall, autofill,
// extraction) passes the zero Feature.
//
// Mounted AFTER the authentication gate, which is what put the caller's id in locals.
//
// It is one of two halves. The other is the user-call binding (user_llm.go), which refuses
// to hand out a model client for a context this gate did not mark — so a route that makes a
// model call and was mounted without this middleware fails closed on its first request
// rather than spending on somebody who never paid.
func paidAIGate(plans paidEntitlement) func(plan.Feature) fiber.Handler {
	return func(feature plan.Feature) fiber.Handler {
		return func(c *fiber.Ctx) error {
			userID, err := requireUserID(c)
			if err != nil {
				return err
			}
			if ok, err := authorizePaidAI(c, plans, userID, feature); !ok {
				return err
			}
			return c.Next()
		}
	}
}

// authorizePaidAI is the gate's check for one user, for a handler that cannot use the
// middleware. A refusal is recorded as a paywall hit (see recordPaywallHit).
//
// ok is false when the request has been answered: a 402 written to c (with a nil error, since
// `c.Status(402).JSON(...)` returns nil — which is why there is a bool beside it), or a 503
// returned as the error when the plan could not be read. A read failure never authorises:
// an entitlement nobody could read is not one, and what this protects is model spend.
//
// On success the request's user context is marked (plan.WithPaidAI), which is what the
// user-call binding reads before it hands out a model client. Handlers pass c.UserContext()
// — or a context derived from it — to the binding for exactly that reason.
func authorizePaidAI(c *fiber.Ctx, plans paidEntitlement, userID int64, feature plan.Feature) (bool, error) {
	return checkPaidAI(c, plans, userID, feature, true)
}

// authorizePaidAIUnrecorded is authorizePaidAI for the handler whose user is not the
// authenticated caller — the auto-apply orchestrator acts for whichever candidate owns a queue
// entry. Nobody pressed a paid button there, so a refusal is not a buying signal and is not
// recorded.
func authorizePaidAIUnrecorded(c *fiber.Ctx, plans paidEntitlement, userID int64, feature plan.Feature) (bool, error) {
	return checkPaidAI(c, plans, userID, feature, false)
}

func checkPaidAI(c *fiber.Ctx, plans paidEntitlement, userID int64, feature plan.Feature, record bool) (bool, error) {
	if plans == nil {
		// A deployment wired without a plan store has no way to know who paid, and the
		// answer to "we could not tell" is the same as for a failed read.
		return false, fiber.NewError(fiber.StatusServiceUnavailable, "plan information is unavailable")
	}
	tier, err := plans.RequirePaid(c.Context(), userID)
	if errors.Is(err, plan.ErrPaymentRequired) {
		if record {
			recordPaywallHit(c, plans, userID, feature)
		}
		return false, refusePaidAI(c, plans, userID, tier, feature)
	}
	if err != nil {
		log.Printf("plan: paid entitlement for user %d: %v", userID, err)
		return false, fiber.NewError(fiber.StatusServiceUnavailable, "plan information is unavailable")
	}
	c.SetUserContext(plan.WithPaidAI(c.UserContext()))
	return true, nil
}

// refusePaidAI writes the 402 an unpaid caller gets, in the shape every plan refusal has —
// the sentence, the upgrade link, and for a metered feature the allowance it does not
// include — so the web and the extension render it through the path they already have.
//
// The allowance is read rather than assembled: a refusal built from fields filled in by
// hand is a lie that reads as fact at the next call site (see refuseStanding). If that read
// fails, the refusal still stands and goes out without it; there is nothing left to decide.
func refusePaidAI(c *fiber.Ctx, plans paidEntitlement, userID int64, tier plan.Tier, feature plan.Feature) error {
	if feature != "" {
		st, err := plans.Standing(c.Context(), userID, feature)
		if err == nil {
			return refuseStanding(c, st)
		}
		log.Printf("plan: standing for a paid-AI refusal, user %d on %s: %v", userID, feature, err)
	}
	body := fiber.Map{"error": refusalMessage(feature, false, false)}
	if hasSomethingToBuy(tier) {
		body["upgrade_url"] = upgradePath
	}
	return c.Status(fiber.StatusPaymentRequired).JSON(body)
}

// paywallWriteTimeout bounds the detached write of a paywall hit.
const paywallWriteTimeout = 5 * time.Second

// paywallWrites counts the paywall-hit writes in flight, so a test can wait for the detached
// goroutine instead of sleeping.
var paywallWrites sync.WaitGroup

// unmeteredFeatureNames labels the paid routes that draw on no allowance, by route template —
// never by the raw URL, which carries slugs and ids.
var unmeteredFeatureNames = map[string]string{
	"/api/v1/me/profile/ats-report":         "ats_review",
	"/api/v1/me/tracking/:slug/mail-recall": "mail_recall",
	"/api/v1/me/autofill/run":               "autofill",
}

// unknownPaywallFeature labels a refusal on an unmetered route missing from the table above.
const unknownPaywallFeature = "ai_other"

// recordPaywallHit remembers that an unpaid signed-in user was refused, for the paywall
// report and the letter that answers it.
//
// It reads everything it needs from the request now — fiber recycles the context once the
// handler returns — and writes from a goroutine on a context detached from the request, so
// the 402 neither waits for the database nor depends on it. The goroutine recovers its own
// panic, which the server's recover middleware cannot reach; a failed write is logged and
// dropped.
func recordPaywallHit(c *fiber.Ctx, plans paidEntitlement, userID int64, feature plan.Feature) {
	label := string(feature)
	if feature == unmeteredAI {
		var ok bool
		if label, ok = unmeteredFeatureNames[c.Route().Path]; !ok {
			label = unknownPaywallFeature
		}
	}
	job := paywallJob(c)
	ctx := context.WithoutCancel(c.UserContext())

	paywallWrites.Add(1)
	go func() {
		defer paywallWrites.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("plan: paywall hit for user %d on %s panicked: %v", userID, label, r)
			}
		}()
		ctx, cancel := context.WithTimeout(ctx, paywallWriteTimeout)
		defer cancel()
		if err := plans.RecordPaywallHit(ctx, userID, label, job); err != nil {
			log.Printf("plan: record paywall hit for user %d on %s: %v", userID, label, err)
		}
	}()
}

// paywallJob names the vacancy the refused route concerns, chosen from the route TEMPLATE:
// a job slug in the path or, for starting a tailoring, in the body; or the CV or assistant
// session the path names, which the insert resolves through the caller's own rows. A value
// that does not parse is no reference.
func paywallJob(c *fiber.Ctx) plan.PaywallJob {
	path := c.Route().Path
	switch {
	case strings.HasSuffix(path, "/me/cvs/tailor"):
		var body struct {
			JobSlug string `json:"job_slug"`
		}
		_ = json.Unmarshal(c.Body(), &body)
		return plan.PaywallJob{Slug: body.JobSlug}
	case strings.Contains(path, "/me/cvs/:id"):
		id, _ := uuid.Parse(c.Params("id"))
		return plan.PaywallJob{CVID: id}
	case strings.Contains(path, "/assistant/sessions/:id"):
		id, _ := uuid.Parse(c.Params("id"))
		return plan.PaywallJob{SessionID: id}
	}
	return plan.PaywallJob{Slug: c.Params("slug")}
}
