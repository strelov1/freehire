package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
)

// allowanceView is how a plan allowance reaches the wire, everywhere it does. Usage against
// a limit — never a balance of a currency, and the word "credits" appears nowhere in it.
//
// Limit is omitted for an unlimited caller rather than sent as a number. A pro plan has no
// user-facing ceiling, and sending the fair-use guard as "limit" would present an
// infrastructure defence as the thing they bought.
//
// Enforced is on the wire because the SPA pre-blocks actions a spent allowance would refuse
// — the tailoring dialog, the fit CTA — and a client that cannot tell a live ceiling from a
// shadow one refuses what the server would have allowed. That is not only a wall nobody
// meant to build: it suppresses exactly the requests the shadow run is counting, so the
// numbers the enforcement decision rests on would come back understated.
type allowanceView struct {
	Feature   string    `json:"feature"`
	Used      int       `json:"used"`
	Limit     int       `json:"limit,omitempty"`
	Unlimited bool      `json:"unlimited"`
	Enforced  bool      `json:"enforced"`
	ResetsAt  time.Time `json:"resets_at"`
}

// view is the one place an allowance becomes wire shape. A Standing, a Decision and a
// FeatureUsage all describe the same facts, so they converge here rather than each growing
// its own mapping that could disagree about the unlimited rule or the enforcement flag.
func view(feature plan.Feature, used, limit int, unlimited, enforced bool, resetsAt time.Time) allowanceView {
	v := allowanceView{
		Feature:   string(feature),
		Used:      used,
		Unlimited: unlimited,
		Enforced:  enforced,
		ResetsAt:  resetsAt,
	}
	if !unlimited {
		v.Limit = limit
	}
	return v
}

func viewStanding(s plan.Standing) allowanceView {
	return view(s.Feature, s.Used, s.Limit, s.Unlimited, s.Enforced, s.ResetsAt)
}

func viewDecision(d plan.Decision) allowanceView {
	return view(d.Feature, d.Used, d.Limit, d.Unlimited, d.Enforced, d.ResetsAt)
}

// refusalMessage is what a refused caller is told. It names the feature that ran out rather
// than the account, because the two refusals mean opposite things to the person reading
// them: one clears at midnight, the other means they are being throttled.
func refusalMessage(feature plan.Feature, fairUse bool) string {
	if fairUse {
		return "This account has hit an unusually high volume for today. It will reset tomorrow."
	}
	switch feature {
	case plan.FeatureTailor:
		return "You've used today's CV edits."
	case plan.FeatureFit:
		return "You've used today's job analyses."
	case plan.FeatureAssistant:
		return "You've used today's assistant messages."
	case plan.FeatureDictation:
		return "You've used today's dictation."
	default:
		return "You've used today's allowance for this."
	}
}

// isRefusal reports whether an error is the meter saying no, as opposed to the meter
// itself failing. The two must never be confused: one is an answer to give the caller,
// the other is a fault to log and let the request through.
func isRefusal(err error) bool { return errors.Is(err, plan.ErrRefused) }

// refuse writes the 402 a spent allowance answers with: what ran out, where the caller
// stands, when it resets, and where to upgrade.
//
// Upgrade is omitted for a fair-use refusal, and for a caller who already pays. Selling a
// bigger plan at the moment an infrastructure guard stopped somebody reads as a shakedown,
// and there is nothing to sell a pro account anyway.
func refuse(c *fiber.Ctx, d plan.Decision) error {
	return write402(c, viewDecision(d), refusalMessage(d.Feature, d.FairUse), !d.FairUse && d.Tier == plan.TierFree)
}

// refuseStanding is refuse for a caller stopped by a PRE-CHECK, which holds a Standing
// rather than the Decision a consumption returns. Same body and same rules: a pre-check's
// refusal and the charge's own must be indistinguishable to whoever reads them.
//
// It exists so a caller does not have to assemble a Decision it never made — a fake one
// filled in field by field is a lie that reads as fact at the next call site.
func refuseStanding(c *fiber.Ctx, st plan.Standing) error {
	return write402(c, viewStanding(st), refusalMessage(st.Feature, false), st.Tier == plan.TierFree)
}

// upgradePath is where a refused free caller is sent. It is the plan page rather than a
// pricing page, because the plan page EXISTS: /pricing arrives with the change that has
// something to sell, and a 402 that links to a 404 is worse than one that links nowhere.
const upgradePath = "/my/plan"

func write402(c *fiber.Ctx, allowance allowanceView, message string, offerUpgrade bool) error {
	body := fiber.Map{"error": message, "allowance": allowance}
	if offerUpgrade {
		body["upgrade_url"] = upgradePath
	}
	return c.Status(fiber.StatusPaymentRequired).JSON(body)
}
