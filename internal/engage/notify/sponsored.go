package notify

import (
	"context"
	"log"
	"net/url"
	"time"

	"github.com/strelov1/freehire/internal/engage/sponsored"
	"github.com/strelov1/freehire/internal/search/search"
)

// sponsoredSearchTimeout bounds the extra search an email digest makes: an ad must never hold
// a delivery.
const sponsoredSearchTimeout = 5 * time.Second

// adConfig switches paid slots on. The zero value is off.
type adConfig struct {
	serving search.ServingReader
	key     []byte
	imp     *sponsored.Impressions
}

// WithSponsored turns on the paid job in email digests: one sponsored hit for the saved
// search's own query, linked through a click token signed with key. An empty key leaves it off.
// imp may be nil; the caller flushes it after Run.
func (r *Runner) WithSponsored(serving search.ServingReader, key []byte, imp *sponsored.Impressions) *Runner {
	r.ads = adConfig{serving: serving, key: key, imp: imp}
	return r
}

// sponsoredSlot returns the paid job for an email digest, or nil: another channel, ads off, an
// alert that is not narrowed (the unfiltered feed gets no paid slot) or a watchlist alert (it
// needs its owner's companies), nothing matching, a pick the subscriber's skill filters would
// drop from the digest itself, or any failure. It never fails a delivery.
func (r *Runner) sponsoredSlot(ctx context.Context, channel string, userID int64, d Digest) *SponsoredSlot {
	if channel != ChannelEmail || len(r.ads.key) == 0 || r.ads.serving == nil {
		return nil
	}
	vals, _ := url.ParseQuery(d.SearchQuery)
	if !search.Narrows(vals) || search.WantsWatchlist(vals) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, sponsoredSearchTimeout)
	defer cancel()
	now := r.now()
	picks, err := sponsored.Find(ctx, r.searcher, r.ads.serving, search.SearchParams{
		Query: vals.Get("q"), Filter: search.FilterFromValues(vals),
	}, 1, now)
	if err != nil {
		log.Printf("notify: sponsored lookup for %q: %v", d.SavedSearchName, err)
		return nil
	}
	if len(picks) == 0 {
		return nil
	}
	p := picks[0]
	skills, err := r.store.ListUserProfileSkillSets(ctx, []int64{userID})
	if err != nil {
		log.Printf("notify: sponsored skill filters for user %d: %v", userID, err)
		return nil
	}
	for _, sk := range skills {
		if hasAvoidedSkill(p.Doc.Skills, sk.ExcludedSkills) || !meetsSkillMatch(p.Doc.Skills, sk.Skills, minSkillMatch(vals)) {
			return nil
		}
	}
	token := sponsored.SignToken(r.ads.key, sponsored.Claims{CampaignID: p.CampaignID, JobID: p.Doc.ID, Placement: sponsored.PlacementEmail}, now)
	return &SponsoredSlot{
		Job:        DigestJob{Title: p.Doc.Title, Company: p.Doc.Company, Slug: p.Doc.PublicSlug, URL: p.Doc.URL},
		Path:       sponsored.ClickPath + token,
		CampaignID: p.CampaignID,
	}
}
