package handler

import (
	"context"
	"net/url"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/job/jobview"
)

// onboardingMatchesLimit is how many postings the onboarding results screen shows. The
// skill list's bound (50) is the contract's: the generated server refuses a longer one.
const onboardingMatchesLimit = 10

// OnboardingMatches ranks open postings by how well their skills match the posted skill
// list, under the jobs-search filters in the body's query string. It is the match sort of
// /jobs/search for a caller with no stored profile — the CV-first onboarding's visitor —
// with the skills supplied instead of read from an account. It stores nothing and the
// answer is the same for anyone who sends the same body.
func (h *searchHandlers) OnboardingMatches(ctx context.Context, req *apigen.OnboardingMatchesRequest) (*apigen.OnboardingMatchesOK, error) {
	if h.search == nil {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}
	vals, err := url.ParseQuery(req.Query.Or(""))
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "query is not a valid query string")
	}
	// The ranking is the endpoint's own, never the query's: a sort or page in the query
	// would page past the top or reorder it away from the CV.
	for _, k := range []string{"sort", "order", "limit", "offset"} {
		vals.Del(k)
	}

	out, err := h.executeJobSearch(ctx, vals, jobSearchInput{
		limit:  onboardingMatchesLimit,
		vector: skillvec.ProfileVector(req.Skills),
	})
	if err != nil {
		return nil, err
	}
	res := out.res
	views := make([]jobview.Job, len(res.Hits))
	for i, hit := range res.Hits {
		views[i] = hit.Job
	}
	h.attachGhost(ctx, res.Hits, views)
	raws, err := marshalRaws(views)
	if err != nil {
		return nil, err
	}
	data := make([]apigen.OnboardingMatch, len(views))
	for i, v := range views {
		data[i] = apigen.OnboardingMatch{Job: raws[i], MatchPercent: int32(skillvec.MatchPercent(v.Skills, req.Skills))}
	}
	return &apigen.OnboardingMatchesOK{
		Data: data,
		Meta: apigen.ListMeta{
			Total: res.Total, Limit: onboardingMatchesLimit,
			IgnoredParams: contractIgnoredParams(mergedJobSearchIgnored(vals, jobSearchParams, out.dropped)),
			IgnoredValues: contractIgnoredValues(ignoredJobValues(vals)),
		},
	}, nil
}
