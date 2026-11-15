package handler

import (
	"errors"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/atscheck"
	"github.com/strelov1/freehire/internal/candidate/cvsection"
	"github.com/strelov1/freehire/internal/job/verdict"
	"github.com/strelov1/freehire/internal/search/search"
)

// roastRoleValues builds the market filter for a public roast and names the role it
// selected. Like roleValues, but an anonymous caller has no stored profile, so the
// fallback is the CV's own inferred categories.
//
// Order: an explicit `category` param, else the first inferred category (primary first),
// else NO role filter and an empty role name. The last is a refusal, not a guess: a
// coverage figure for the wrong role reads like a correct one.
//
// Seniority is deliberately not folded in: junior-only coverage is a smaller number that
// says nothing about skills, which is what this page sells.
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

// roastResponse is the public roast's wire shape. Report is the deterministic ATS score,
// never model-refined here. Role is the category the market was measured against, empty
// when none resolved; MarketScoped says so explicitly. Market is nil when the facet
// backend is unavailable (see MarketAvailable).
type roastResponse struct {
	Report          *atscheck.Report `json:"report"`
	Role            string           `json:"role"`
	MarketScoped    bool             `json:"market_scoped"`
	MarketAvailable bool             `json:"market_available"`
	Market          *verdict.Verdict `json:"market,omitempty"`
}

// RoastCV scores an uploaded CV for an ANONYMOUS caller: the deterministic ATS report
// plus a live market-coverage reading, with no account, no stored bytes and no model
// call. Body forms match ExtractResumeProfile (multipart PDF or {text} JSON).
//
// Nothing is written: the bytes die with the request. PII masking is absent because no
// model runs. Public on purpose: it reads a CV without storing it.
func (h *resumeHandlers) RoastCV(c *fiber.Ctx) error {
	// Before readResumeUpload: Fiber decompresses an encoded body without a ceiling and
	// this route has no account behind it. See refuseEncodedBody.
	if err := refuseEncodedBody(c); err != nil {
		return err
	}

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

	// One role query feeds both readings so they cannot disagree about the role.
	roleFacet, ferr := h.roleFacet(c, roleFilter)

	report := atscheck.Score(up.Text, profile.Skills, atscheck.TopRoleSkills(roleFacet.Facets["skills"], atscheck.RoleTopN))
	out := roastResponse{
		Report:       &report,
		Role:         role,
		MarketScoped: role != "",
	}

	// Search being down costs only the market half: the ATS score needs no index, and a
	// 503 (as MarketCoverage answers) would discard the reading the visitor came for.
	if ferr == nil {
		declared, body, all := cvsection.Parse(up.Text)
		v, err := h.coverageWithRole(c.Context(), roleFilter, roleFacet, profile.Skills, declared, body, all)
		if err == nil {
			out.MarketAvailable = true
			out.Market = &v
		}
	}
	return dataResponseWithIgnored(c, out, coverageIgnoredParams(queryValues(c)), ignoredJobValues(queryValues(c)))
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
