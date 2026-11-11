package handler

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/search/search"
)

// minSkillMatchParam is the job list's "Minimum skill match": keep only postings whose
// percent on the card (skillvec.MatchPercent) reaches it. It is no index facet — the
// percent depends on the reader, and a Meilisearch filter cannot express a share of
// matched skills — so search.FilterFromValues does not read it and a saved search
// carrying it narrows nothing.
const minSkillMatchParam = "min_skill_match"

const (
	// minMatchScanBatch is how many postings one index read takes. A posting is a heavy
	// document (a description preview), so a read is kept small enough to answer quickly.
	minMatchScanBatch = 250
	// minMatchScanDepth bounds how far into the ranking one request reads. It is a
	// conservative default, not a measurement: how deep a typical threshold must go on
	// production data is still to be measured. It stays well inside maxPageWindow, the
	// deepest offset the index is asked for.
	minMatchScanDepth = 2000
)

// skillMatchFilter is a minimum skill match in force for one request: the threshold and
// the profile skills the percent is counted against.
type skillMatchFilter struct {
	percent int
	held    []string
}

// skillMatchFilterFor honours the param for a signed-in caller with recognised skills
// under sort=match only. Everything else falls back to the unfiltered search and says so
// rather than refusing a shared link: under another ordering the matching postings are
// scattered through the catalogue and the read could not stop early, so the browser keeps
// trimming the page it holds there.
func (h *searchHandlers) skillMatchFilterFor(ctx context.Context, userID int64, vals url.Values) (*skillMatchFilter, bool) {
	if !vals.Has(minSkillMatchParam) {
		return nil, false
	}
	percent, err := strconv.Atoi(strings.TrimSpace(vals.Get(minSkillMatchParam)))
	if err != nil || percent < 1 || percent > 100 {
		return nil, true
	}
	vector, held := h.matchProfileFor(ctx, userID, vals)
	if vector == nil {
		return nil, true
	}
	return &skillMatchFilter{percent: percent, held: held}, false
}

// scan reads the match ranking from the top and keeps the postings at or above the
// threshold, until it holds the requested page and one posting more, or the ranking can
// no longer produce one, or the depth bound is reached. p carries the request's filter,
// vector and window; its Offset addresses the FILTERED stream, so a deep page rescans
// from the top.
//
// The ranking is by cosine, which follows coverage because of the ballast
// (internal/dict/skillvec). Once a posting scores below what a posting of six skills or
// more at the threshold would, no later one qualifies and the read stops. A posting of
// fewer skills scores lower for the same percent and may be missed; that is the price of
// stopping at all.
//
// The total is what the reader has been shown plus one when more exists, and a lower
// bound unless the whole ranking was read — the stop rule and the depth bound both
// leave postings unread.
func (f *skillMatchFilter) scan(ctx context.Context, idx searcher, p search.SearchParams) (search.SearchResult, error) {
	want := p.Offset + p.Limit + 1
	floor := skillvec.MinCosineAtPercent(p.Vector, f.percent)
	kept := make([]search.JobDocument, 0, want)
	exhausted := false
	read := 0
read:
	for read < minMatchScanDepth && len(kept) < want {
		batch := p
		batch.Offset = read
		batch.Limit = min(minMatchScanBatch, minMatchScanDepth-read)
		res, err := idx.Search(ctx, batch)
		if err != nil {
			return search.SearchResult{}, err
		}
		for _, hit := range res.Hits {
			if skillvec.Similarity(p.Vector, hit.Skills) < floor {
				break read
			}
			if skillvec.MatchPercent(hit.Skills, f.held) >= f.percent {
				kept = append(kept, hit)
				if len(kept) == want {
					break read
				}
			}
		}
		read += len(res.Hits)
		if len(res.Hits) < batch.Limit {
			exhausted = true
			break
		}
	}

	if len(kept) > p.Offset+p.Limit {
		return search.SearchResult{
			Hits:              kept[p.Offset : p.Offset+p.Limit],
			Total:             int64(p.Offset + p.Limit + 1),
			TotalIsLowerBound: true,
		}, nil
	}
	var page []search.JobDocument
	if p.Offset < len(kept) {
		page = kept[p.Offset:]
	}
	return search.SearchResult{Hits: page, Total: int64(len(kept)), TotalIsLowerBound: !exhausted}, nil
}
