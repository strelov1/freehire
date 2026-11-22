package handler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/strelov1/freehire/internal/platform/pgconv"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/jobmatch"
	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/feed"
	"github.com/strelov1/freehire/internal/search/search"
)

// Reasons the feed returns no cards instead of searching the whole index (AD-3).
const (
	feedReasonNoProfile     = "no_profile"
	feedReasonSourceChanged = "source_changed"
)

var feedDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "freehire_me_feed_duration_seconds",
	Help:    "GET /me/feed latency by outcome (ok, empty, payment_required, error).",
	Buckets: prometheus.DefBuckets,
}, []string{"outcome"})

// excludedJobs is the tracking read the feed skips jobs by: those saved, dismissed or
// applied to before the paging snapshot (AD-7).
type excludedJobs interface {
	FeedExcludedJobIDs(ctx context.Context, userID int64, before time.Time) ([]int64, error)
}

// feedState holds the per-source "seen" watermark and the search drain high-water.
// feedStateDB in production.
type feedState interface {
	// GetOrInitFeedSeen returns the watermark, storing initAt on the first read.
	GetOrInitFeedSeen(ctx context.Context, userID int64, source string, initAt time.Time) (time.Time, error)
	// AdvanceFeedSeen moves the watermark forward only.
	AdvanceFeedSeen(ctx context.Context, userID int64, source string, at time.Time) error
	// SearchDrainHighWater is the oldest recent open job not yet indexed; false when none.
	SearchDrainHighWater(ctx context.Context) (time.Time, bool, error)
}

type feedStateDB struct{ q *db.Queries }

func (s feedStateDB) GetOrInitFeedSeen(ctx context.Context, userID int64, source string, initAt time.Time) (time.Time, error) {
	v, err := s.q.GetOrInitFeedSeen(ctx, db.GetOrInitFeedSeenParams{UserID: userID, Source: source, InitAt: pgconv.Instant(initAt)})
	return v.Time, err
}

func (s feedStateDB) AdvanceFeedSeen(ctx context.Context, userID int64, source string, at time.Time) error {
	return s.q.AdvanceFeedSeen(ctx, db.AdvanceFeedSeenParams{UserID: userID, Source: source, At: pgconv.Instant(at)})
}

func (s feedStateDB) SearchDrainHighWater(ctx context.Context) (time.Time, bool, error) {
	v, err := s.q.SearchDrainHighWater(ctx)
	return v.Time, v.Valid, err
}

// feedPlans is the plan check the feed is gated on. *plan.Store in production.
type feedPlans interface {
	RequirePaid(ctx context.Context, userID int64) (plan.Tier, error)
}

// feedHandlers serves the match feed: the caller's profile turned into a search
// (feed.ProfileQuery), never relaxed, each card carrying a deterministic match block (AD-4).
// Registered only when ME_FEED_ENABLED is set.
type feedHandlers struct {
	search   searcher
	profiles profileReader
	excluded excludedJobs
	plans    feedPlans
	rank     *feed.Reranker
	cvs      feedCVs
	cursors  *feed.Signer
	state    feedState
}

// newFeedHandlers is nil with the flag off, so the route is never registered.
func newFeedHandlers(on bool, s searcher, p profileReader, e excludedJobs, pl feedPlans, cvs feedCVs, st feedState, jevKey string, c cache.Cache, cursorKey []byte) *feedHandlers {
	if !on {
		return nil
	}
	return &feedHandlers{search: s, profiles: p, excluded: e, plans: pl, rank: newFeedReranker(jevKey, c), cvs: cvs, state: st, cursors: feed.NewSigner(cursorKey)}
}

func (h *feedHandlers) register(api fiber.Router, mw middleware) {
	// Hand-written like the swipe deck: it reads the whole search vocabulary.
	api.Get("/me/feed", mw.key, h.Feed)
	api.Post("/me/feed/seen", mw.key, h.Seen)
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
	// IsNew: the card arrived after the user's last pass through this source's new cards.
	IsNew bool `json:"is_new"`
}

type feedMeta struct {
	Source string `json:"source"`
	Total  int64  `json:"total"`
	Reason string `json:"reason,omitempty"`
	// Degraded is "no_match_vector" when the profile has no skills, so cards come in the
	// search's default order rather than by match.
	Degraded string `json:"degraded,omitempty"`
	// Ranking is how the page is ordered: fit (Jev), skills (vector) or date (no vector). Open enum.
	Ranking string `json:"ranking,omitempty"`
	// NextCursor pages on; absent on the last page. Opaque: clients never parse it (AD-7).
	NextCursor string `json:"next_cursor,omitempty"`
	// Phase is the phase of the page's first card: new or rest. Open enum.
	Phase string `json:"phase,omitempty"`
	// SeenCursor rides on the page where the new phase ends. The client posts it to
	// POST /me/feed/seen once the candidate passes the last new card.
	SeenCursor string `json:"seen_cursor,omitempty"`
}

