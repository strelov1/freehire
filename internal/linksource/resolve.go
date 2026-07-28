package linksource

import (
	"context"
	"fmt"
	"log"
	"net/url"

	"github.com/strelov1/freehire/internal/sources"
)

// Resolved is one vacancy parsed from a post link, tagged with the destination platform
// that produced it (which becomes jobs.source).
type Resolved struct {
	Source string
	Job    sources.Job
}

// ResolveLinks runs each url through the registry and returns the vacancies from every
// link a destination adapter both matched and resolved. A matched link that is not a
// single vacancy (ok=false) is skipped; a matched link whose fetch/parse errors is logged.
// err is non-nil only when matched links existed but all of them failed — a transient
// outcome the caller should retry. Links no adapter matches are ignored, yielding
// (nil, nil) so the caller can fall back to its own extraction.
func ResolveLinks(ctx context.Context, reg []Source, urls []string) ([]Resolved, error) {
	var out []Resolved
	var matched, failed int
	var firstErr error

	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		ls := Find(reg, u)
		if ls == nil {
			continue
		}
		matched++

		job, ok, err := ls.Resolve(ctx, raw)
		if err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			log.Printf("linksource: resolve %s (%s) failed: %v", raw, ls.Source(), err)
			continue
		}
		if !ok {
			continue // matched host but not a single vacancy — skip
		}
		out = append(out, Resolved{Source: sourceOf(ls, u), Job: job})
	}

	if len(out) == 0 && failed > 0 {
		return nil, fmt.Errorf("linksource: all %d resolvable link(s) failed: %w", failed, firstErr)
	}
	return out, nil
}

// sourceOf is the identity a resolved job is stored under: the adapter's own Source for a
// host-scoped adapter, or the platform this particular link belongs to for one that serves
// many (see PerLinkSource). An adapter that cannot name a platform for the link falls back to
// its own key rather than yielding an empty jobs.source.
func sourceOf(ls Source, u *url.URL) string {
	if per, ok := ls.(PerLinkSource); ok {
		if s := per.SourceFor(u); s != "" {
			return s
		}
	}
	return ls.Source()
}

// MatchesAny reports whether any url is a link a destination adapter handles. The crawl
// prefilter uses it so a link-out digest post is kept even when its teaser text alone does
// not look like a vacancy.
func MatchesAny(reg []Source, urls []string) bool {
	for _, raw := range urls {
		if u, err := url.Parse(raw); err == nil && Find(reg, u) != nil {
			return true
		}
	}
	return false
}
