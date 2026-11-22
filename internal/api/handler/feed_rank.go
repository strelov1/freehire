package handler

import (
	"context"
	"strconv"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/jevrank"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/search/feed"
	"github.com/strelov1/freehire/internal/search/search"
)

// feedCVs reads the caller's default base CV. *cv.Store in production.
type feedCVs interface {
	BaseCV(ctx context.Context, userID int64) (cv.Record, bool, error)
}

// newFeedReranker is nil without a Jev key, which turns the re-rank off.
func newFeedReranker(key string, c cache.Cache) *feed.Reranker {
	if key == "" {
		return nil
	}
	return feed.NewReranker(jevrank.New(key), c, feed.DefaultBudget, feed.DefaultTTL)
}

// rerankCards reorders cards (same order as hits) by Jev fit and reports meta.ranking.
// Any failure keeps the skill order (AD-11); the cards never change, only their order.
func (h *feedHandlers) rerankCards(ctx context.Context, userID int64, hits []search.JobDocument, cards []feedCard) ([]feedCard, string) {
	if h.rank == nil || h.cvs == nil || len(cards) == 0 {
		return cards, "skills"
	}
	rec, ok, err := h.cvs.BaseCV(ctx, userID)
	if err != nil {
		feed.CountFallback(feed.FallbackError)
		return cards, "skills"
	}
	if !ok {
		return cards, "skills"
	}
	jobs := make([]jevrank.Job, len(hits))
	for i, hit := range hits {
		jobs[i] = jevrank.Job{
			Slug: hit.PublicSlug, Title: hit.Title, Company: hit.Company,
			Seniority: hit.Enrichment.Seniority, Description: hit.Description, Skills: hit.Skills,
		}
	}
	version := rec.ID.String() + "-" + strconv.FormatInt(rec.UpdatedAt.UnixNano(), 10)
	order, fallback := h.rank.Rerank(ctx, userID, version, jevrank.CVText(rec.Document), jobs)
	if fallback != "" {
		return cards, "skills"
	}
	bySlug := make(map[string]feedCard, len(cards))
	for _, c := range cards {
		bySlug[c.PublicSlug] = c
	}
	if len(bySlug) != len(cards) { // duplicate slugs: a reorder by slug would lose a card
		return cards, "skills"
	}
	out := make([]feedCard, 0, len(cards))
	for _, slug := range order {
		out = append(out, bySlug[slug])
	}
	return out, "fit"
}
