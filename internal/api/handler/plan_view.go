package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
)

// allowanceView is how a plan allowance reaches the wire: usage against a limit, never a
// balance of a currency (the word "credits" appears nowhere).
//
// Limit is omitted for an unlimited caller: sending the fair-use guard as "limit" would
// present an infrastructure defence as what they bought.
//
// Enforced is on the wire because the SPA pre-blocks actions a spent allowance would refuse;
// a client that cannot tell a live ceiling from a shadow one refuses what the server would
// allow, and suppresses the very requests the shadow run is counting.
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
// than the account, because the three refusals differ: one clears at midnight, one is
// throttling, one means the plan never included this.
//
// inPlan is that third case. Free has no allowance for any metered feature
// (internal/ai/plan.DefaultConfig), so `used >= limit` holds at 0 >= 0 and a free account
// would be told "You've used today's job analyses" with a false reset time.
func refusalMessage(feature plan.Feature, fairUse, inPlan bool) string {
	if fairUse {
		return "This account has hit an unusually high volume for today. It will reset tomorrow."
	}
	noun := featureNoun(feature)
	if !inPlan {
		if noun == "" {
			return "Your plan doesn't include this."
		}
		return "Your plan doesn't include " + noun + "."
	}
	if noun == "" {
		return "You've used today's allowance for this."
	}
	return "You've used today's " + noun + "."
}

// featureNoun names what a feature gives the candidate, in the plural: "You've used today's
// <noun>" and "Your plan doesn't include <noun>".
//
// A feature with no noun falls through to a sentence that reads without one rather than a
// placeholder (a wrong noun is worse than none). A new feature's wording is added here.
func featureNoun(feature plan.Feature) string {
	switch feature {
	case plan.FeatureTailor:
		return "CV edits"
	case plan.FeatureFit:
		return "job analyses"
	case plan.FeatureAssistant:
		return "assistant messages"
	case plan.FeatureDictation:
		return "dictation"
	case plan.FeatureAutoApply:
		return "automatic applications"
	default:
		return ""
	}
}

// planIncludes reports whether the plan behind an allowance gives the caller any of the
// feature. Read off the allowance, not the tier: free is refused by the paid gate
// (paid_ai.go) before an allowance is read, but a paid tier also reads zero for an
// unconfigured feature, and "paid" is not "gets some of it".
func planIncludes(a allowanceView) bool { return a.Unlimited || a.Limit > 0 }

// isRefusal reports whether an error is the meter saying no, as opposed to the meter
// itself failing. The two must never be confused: one is an answer to give the caller,
// the other is a fault to log and let the request through.
func isRefusal(err error) bool { return errors.Is(err, plan.ErrRefused) }

// refuse writes the 402 a spent allowance answers with: what ran out, where the caller
// stands, when it resets, and where to upgrade.
//
// Upgrade is omitted for a fair-use refusal and for a caller already on the top tier: selling
// a bigger plan when an infrastructure guard stopped somebody reads as a shakedown, and there
// is nothing to sell the top tier. A pro subscriber refused for auto-apply is the one person
// for whom the next plan is worth offering.
func refuse(c *fiber.Ctx, d plan.Decision) error {
	return decisionRefusal(d).write(c)
}

// planRefusal is the body of a plan 402, held apart from the response it is written to so
// a Fiber handler (write) and a contract handler (see cvHandlers) answer it identically.
type planRefusal struct {
	allowance    allowanceView
	message      string
	offerUpgrade bool
}

// decisionRefusal is the refusal for a consumption the plan turned away (see refuse).
func decisionRefusal(d plan.Decision) planRefusal {
	a := viewDecision(d)
	return planRefusal{a, refusalMessage(d.Feature, d.FairUse, planIncludes(a)), !d.FairUse && hasSomethingToBuy(d.Tier)}
}

// standingRefusal is the refusal for a pre-check, which holds a Standing (see refuseStanding).
func standingRefusal(st plan.Standing) planRefusal {
	a := viewStanding(st)
	return planRefusal{a, refusalMessage(st.Feature, false, planIncludes(a)), hasSomethingToBuy(st.Tier)}
}

func (r planRefusal) write(c *fiber.Ctx) error {
	return write402(c, r.allowance, r.message, r.offerUpgrade)
}

// hasSomethingToBuy reports whether a tier has a bigger one above it. Written as "not the
// top" so adding a tier does not silently stop offering an upgrade to the one it displaced.
func hasSomethingToBuy(tier plan.Tier) bool { return tier != plan.TierUltra }

// refuseStanding is refuse for a caller stopped by a PRE-CHECK, which holds a Standing
// rather than a Decision. Same body and rules, so a pre-check's refusal and the charge's own
// are indistinguishable. A Decision filled in by hand would be a lie that reads as fact at
// the next call site.
func refuseStanding(c *fiber.Ctx, st plan.Standing) error {
	return standingRefusal(st).write(c)
}

// upgradePath is where a refused caller is sent: the account plan page.
const upgradePath = "/my/plan"

func write402(c *fiber.Ctx, allowance allowanceView, message string, offerUpgrade bool) error {
	body := fiber.Map{"error": message, "allowance": allowance}
	if offerUpgrade {
		body["upgrade_url"] = upgradePath
	}
	return c.Status(fiber.StatusPaymentRequired).JSON(body)
}
