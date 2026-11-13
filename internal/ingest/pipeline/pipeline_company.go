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

// companySlugOf is the slug saveOne would store for company: the canonical one when the
// registry has one, the derived one otherwise.
func companySlugOf(canon companyCanonical, company string) string {
	if slug := canonicalCompanySlug(canon, company); slug != "" {
		return slug
	}
	return normalize.CompanySlug(company)
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
// coverage probe — which holds company names, not Jobs — resolves its whole board in the same
// single call.
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
// batch: it resolves one company as it is first seen and memoizes the answer. Like
// streamAggregatorCoverage it is only ever called from ingestStream's emit callback, already
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

// aggregatorCoverageForBatch resolves the ingest-time coverage gate for a buffered board's
// whole raw fetch in one batched call, up front.
func (r Runner) aggregatorCoverageForBatch(ctx context.Context, e sources.CompanyEntry, raw []sources.Job, canon companyCanonical) aggregatorCoverage {
	aggregators, countries, applies := r.aggregatorGate(e)
	if !applies {
		return nil
	}
	// The gate is asked about the CANONICAL slugs, because saveOne will ask it with the
	// canonical slug it is about to store. Resolving here rather than re-deriving there is
	// what makes the two sides one value instead of two that have to be kept equal.
	slugs := resolveAll(canon, distinctCompanySlugs(raw))
	if len(slugs) == 0 {
		return nil
	}
	covered, err := r.Coverage.NonAggregatorCompanies(ctx, slugs, aggregators, countries)
	if err != nil {
		log.Printf("ingest: %s board %q (%s): coverage lookup failed, gate disabled for this board: %v",
			e.Provider, e.Board, e.Company, err)
		return nil
	}
	return func(companySlug string) bool { return covered[companySlug] }
}

// distinctCompanySlugs returns the distinct company slugs raw's postings will DERIVE to —
// before the alias registry has its say. Its two callers both resolve the result through
// canonicalOf before using it, and that is the invariant to preserve: what reaches the
// coverage lookup must be what the upsert will store.
//
// This used to be guaranteed by both sides independently calling the same pure function on the
// same input, and that is no longer enough. A company slug now depends on registry state, so
// "both derive it the same way" can hold while the two values differ. One resolved map is
// handed to both consumers instead; see canonicalOf.
//
// normalizeJob is not called here (it can fail, and its result would be discarded) — just
// normalize.CompanySlug(j.Company), the same derivation jobderive performs, on the same input
// (j.Company, with no fallback to the entry's company — normalizeJob has none either).
func distinctCompanySlugs(raw []sources.Job) []string {
	names := make([]string, len(raw))
	for i, j := range raw {
		names[i] = j.Company
	}
	return distinctSlugs(names)
}

// distinctSlugs is distinctCompanySlugs over bare company names, for the coverage probe a
// CoverageGated adapter consults before it has any Jobs to speak of. Both callers must slug
// the same way, so neither does it itself — and it must be normalize.CompanySlug, the
// derivation jobderive performs, not normalize.Slug. Asking about "acme-inc" and then deciding
// about "acme" is a gate that answers a question nobody consults.
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

// streamAggregatorCoverage builds the ingest-time coverage gate for a streaming board: unlike
// aggregatorCoverageForBatch it has no upfront batch to resolve, so it resolves one company at
// a time as postings are emitted, memoizing each answer so a company with several postings in
// one stream is only ever looked up once. The returned closure is only ever called from within
// ingestStream's emit callback, which already serializes access under its own mutex, so the
// cache needs no locking of its own. See aggregatorGate for the shared applicability check.
func (r Runner) streamAggregatorCoverage(ctx context.Context, e sources.CompanyEntry) aggregatorCoverage {
	aggregators, countries, applies := r.aggregatorGate(e)
	if !applies {
		return nil
	}
	cache := make(map[string]bool)
	return func(companySlug string) bool {
		if companySlug == "" {
			return false // nothing to ask about -- mirrors distinctCompanySlugs's skip
		}
		if v, ok := cache[companySlug]; ok {
			return v
		}
		result, err := r.Coverage.NonAggregatorCompanies(ctx, []string{companySlug}, aggregators, countries)
		if err != nil {
			log.Printf("ingest: %s board %q (%s): coverage lookup failed for company %q, not gated: %v",
				e.Provider, e.Board, e.Company, companySlug, err)
			return false // not cached: a transient failure should not permanently un-gate the company
		}
		v := result[companySlug]
		cache[companySlug] = v
		return v
	}
}
