package handler

import (
	"context"
	"log"
	"net/url"
	"slices"
	"time"

	"github.com/strelov1/freehire/internal/engage/sponsored"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// sponsoredSlots is how many paid hits one page carries at most.
const sponsoredSlots = 2

// sponsoredSearchSlots are the final indexes the paid hits take in the page: the first and the
// sixth card in the feed, or both pinned at the top of a company page.
var (
	sponsoredSearchSlots  = []int{0, 5}
	sponsoredCompanySlots = []int{0, 1}
)

// sponsoredAds is what the sponsored lookup placed in a page: job id to campaign id, and the
// placement the click tokens must name. The zero value places nothing.
type sponsoredAds struct {
	campaigns map[int64]int64
	placement string
}

// sponsoredEnabled reports whether paid placements are on at all: no click key means no tokens
// can be signed, so none are served.
func (h *searchHandlers) sponsoredEnabled() bool {
	return len(h.sponsoredKey) > 0 && h.serving != nil && h.search != nil
}

// sponsoredPlacement returns the placement of a search request, or "" when it gets no paid
// slot: the unfiltered feed never does (product decision), nor do later pages.
func sponsoredPlacement(vals url.Values, offset int) string {
	if offset != 0 || !search.Narrows(vals) {
		return ""
	}
	if slugs := vals["company_slug"]; len(slugs) == 1 && slugs[0] != "" {
		return sponsored.PlacementCompany
	}
	return sponsored.PlacementSearch
}

// startSponsored runs the sponsored twin of the organic search in parallel with it. The
// channel is buffered, so a caller that never reads it (the organic search failed) leaks nothing.
func (h *searchHandlers) startSponsored(ctx context.Context, params search.SearchParams) <-chan []sponsored.Pick {
	out := make(chan []sponsored.Pick, 1)
	go func() {
		picks, err := sponsored.Find(ctx, h.search, h.serving, params, sponsoredSlots, time.Now())
		if err != nil {
			// The organic answer is never failed for an ad.
			log.Printf("search: sponsored lookup: %v", err)
			picks = nil
		}
		out <- picks
	}()
	return out
}

// placeSponsored puts the picks into the organic page at their slots and drops the organic
// copy of a promoted job. The page may end up up to sponsoredSlots longer than the limit
// rather than push an organic hit onto no page at all.
func placeSponsored(hits []search.JobDocument, picks []sponsored.Pick, slots []int) []search.JobDocument {
	promoted := make(map[int64]bool, len(picks))
	for _, p := range picks {
		promoted[p.Doc.ID] = true
	}
	out := make([]search.JobDocument, 0, len(hits)+len(picks))
	for _, h := range hits {
		if !promoted[h.ID] {
			out = append(out, h)
		}
	}
	for i, p := range picks {
		at := min(slots[i], len(out))
		out = slices.Insert(out, at, p.Doc)
	}
	return out
}

// markSponsored flags the views of the paid hits and points them at the click redirect. The
// hits are index documents, so every other view keeps jobview.Job.Sponsored false.
func (h *searchHandlers) markSponsored(hits []search.JobDocument, views []jobview.Job, ads sponsoredAds) {
	now := time.Now()
	for i, hit := range hits {
		campaign, ok := ads.campaigns[hit.ID]
		if !ok {
			continue
		}
		token := sponsored.SignToken(h.sponsoredKey, sponsored.Claims{CampaignID: campaign, JobID: hit.ID, Placement: ads.placement}, now)
		views[i].Sponsored = true
		views[i].SponsoredURL = sponsored.ClickPath + token
	}
}

// enableSponsored wires the paid placements. They stay off unless the click key is set (no key,
// no signable token) and a database is behind the handler.
func (h *searchHandlers) enableSponsored(cfg Config, queries *db.Queries) {
	h.sponsoredKey, h.impressions = cfg.SponsoredClickKey, cfg.SponsoredImpressions
	if queries != nil {
		h.serving = queries
	}
}
