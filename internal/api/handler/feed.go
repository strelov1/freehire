package handler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/strelov1/freehire/internal/candidate/jobmatch"
	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/search/feed"
	"github.com/strelov1/freehire/internal/search/search"
)

// Reasons the feed returns no cards instead of searching the whole index (AD-3).
const feedReasonNoProfile = "no_profile"

var feedDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "freehire_me_feed_duration_seconds",
	Help:    "GET /me/feed latency by outcome (ok, empty, error).",
	Buckets: prometheus.DefBuckets,
}, []string{"outcome"})

// excludedJobs is the tracking read the feed skips saved and dismissed jobs by, the same
// set the swipe deck excludes.
type excludedJobs interface {
	ExcludedJobIDs(ctx context.Context, userID int64) ([]int64, error)
}

// feedHandlers serves the match feed: the caller's profile turned into a search
// (feed.ProfileQuery), never relaxed, each card carrying a deterministic match block (AD-4).
// Registered only when ME_FEED_ENABLED is set.
type feedHandlers struct {
	search   searcher
	profiles profileReader
	excluded excludedJobs
}

func (h *feedHandlers) register(api fiber.Router, mw middleware) {
	// Hand-written like the swipe deck: it reads the whole search vocabulary.
	api.Get("/me/feed", mw.key, h.Feed)
}

// feedMatch is a card's explanation: why it fits, what may not, what the job does not say.
type feedMatch struct {
	Why       []string `json:"why"`
	MayNotFit []string `json:"may_not_fit"`
	Unknown   []string `json:"unknown"`
}

type feedCard struct {
	jobview.Job
	Match feedMatch `json:"match"`
}

type feedMeta struct {
	Source string `json:"source"`
	Total  int64  `json:"total"`
	Reason string `json:"reason,omitempty"`
	// Degraded is "no_match_vector" when the profile has no skills, so cards come in the
	// search's default order rather than by match.
	Degraded string `json:"degraded,omitempty"`
}

// Feed returns one page of jobs matching the caller's profile, best match first.
func (h *feedHandlers) Feed(c *fiber.Ctx) error {
	start := time.Now()
	outcome := "error"
	defer func() { feedDuration.WithLabelValues(outcome).Observe(time.Since(start).Seconds()) }()

	if h.search == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	limit, _, err := pageParams(c)
	if err != nil {
		return err
	}

	empty := func(reason string) error {
		outcome = "empty"
		return c.JSON(fiber.Map{"data": []feedCard{}, "meta": feedMeta{Source: "profile", Reason: reason}})
	}
	profile, err := h.profiles.Get(c.Context(), userID)
	if errors.Is(err, userprofile.ErrNotFound) {
		return empty(feedReasonNoProfile)
	}
	if err != nil {
		return err
	}
	vals, reason := feed.ProfileQuery(profile)
	if reason != "" {
		return empty(reason)
	}
	excluded, err := h.excluded.ExcludedJobIDs(c.Context(), userID)
	if err != nil {
		return err
	}

	params := search.SearchParams{
		Filter: withExcludedIDs(search.FilterFromValues(vals), excluded),
		Limit:  limit,
	}
	meta := feedMeta{Source: "profile"}
	if len(profile.Skills) > 0 {
		// ProfileVector, never JobVector: the ballast belongs to the stored side only.
		params.Vector = skillvec.ProfileVector(profile.Skills)
	} else {
		meta.Degraded = "no_match_vector"
	}
	res, err := h.search.Search(c.Context(), params)
	if err != nil {
		return err
	}

	cards := make([]feedCard, len(res.Hits))
	for i, hit := range res.Hits {
		cards[i] = feedCard{Job: hit.Job, Match: matchBlock(hit.Job, profile.Skills)}
	}
	meta.Total = res.Total
	outcome = "ok"
	return c.JSON(fiber.Map{"data": cards, "meta": meta})
}

// matchBlock explains a card from the job and the profile alone, no model (AD-4). A
// condition the job does not state goes under unknown, never shown as met.
func matchBlock(j jobview.Job, profileSkills []string) feedMatch {
	m := jobmatch.Compute(j.Skills, profileSkills)
	out := feedMatch{Why: []string{}, MayNotFit: []string{}, Unknown: []string{}}
	out.Why = append(out.Why, m.Matched...)
	for _, a := range m.Adjacent {
		out.Why = append(out.Why, fmt.Sprintf("%s (via %s)", a.Name, a.Via))
	}
	out.MayNotFit = append(out.MayNotFit, m.Missing...)
	if j.Enrichment.SalaryMin == nil && j.Enrichment.SalaryMax == nil {
		out.Unknown = append(out.Unknown, "salary")
	}
	if j.WorkMode == "" {
		out.Unknown = append(out.Unknown, "work_mode")
	}
	if j.Enrichment.Seniority == "" {
		out.Unknown = append(out.Unknown, "seniority")
	}
	return out
}
