package sources

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// startupjobsIndexFromHome reads the Algolia application, search key and posting index the
// site's own search uses from the homepage's <meta name="current-algolia-..."> tags. A missing
// tag is a markup change and must fail loudly, never fall back to a remembered key.
func startupjobsIndexFromHome(root *html.Node) (startupjobsIndex, error) {
	idx := startupjobsIndex{
		AppID:  metaName(root, "current-algolia-application-id"),
		APIKey: metaName(root, "current-algolia-api-key-search"),
		Index:  metaName(root, "current-algolia-index-post"),
	}
	if idx.AppID == "" || idx.APIKey == "" || idx.Index == "" {
		return startupjobsIndex{}, fmt.Errorf("startupjobs: homepage names no Algolia index (app %q, key set %v, index %q)",
			idx.AppID, idx.APIKey != "", idx.Index)
	}
	// The app id becomes a hostname and the index a path segment, so neither is trusted to be
	// anything but the plain identifier Algolia issues.
	if !startupjobsIdentifier.MatchString(idx.AppID) || !startupjobsIdentifier.MatchString(idx.Index) {
		return startupjobsIndex{}, fmt.Errorf("startupjobs: homepage names an implausible Algolia index (app %q, index %q)", idx.AppID, idx.Index)
	}
	return idx, nil
}

// metaName returns the content of the first <meta name="..."> with that name, or "".
func metaName(root *html.Node, name string) string {
	var found string
	walk(root, func(n *html.Node) bool {
		if found != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "meta" && Attr(n, "name") == name {
			found = Attr(n, "content")
			return false
		}
		return true
	})
	return found
}

// sitemap reads every posting the sitemap lists and when the sitemap was generated — the
// OLDEST of its posting files' lastmod, so the window reaches back past the first file
// written. Any file failing fails the run: a missing file would read as a sixth of the
// catalogue gone.
func (s startupjobs) sitemap(ctx context.Context) ([]startupjobsSitemapPosting, time.Time, error) {
	index, err := getSitemap(ctx, s.feed, startupjobsSitemapIndex)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("startupjobs: sitemap index: %w", err)
	}
	var (
		out       []startupjobsSitemapPosting
		seen      = make(map[string]bool)
		generated time.Time
		files     int
	)
	for _, sm := range index.Sitemaps {
		if !startupjobsIsPostingSitemap(sm.Loc) {
			continue
		}
		files++
		mod := parseRFC3339(sm.LastMod)
		if mod == nil {
			return nil, time.Time{}, fmt.Errorf("startupjobs: sitemap %s states no lastmod", sm.Loc)
		}
		if generated.IsZero() || mod.Before(generated) {
			generated = *mod
		}
		doc, err := getSitemap(ctx, s.feed, sm.Loc)
		if err != nil {
			return nil, time.Time{}, fmt.Errorf("startupjobs: sitemap %s: %w", sm.Loc, err)
		}
		for _, u := range doc.URLs {
			id := startupjobsPostingID(u.Loc)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, startupjobsSitemapPosting{ID: id, URL: u.Loc})
		}
	}
	if files == 0 || len(out) == 0 {
		return nil, time.Time{}, fmt.Errorf("startupjobs: sitemap index lists %d posting files carrying %d postings", files, len(out))
	}
	return out, generated, nil
}

// walk reads every posting created at or after since, in slices of created_at_i, newest
// first.
//
// created_at_i, not published_at_i, is the axis: published_at_i is re-stamped in daily batches
// at midnight UTC, so a one-second slice of it can hold far more than the 500 a query may read,
// and nothing narrower than a second exists. created_at_i is when the row was created and never
// moves, so a posting cannot migrate along it mid-walk.
//
// Each slice is [lower, upper); the first is open above, so a posting created during the walk
// is either read or created after the walk began. A slice whose nbHits exceeds the window is
// re-asked narrower; one that cannot be narrowed — more than 500 postings created in one
// second — fails the crawl, because reading its first 500 and calling it complete is exactly
// the silent truncation a source-scoped sweep turns into mass closure.
func (s startupjobs) walk(ctx context.Context, idx startupjobsIndex, since, now int64, budget *startupjobsBudget) ([]startupjobsHit, error) {
	var (
		out   []startupjobsHit
		seen  = make(map[string]bool)
		upper int64 // exclusive; 0 = open
		width = max(int64(1), (now-since)/4)
	)
	for {
		hi := now
		if upper != 0 {
			hi = upper
		}
		lower := max(hi-width, since)
		filters := []string{"created_at_i>=" + strconv.FormatInt(lower, 10)}
		if upper != 0 {
			filters = append(filters, "created_at_i<"+strconv.FormatInt(upper, 10))
		}

		first, err := s.page(ctx, idx, filters, 0, budget)
		if err != nil {
			return nil, err
		}
		if first.NbHits > startupjobsWindow {
			if hi-lower <= 1 {
				return nil, fmt.Errorf("startupjobs: %d postings created in the second at %d; Algolia reads at most %d per query and created_at_i cannot be narrowed further",
					first.NbHits, lower, startupjobsWindow)
			}
			width = narrowedWidth(hi-lower, first.NbHits)
			continue
		}
		hits, err := s.readSlice(ctx, idx, filters, first, budget)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			if !seen[h.ObjectID] {
				seen[h.ObjectID] = true
				out = append(out, h)
			}
		}
		if lower <= since {
			return out, nil
		}
		upper = lower
		width = widenedWidth(hi-lower, first.NbHits)
	}
}