const (
	feedPhaseNew  = "new"
	feedPhaseRest = "rest"
)

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
	// A paid feature (AD-10): judged before any profile or search read.
	if _, err := h.plans.RequirePaid(c.Context(), userID); errors.Is(err, plan.ErrPaymentRequired) {
		outcome = "payment_required"
		return fiber.NewError(fiber.StatusPaymentRequired, "The match feed needs Pro or Ultra")
	} else if err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Cannot verify your plan right now")
	}
	limit, _ := pageParamsBounded(c, defaultLimit, maxLimit) // paging is by cursor, never offset
	cur, err := h.pageCursor(c, userID)
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
	version := profileVersion(profile)
	if c.Query("cursor") != "" && cur.Version != version {
		return empty(feedReasonSourceChanged)
	}
	vals, reason := feed.ProfileQuery(profile)
	if reason != "" {
		return empty(reason)
	}
	excluded, err := h.excluded.FeedExcludedJobIDs(c.Context(), userID, time.Unix(cur.ActedBefore, 0))
	if err != nil {
		return err
	}

	meta := feedMeta{Source: "profile"}
	var vector []float32
	if len(profile.Skills) > 0 {
		// ProfileVector, never JobVector: the ballast belongs to the stored side only.
		vector = skillvec.ProfileVector(profile.Skills)
	} else {
		meta.Degraded = "no_match_vector"
	}
	// Jobs indexed after the snapshot never shift a window; they appear after a restart.
	// New is (seen, snapshot], rest is everything at or before seen: the two never overlap.
	query := func(phase string, limit, offset int) (search.SearchResult, error) {
		f := withExcludedIDs(search.FilterFromValues(vals), excluded)
		if phase == feed.PhaseNew {
			f = withFilterGroup(withFilterGroup(f, search.Gte("created_ts", int(cur.SeenAt)+1)), search.Lte("created_ts", int(cur.SnapshotAt)))
		} else {
			f = withFilterGroup(f, search.Lte("created_ts", int(cur.SeenAt))) // pageCursor keeps SeenAt <= SnapshotAt)
		}
		return h.search.Search(c.Context(), search.SearchParams{Filter: f, Vector: vector, Limit: limit, Offset: offset})
	}
	// segment ranks one phase's hits on their own: phases never mix inside a reorder.
	segment := func(hits []search.JobDocument, isNew bool) ([]feedCard, string) {
		cards := make([]feedCard, len(hits))
		for i, hit := range hits {
			cards[i] = feedCard{Job: hit.Job, Match: matchBlock(hit.Job, profile.Skills), IsNew: isNew}
		}
		if meta.Degraded != "" {
			return cards, "date"
		}
		return h.rerankCards(c.Context(), userID, hits, cards)
	}

	phase := cur.Phase // pageCursor admits only new or rest
	res, err := query(phase, limit, cur.Offset)
	if err != nil {
		return err
	}
	cards, ranking := segment(res.Hits, phase == feed.PhaseNew)
	meta.Ranking, meta.Total = ranking, res.Total
	next := cur
	next.Phase, next.Version = phase, version
	if phase == feed.PhaseNew && (len(res.Hits) < limit || cur.Offset+len(res.Hits) >= int(res.Total)) {
		// The new phase ends here: the rest of the page comes from the rest phase, offset 0.
		fill := limit - len(res.Hits)
		rest, err := query(feed.PhaseRest, max(fill, 1), 0) // fill 0 still probes for a rest phase
		if err != nil {
			return err
		}
		meta.Total += rest.Total
		got := 0
		if fill > 0 {
			restCards, rk := segment(rest.Hits, false)
			cards, got = append(cards, restCards...), len(restCards)
			if len(res.Hits) == 0 {
				meta.Ranking = rk
			}
		}
		seen := next
		seen.Phase, seen.Offset = feed.PhaseSeen, 0
		meta.SeenCursor = h.cursors.Sign(seen)
		next.Phase, next.Offset = feed.PhaseRest, 0
		if fill == 0 {
			if rest.Total > 0 {
				meta.NextCursor = h.cursors.Sign(next)
			}
		} else {
			meta.NextCursor = h.nextCursor(next, version, fill, got, rest.Total)
		}
	} else {
		meta.NextCursor = h.nextCursor(next, version, limit, len(res.Hits), res.Total)
	}
	meta.Phase = feedPhaseRest
	if len(cards) > 0 && cards[0].IsNew {
		meta.Phase = feedPhaseNew
	}
	outcome = "ok"
	return c.JSON(fiber.Map{"data": cards, "meta": meta})
}

// Seen moves the source's watermark to the snapshot of the seen_cursor a feed page handed
// out, once the client has passed the last new card. It never moves back.
func (h *feedHandlers) Seen(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if _, err := h.plans.RequirePaid(c.Context(), userID); errors.Is(err, plan.ErrPaymentRequired) {
		return fiber.NewError(fiber.StatusPaymentRequired, "The match feed needs Pro or Ultra")
	} else if err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Cannot verify your plan right now")
	}
	var in struct {
		Cursor string `json:"cursor"`
	}
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	cur, err := h.verifyCursor(in.Cursor, userID)
	if err != nil || cur.Phase != feed.PhaseSeen {
		return fiber.NewError(fiber.StatusBadRequest, "invalid cursor")
	}
	if err := h.state.AdvanceFeedSeen(c.Context(), userID, cur.Source, time.Unix(cur.SnapshotAt, 0)); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
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
