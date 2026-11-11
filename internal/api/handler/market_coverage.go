package handler

import (
	"context"
	"encoding/json"
	"maps"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/search/search"
)

// maxCoverageSkills caps the supplied skill list. Each skill becomes its own
// `skills != "<skill>"` AND group in the uncovered query, so an unbounded list
// would balloon the Meilisearch filter; a real CV lists well under this many
// canonical skills.
const maxCoverageSkills = 100

// MarketCoverage scores a caller-supplied skill list against the live open-vacancy
// market for a facet-filtered role: how many of the role's vacancies list at least
// one of the skills (the covered count), which missing skill unlocks the most new
// vacancies, and the role's top in-demand skills scored against the list. It is the
// stateless, API-key sibling of the CV-based verdict — skills come from the request
// body, the market filter from the facet query params. 400 on empty skills, 503
// when search is unconfigured.
func (h *resumeHandlers) MarketCoverage(ctx context.Context, req *apigen.ResumeCoverageRequest) (*apigen.MarketCoverageOK, error) {
	if h.facets == nil {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	skills := nonEmptyStrings(req.Skills.Value)
	if len(skills) == 0 {
		return nil, fiber.NewError(fiber.StatusBadRequest, "skills is required")
	}
	if len(skills) > maxCoverageSkills {
		return nil, fiber.NewError(fiber.StatusBadRequest, "too many skills")
	}
	query := contractQuery(ctx)

	// A flat list has no CV declared/body sections, so declared and all are the
	// same list and body is empty (statuses collapse to strong/adjacent/missing).
	v, err := h.coverageFor(ctx, marketFilter(query), skills, skills, nil, skills)
	if err != nil {
		return nil, err
	}
	// coherence is a CV-section metric (declared ∩ body); a flat skill list has no
	// sections, so it is meaningless here — zero it rather than advertise a score.
	v.CoherencePercent = 0
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	out := &apigen.MarketCoverageOK{Data: raw}
	ignored, values := contractIgnoredParams(coverageIgnoredParams(query)), contractIgnoredValues(ignoredJobValues(query))
	if ignored != nil || values != nil {
		out.Meta = apigen.NewOptIgnoredMeta(apigen.IgnoredMeta{IgnoredParams: ignored, IgnoredValues: values})
	}
	return out, nil
}

// coverageIgnoredParams reports the query params this endpoint did not filter on.
//
// The skills facet is reported rather than excused: the measured skills arrive
// in the body, so marketFilter drops them via stripSkillParams — but a caller who
// wrote `?skills=rust` still wrote a filter that was thrown away, and the score
// comes back looking equally confident. search.UnknownParams cannot name them
// (the facet is real vocabulary, just not vocabulary this endpoint reads), so
// they are collected here and folded into the one sorted, bounded report.
func coverageIgnoredParams(query url.Values) []search.UnknownParam {
	vals := maps.Clone(query)

	var out []search.UnknownParam
	for _, param := range skillParams {
		if vals.Has(param) {
			out = append(out, search.UnknownParam{Param: param})
			delete(vals, param)
		}
	}
	return search.SortAndCap(append(out, search.UnknownParams(vals, nil)...))
}

// marketFilter builds the market filter from the request's facet query params
// (the full facet vocabulary), with the skills facet stripped (see stripSkillParams).
func marketFilter(query url.Values) any {
	vals := maps.Clone(query)
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
