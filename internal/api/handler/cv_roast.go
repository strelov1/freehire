package handler

import (
	"net/url"

	"github.com/gofiber/fiber/v2"
)

// roastRoleValues builds the market filter for a public roast and names the role it
// selected. It mirrors roleValues (same skills strip, same category default) with one
// difference that matters: roleValues falls back to the caller's stored profile, and an
// anonymous caller has none, so the fallback is the CV's own inferred categories.
//
// Three outcomes, and the third is a refusal rather than a guess: an explicit `category`
// param wins; otherwise the first inferred category (classify returns them in precedence
// order, primary first); otherwise NO role filter and an empty role name. classify never
// guesses either — it returns nothing when its dictionary resolves nothing — and a
// coverage figure attributed to the wrong role reads exactly like a correct one, so the
// page is told the truth and says "the whole catalogue" instead.
//
// The seniority resolveProfile also derives is deliberately not folded in: filtering a
// junior's coverage to junior-only postings yields a smaller number that says nothing
// about their skills, and skills are the reading this page sells.
func roastRoleValues(c *fiber.Ctx, categories []string) (url.Values, string) {
	vals := queryValues(c)
	stripSkillParams(vals)
	if hasNonEmpty(vals["category"]) {
		return vals, vals["category"][0]
	}
	if len(categories) > 0 {
		vals["category"] = []string{categories[0]}
		return vals, categories[0]
	}
	delete(vals, "category")
	return vals, ""
}
