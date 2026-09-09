package handler

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/candidate/atscheck"
	"github.com/strelov1/freehire/internal/candidate/cvsection"
	"github.com/strelov1/freehire/internal/job/verdict"
	"github.com/strelov1/freehire/internal/search/search"
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

// roastResponse is the public roast's wire shape. Report is the deterministic ATS
// score — never model-refined on this path. Role names the category the market was
// measured against, empty when the dictionary resolved none; MarketScoped says which
// of those two happened, so the page never has to infer it from an empty string.
// Market is nil when the facet backend is unavailable (see MarketAvailable).
type roastResponse struct {
	Report          *atscheck.Report `json:"report"`
	Role            string           `json:"role"`
	MarketScoped    bool             `json:"market_scoped"`
	MarketAvailable bool             `json:"market_available"`
	Market          *verdict.Verdict `json:"market,omitempty"`
}

// RoastCV scores an uploaded CV for an ANONYMOUS caller: the deterministic ATS report
// plus a live market-coverage reading, with no account, no stored bytes and no model
// call. It accepts the same two body forms as ExtractResumeProfile (multipart PDF or
// {text} JSON) through the same reader.
//
// Nothing here writes. ExtractResumeProfile stores the résumé because its signed-in
// caller's later steps need the file; an anonymous visitor has no later step, so the
// bytes die with the request. The PII masking layer is likewise absent by construction
// rather than by omission — it exists to protect a CV from a MODEL, and no model runs.
//
// Withholding the model review is also the page's whole offer: the deterministic score
// names what is wrong, and signing in is what rewrites it.
//
// Public on purpose and rate-limited by IP at the route (see resume.go's register).
func (h *resumeHandlers) RoastCV(c *fiber.Ctx) error {
	up, err := readResumeUpload(c)
	if err != nil {
		return err
	}
	if strings.TrimSpace(up.Text) == "" {
		return fiber.NewError(fiber.StatusBadRequest, errResumeNoText)
	}

	profile := resumeProfile(up.Text)
	vals, role := roastRoleValues(c, profile.Categories)
	roleFilter := search.FilterFromValues(vals)

	// One role query feeds both readings, so the ATS keyword score and the coverage
	// figure can never disagree about what this role demands.
	roleFacet, ferr := h.roleFacet(c, roleFilter)

	report := atscheck.Score(up.Text, profile.Skills, topRoleSkills(roleFacet.Facets["skills"], atsRoleTopN))
	out := roastResponse{
		Report:       &report,
		Role:         role,
		MarketScoped: role != "",
	}

	// Search being down costs the market half, not the answer: the ATS score needs no
	// index at all, and a 503 would throw away the reading the visitor came for.
	// MarketCoverage answers 503 in the same situation, which is right for an API
	// client and wrong for a landing page.
	if ferr == nil {
		declared, body, all := cvsection.Parse(up.Text)
		v, err := h.coverageWithRole(c.Context(), roleFilter, roleFacet, profile.Skills, declared, body, all)
		if err == nil {
			out.MarketAvailable = true
			out.Market = &v
		}
	}
	return dataResponseWithIgnored(c, out, coverageIgnoredParams(c))
}

// roleFacet answers the role's skill distribution, or a zero result and an error when
// the facet backend is unconfigured or unreachable. Both are non-fatal here.
func (h *resumeHandlers) roleFacet(c *fiber.Ctx, roleFilter any) (search.FacetResult, error) {
	if h.facets == nil {
		return search.FacetResult{}, errSearchUnavailable
	}
	return h.facets.FacetCounts(c.Context(), search.FacetParams{
		Filter: roleFilter,
		Facets: []string{"skills"},
	})
}

// errSearchUnavailable stands for "no facet backend" so roleFacet has one error kind
// for both of its failures and the caller needs no nil check of its own.
var errSearchUnavailable = errors.New("search is not available")

// cvRoastPerHour bounds the public roast per IP. The work is a pdftotext subprocess
// plus three facet queries — not free, and not a model call either, so this is sized to
// stop a scraper rather than to ration something scarce.
//
// Per HOUR rather than per minute, on purpose: someone fixing their CV genuinely
// re-uploads it several times in a row, and that is the behaviour the page wants. A
// per-minute ceiling would punish exactly the visitor who is getting value.
const cvRoastPerHour = 10

// cvRoastLimiter bounds the public roast by source address. There is no authenticated
// caller to key by — that is what public means — and the route mounts no auth gate, so
// KeyByIP is not a fallback here, it is the only thing there is.
func cvRoastLimiter(throttler ratelimit.Throttler) fiber.Handler {
	return ratelimit.Middleware(throttler, ratelimit.KeyByIP("cvroast"), cvRoastPerHour, time.Hour)
}