// readSlice reads every page of a slice that fits the window and proves it read all of it:
// the count must be exhaustive and the distinct hits must equal it. A slice that changed
// between its pages (a posting removed or added mid-read) is re-read once from the start
// before the mismatch is allowed to fail the crawl.
func (s startupjobs) readSlice(ctx context.Context, idx startupjobsIndex, filters []string,
	first startupjobsResult, budget *startupjobsBudget) ([]startupjobsHit, error) {
	for attempt := 0; ; attempt++ {
		if !first.ExhaustiveNbHits {
			return nil, fmt.Errorf("startupjobs: slice %v: Algolia reported an estimated count (%d), which cannot prove the slice was read whole",
				filters, first.NbHits)
		}
		var hits []startupjobsHit
		read := make(map[string]bool, first.NbHits)
		add := func(page []startupjobsHit) {
			for _, h := range page {
				if !read[h.ObjectID] {
					read[h.ObjectID] = true
					hits = append(hits, h)
				}
			}
		}
		add(first.Hits)
		for page := 1; page*startupjobsPageSize < first.NbHits; page++ {
			res, err := s.page(ctx, idx, filters, page, budget)
			if err != nil {
				return nil, err
			}
			add(res.Hits)
		}
		if len(hits) == first.NbHits {
			return hits, nil
		}
		if attempt == 1 {
			return nil, fmt.Errorf("startupjobs: slice %v: read %d distinct postings of the %d Algolia counted",
				filters, len(hits), first.NbHits)
		}
		var err error
		if first, err = s.page(ctx, idx, filters, 0, budget); err != nil {
			return nil, err
		}
		if first.NbHits > startupjobsWindow {
			return nil, fmt.Errorf("startupjobs: slice %v grew past the %d window while being read (%d)",
				filters, startupjobsWindow, first.NbHits)
		}
	}
}

// lookupBacklog reads the index metadata of the listed postings the catalogue has never
// stored and the window did not cover, as far as the budget the window left allows.
//
// The backlog is mostly postings judged before and discarded — non-technical, or from an
// employer crawled first-party — which are never stored, so they come back every run. Reading
// it newest-first would spend every run on the same freshly-discarded top and never reach the
// rest, so the run reads one chunk of it chosen at RANDOM. Not by the clock: the provider runs
// on a fixed cadence, and a chunk named by the hour would visit only every other chunk at a
// two-hour cadence whenever the chunk count is even. A new technical posting that fell through
// a gap in the window is found within a few cycles of len/chunk runs.
// An id the index no longer knows was removed since the sitemap was written, and is skipped.
func (s startupjobs) lookupBacklog(ctx context.Context, idx startupjobsIndex, backlog []string,
	budget *startupjobsBudget) ([]startupjobsHit, error) {
	queries := min(budget.left, startupjobsMaxBacklogQueries)
	if len(backlog) == 0 || queries <= 0 {
		return nil, nil
	}
	slices.SortFunc(backlog, func(a, b string) int { return startupjobsCompareIDs(b, a) })
	chunk := queries * startupjobsPageSize
	chunks := (len(backlog) + chunk - 1) / chunk
	start := s.pick(chunks) * chunk
	ids := backlog[start:min(start+chunk, len(backlog))]

	var out []startupjobsHit
	for len(ids) > 0 {
		n := min(len(ids), startupjobsPageSize)
		clauses := make([]string, n)
		for i, id := range ids[:n] {
			clauses[i] = "objectID:" + id
		}
		res, err := s.query(ctx, idx, startupjobsQuery{
			HitsPerPage:          startupjobsPageSize,
			Filters:              strings.Join(clauses, " OR "),
			AttributesToRetrieve: startupjobsHitAttributes,
		}, budget)
		if err != nil {
			return nil, err
		}
		out = append(out, res.Hits...)
		ids = ids[n:]
	}
	return out, nil
}

// page asks Algolia for one page of one walk slice.
func (s startupjobs) page(ctx context.Context, idx startupjobsIndex, filters []string, page int, budget *startupjobsBudget) (startupjobsResult, error) {
	return s.query(ctx, idx, startupjobsQuery{
		HitsPerPage:          startupjobsPageSize,
		Page:                 page,
		NumericFilters:       filters,
		AttributesToRetrieve: startupjobsHitAttributes,
	}, budget)
}
