package pipeline

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/sources"
)

// canonicalCompanySlug is the canonical slug for a posting's company, or "" when the registry
// has nothing to say about it — which job.Draft reads as "keep the derived slug".
func canonicalCompanySlug(canon companyCanonical, company string) string {
	if canon == nil {
		return ""
	}
	derived := normalize.CompanySlug(company)
	if canonical := canon(derived); canonical != derived {
		return canonical
	}
	return ""
}

// resolveCompanyAliases resolves a buffered board's distinct company slugs against the alias
// registry in ONE call, whatever the posting count.
//
// A failed lookup keeps every posting on its derived slug rather than failing the board. That
// is the recoverable direction: the worst case is a duplicate company the next merge wave
// collapses, where refusing the board loses the postings outright.
func (r Runner) resolveCompanyAliases(ctx context.Context, e sources.CompanyEntry, raw []sources.Job) companyCanonical {
	return r.resolveSlugAliases(ctx, e, distinctCompanySlugs(raw))
}

// resolveSlugAliases is resolveCompanyAliases over already-derived distinct slugs, so the
// callers with already-derived company slugs share the same batched lookup.
func (r Runner) resolveSlugAliases(ctx context.Context, e sources.CompanyEntry, slugs []string) companyCanonical {
	if r.Aliases == nil {
		return nil
	}
	if len(slugs) == 0 {
		return nil
	}
	keys := make([]string, len(slugs))
	for i, s := range slugs {
		keys[i] = normalize.FoldSlug(s)
	}
	canonByKey, err := r.Aliases.CanonicalCompanySlugs(ctx, keys)
	if err != nil {
		log.Printf("ingest: %s board %q (%s): company-alias lookup failed, postings keep their derived slugs: %v",
			e.Provider, e.Board, e.Company, err)
		return nil
	}
	if len(canonByKey) == 0 {
		return nil
	}
	return func(derived string) string {
		if canonical, ok := canonByKey[normalize.FoldSlug(derived)]; ok {
			return canonical
		}
		return derived
	}
}

// streamCompanyCanonical is resolveCompanyAliases for a streaming board, which has no upfront
// batch: it resolves one company as it is first seen and memoizes the answer. It is only
// called from ingestStream's emit callback, already
// serialized under that path's own mutex, so the cache needs no lock. fillCompanyDescription
// also builds one for the single slug it resolves, on its own goroutine.
func (r Runner) streamCompanyCanonical(ctx context.Context, e sources.CompanyEntry) companyCanonical {
	if r.Aliases == nil {
		return nil
	}
	cache := make(map[string]string)
	return func(derived string) string {
		if derived == "" {
			return derived
		}
		if v, ok := cache[derived]; ok {
			return v
		}
		key := normalize.FoldSlug(derived)
		result, err := r.Aliases.CanonicalCompanySlugs(ctx, []string{key})
		if err != nil {
			log.Printf("ingest: %s board %q (%s): company-alias lookup failed for %q, keeping the derived slug: %v",
				e.Provider, e.Board, e.Company, derived, err)
			return derived // not cached: a transient failure must not pin the wrong slug for the run
		}
		canonical := derived
		if c, ok := result[key]; ok {
			canonical = c
		}
		cache[derived] = canonical
		return canonical
	}
}

// distinctCompanySlugs derives the company scope using the same normalization as stored jobs.
func distinctCompanySlugs(raw []sources.Job) []string {
	names := make([]string, len(raw))
	for i, j := range raw {
		names[i] = j.Company
		if j.CompanyKey != "" {
			names[i] = j.CompanyKey
		}
	}
	return distinctSlugs(names)
}

// distinctSlugs normalizes company names and removes empty or repeated slugs.
func distinctSlugs(companies []string) []string {
	seen := make(map[string]bool)
	var slugs []string
	for _, c := range companies {
		slug := normalize.CompanySlug(c)
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		slugs = append(slugs, slug)
	}
	return slugs
}
