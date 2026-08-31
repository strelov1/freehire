package handler

import (
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
type allowanceView struct {
	Feature   string    `json:"feature"`
	Used      int       `json:"used"`
	Limit     int       `json:"limit,omitempty"`
	Unlimited bool      `json:"unlimited"`
	ResetsAt  time.Time `json:"resets_at"`
}

func viewStanding(s plan.Standing) allowanceView {
	v := allowanceView{
		Feature:   string(s.Feature),
		Used:      s.Used,
		Unlimited: s.Unlimited,
		ResetsAt:  s.ResetsAt,
	}
	if !s.Unlimited {
		v.Limit = s.Limit
	}
	return v
}

func viewDecision(d plan.Decision) allowanceView {
	v := allowanceView{
		Feature:   string(d.Feature),
		Used:      d.Used,
		Unlimited: d.Unlimited,
		ResetsAt:  d.ResetsAt,
	}
	if !d.Unlimited {
		v.Limit = d.Limit
	}
	return v
}

// refusalMessage is what a refused caller is told. It names the feature that ran out rather
// than the account, because the two refusals mean opposite things to the person reading
// them: one clears at midnight, the other means they are being throttled.
func refusalMessage(d plan.Decision) string {
	if d.FairUse {
		return "This account has hit an unusually high volume for today. It will reset tomorrow."
	}
	switch d.Feature {
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

// refuse writes the 402 a spent allowance answers with: what ran out, where the caller
// stands, when it resets, and where to upgrade.
//
// Upgrade is omitted for a fair-use refusal. Selling a bigger plan to somebody who already
// pays, at the moment an infrastructure guard stopped them, reads as a shakedown.
func refuse(c *fiber.Ctx, d plan.Decision) error {
	body := fiber.Map{
		"error":     refusalMessage(d),
		"allowance": viewDecision(d),
	}
	if !d.FairUse && d.Tier == plan.TierFree {
		body["upgrade_url"] = "/pricing"
	}
	return c.Status(fiber.StatusPaymentRequired).JSON(body)
}
