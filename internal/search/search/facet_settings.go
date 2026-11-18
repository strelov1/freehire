package search

import (
	"github.com/meilisearch/meilisearch-go"
	"github.com/strelov1/freehire/internal/dict/skillvec"
)

// facetSettings is the single source of truth for the jobs index configuration.
func facetSettings() *meilisearch.Settings {
	return &meilisearch.Settings{
		SearchableAttributes: []string{"title", "company", "description", "location"},
		// Enrichment facets are nested and filtered via dot paths; geography (regions,
		// countries), work_mode and skills are served top-level and filtered on the bare attribute.
		FilterableAttributes: []string{
			// id backs the swipe deck's `id NOT IN [...]` exclusion. A new filterable attribute
			// needs a reindex, and must be declared in the live settings BEFORE a binary that filters
			// on it, or /jobs 500s. That applies to every attribute below.
			"id", "is_closed", "is_duplicate",
			"source", "company_slug",
			"work_mode", "regions", "countries", "cities", "metros", "skills", "collections",
			// is_tech is served top-level; an unknown value is absent, so it filters as empty.
			"is_tech",
			// ai_interview is true-or-absent (written only when the company has an un-retracted
			// report), so the negative is NOT of the positive.
			"ai_interview",
			// auto_apply_available is true-or-absent: written only for sources auto-apply can
			// attempt (see JobDocument.AutoApplyAvailable).
			"auto_apply_available",
			// ai_archetype is derived at index time (aiarchetype) from skills and category.
			"ai_archetype",
			// role_type is derived at index time (roletype) from the title alone; no jobs column,
			// so only a reindex reaches existing postings.
			"role_type",
			// source_type is written at index time from the source (see JobDocument.SourceType).
			"source_type",
			// requires_clearance is true-or-absent, never false, so the "exclude clearance jobs"
			// filter negates the positive (see clearanceFragment in query_filter.go).
			"requires_clearance",
			// via_recruiter is a top-level column, true-or-absent like requires_clearance.
			"via_recruiter",
			// equity is a closed string vocabulary, absent when not stated (see EquityNotStated).
			"equity",
			// clearance_levels is present exactly when requires_clearance is (see
			// ClearanceLevelUnspecified in query_filter.go).
			"clearance_levels",
			// perks is a multi-value text[] facet enrolled through StringFacets; a full rebuild must
			// follow the backfill-derive that fills existing rows.
			"perks",
			// engagement is a multi-value text[] facet enrolled through StringFacets; existing rows
			// get the column from cmd/backfill-derive, followed by make reindex.
			"engagement",
			// apply_questions is the captured form's required-question count, attached at index time
			// and ABSENT when no form was captured, so `apply_questions <= N` never matches an unread
			// form. Numeric, behind `apply_questions_max`.
			"apply_questions",
			"enrichment.employment_type", "enrichment.education_level", "enrichment.seniority",
			"enrichment.category", "enrichment.domains",
			"enrichment.company_type", "enrichment.company_size", "enrichment.visa_sponsorship",
			"enrichment.salary_currency", "enrichment.salary_period",
			"enrichment.salary_min", "enrichment.salary_max", "enrichment.experience_years_min",
			"enrichment.relocation", "enrichment.english_level", "enrichment.posting_language",
			// posted_ts is the effective posting date in unix seconds; range filters need a number
			// (the string posted_at is sort-only).
			"posted_ts",
			// created_ts is the first-seen date in unix seconds for the "open within N days" bound.
			// A full rebuild must follow declaring it: the drain pushes only documents whose
			// content_hash moved and this field is not in that hash, so until then the filter matches
			// almost nothing. See AGENTS.md in this package.
			"created_ts",
			// reality.class is the job-reality signal (fresh/stale/likely-evergreen), nested under
			// the served reality object.
			"reality.class",
		},
		// posted_at / created_at are RFC3339 UTC strings and sort chronologically as text.
		// view_count backs the "Most viewed" order. An undeclared sort attribute makes Meili
		// reject the whole query, so declare it before a binary accepts ?sort=view_count.
		SortableAttributes: []string{"posted_at", "created_at", "view_count", "enrichment.salary_min", "enrichment.salary_max"},
		// posted_ts:desc is a freshness tie-breaker after exactness: relevance and any explicit
		// sort decide first. It uses the numeric posted_ts; custom ranking rules need no sortable
		// declaration.
		RankingRules: []string{"words", "sort", "typo", "proximity", "attribute", "exactness", "posted_ts:desc"},
		// The match sort's embedder. `userProvided`: Meilisearch calls no model; indexers write
		// vectors (arithmetic over internal/dict/skillvec) into each document's `_vectors`.
		// Dimensions is the registry's width and cannot change without a full rebuild; until then
		// the index rejects documents with the new width. Adding this embedder to a live index is
		// rebuild-only and must land BEFORE a binary that sends a query vector, else a vector
		// search is a 400.
		//
		// BinaryQuantized stays false: these vectors have 2-12 non-zeros out of 749, so a
		// sign-bit quantiser leaves nearly every bit agreeing across documents and recall
		// collapses, losing the rare skills the weighting surfaces.
		Embedders: map[string]meilisearch.Embedder{
			SkillEmbedder: {
				Source:     meilisearch.UserProvidedEmbedderSource,
				Dimensions: skillvec.Dimensions,
			},
		},
		// Typo tolerance stays at Meilisearch defaults: the SDK always serializes newer fields
		// (e.g. disableOnNumbers) that older server versions reject. Re-add tuning when the pinned
		// server and SDK align.
		Pagination: &meilisearch.Pagination{MaxTotalHits: maxTotalHits},
		// Raise the per-facet cap above Meili's default of 100 and keep the top values BY COUNT:
		// high-cardinality facets (cities) exceed maxValuesPerFacet, and the default alpha order
		// drops busy values that sort late, which client-side sorting cannot recover.
		// sortFacetValuesBy needs Meili 0.28+.
		Faceting: &meilisearch.Faceting{
			MaxValuesPerFacet: maxValuesPerFacet,
			SortFacetValuesBy: map[string]meilisearch.SortFacetType{"*": meilisearch.SortFacetTypeCount},
		},
		// byAttribute skips computing exact word-to-word distance, whose "merging word
		// proximity" indexing phase dominated batch time; job descriptions are long prose where
		// adjacency barely matters for ranking. prefixSearch is deliberately left on: header
		// search and the /jobs filters debounce query-as-you-type and rely on last-word prefix
		// matching.
		// Like FilterableAttributes, it applies only to data written after a reindex.
		ProximityPrecision: meilisearch.ByAttribute,
	}
}
