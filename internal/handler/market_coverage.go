package handler

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search"
)

// maxCoverageSkills caps the supplied skill list. Each skill becomes its own
// `skills != "<skill>"` AND group in the uncovered query, so an unbounded list
// would balloon the Meilisearch filter; a real CV lists well under this many
// canonical skills.
const maxCoverageSkills = 100

// coverageRequest is the market-coverage request body: the caller's skill list,
// measured against the filtered market. One skill or many — a single-element list
// is a valid probe of that one skill's demand.
type coverageRequest struct {
	Skills []string `json:"skills"`
}

// MarketCoverage scores a caller-supplied skill list against the live open-vacancy
// market for a facet-filtered role: how many of the role's vacancies list at least
// one of the skills (the covered count), which missing skill unlocks the most new
// vacancies, and the role's top in-demand skills scored against the list. It is the
// stateless, API-key sibling of the CV-based verdict — skills come from the request
// body, the market filter from the facet query params. 400 on empty skills, 503
// when search is unconfigured.
func (a *API) MarketCoverage(c *fiber.Ctx) error {
	if a.facets == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	var req coverageRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	skills := nonEmptyStrings(req.Skills)
	if len(skills) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "skills is required")
	}
	if len(skills) > maxCoverageSkills {
		return fiber.NewError(fiber.StatusBadRequest, "too many skills")
	}

	// A flat list has no CV declared/body sections, so declared and all are the
	// same list and body is empty (statuses collapse to strong/adjacent/missing).
	v, err := a.coverageFor(c.Context(), marketFilter(c), skills, skills, nil, skills)
	if err != nil {
		return err
	}
	// coherence is a CV-section metric (declared ∩ body); a flat skill list has no
	// sections, so it is meaningless here — zero it rather than advertise a score.
	v.CoherencePercent = 0
	return c.JSON(fiber.Map{"data": v})
}

// marketFilter builds the market filter from the request's facet query params
// (the full facet vocabulary), with the skills facet stripped (see stripSkillParams).
func marketFilter(c *fiber.Ctx) any {
	vals, _ := url.ParseQuery(string(c.Request().URI().QueryString()))
	stripSkillParams(vals)
	return search.FilterFromValues(vals)
}

// nonEmptyStrings trims each value and drops the empties, so a stray "" in the
// skills list never becomes a filter fragment.
func nonEmptyStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
