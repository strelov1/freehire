// Package jobview defines the single public wire shape of a job — the JSON
// representation served by the list, detail, and search endpoints and stored in
// the search index. Keeping one type (instead of parallel per-endpoint structs)
// makes drift between the API surfaces impossible.
package jobview

import (
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/job"
)

// Job is the public wire shape of a job. It carries the public_slug and
// deliberately omits the internal numeric id, which must never be exposed: the
// id is enumerable and its growth leaks inventory size and fill rate.
//
// Enrichment is nested (not flattened) and typed; an unenriched job serializes
// it as `{}`. Timestamps are RFC3339 UTC strings (or null) — the lexicographic
// order is chronological, which the search index relies on for sorting.
type Job struct {
	PublicSlug string `json:"public_slug"`
	Source     string `json:"source"`
	// ManuallyAdded marks a hand-curated posting added by a moderator (created_by is
	// set), as opposed to an automated source. It is the provenance signal, decoupled
	// from Source (which is the real origin); the authoring user id itself is internal
	// and never exposed.
	ManuallyAdded bool   `json:"manually_added"`
	ExternalID    string `json:"external_id"`
	URL           string `json:"url"`
	Title         string `json:"title"`
	Company       string `json:"company"`
	CompanySlug   string `json:"company_slug"`
	Location      string `json:"location"`
	Description   string `json:"description"`
	// WorkMode/Skills are served from the jobs dictionary columns ONLY — the
	// deterministic dictionaries are the sole production source for these facets. The
	// LLM's enrichment values for them are deliberately excluded from the served object
	// (they remain raw in the stored enrichment JSONB), so the LLM can later run free as
	// a discovery signal without corrupting production data.
	//
	// Countries/Regions are a HYBRID facet, like Cities (see geoFacet): the dictionary
	// columns win whenever they pinned a place, but an unpinned geography (no country,
	// and at most the bare-"Remote" "global" bucket) falls back to the LLM's
	// enrichment.countries/regions — catching a restriction stated only in the prose
	// ("Remote (SPAIN only)") that the location string never carried.
	//
	// All four are served top-level and once; the same fields are folded out of the
	// nested Enrichment to avoid duplication.
	Countries []string `json:"countries"`
	Regions   []string `json:"regions"`
	WorkMode  string   `json:"work_mode,omitempty"`
	Skills    []string `json:"skills"`
	// Cities is a HYBRID facet (the one deliberate dict+LLM exception): the
	// deterministic jobs.cities column when it resolved a beacon city, otherwise a
	// normalized fallback to the LLM's enrichment.cities — cities are high-cardinality
	// and the dictionary is only a beacon list, so a pure-dict facet would be too
	// sparse. Values are canonical display names (Title case); the LLM copy is folded
	// out of the nested Enrichment like the other geography facets.
	Cities []string `json:"cities"`
	// Collections is the set of curated-collection slugs (e.g. yc, bigtech) the
	// job's company belongs to, denormalized from the company onto the job. It is a
	// deterministic source fact (no LLM counterpart) served straight from the jobs
	// column; an untagged job serializes it as [].
	Collections []string `json:"collections"`
	PostedAt    *string  `json:"posted_at"`
	CreatedAt   *string  `json:"created_at"`
	UpdatedAt   *string  `json:"updated_at"`
	// ClosedAt is non-null when the posting is no longer open. Lists and the
	// search index never contain closed jobs; only the detail endpoint serves
	// them, and the SPA renders the closed state from this field.
	ClosedAt          *string           `json:"closed_at"`
	Enrichment        enrich.Enrichment `json:"enrichment"`
	EnrichedAt        *string           `json:"enriched_at"`
	EnrichmentVersion int32             `json:"enrichment_version"`
	// ViewCount/AppliedCount are the job's materialized engagement counters —
	// distinct signed-in users who viewed and who marked applied — served straight
	// from the jobs columns (no read-time counting). Displayed on the detail page.
	ViewCount    int32 `json:"view_count"`
	AppliedCount int32 `json:"applied_count"`
	// Reality is the job-reality signal (fresh/stale/likely-evergreen + evidence),
	// computed at index/read time and attached by ClassifyReality — never stored, as
	// it is time-dependent. Nil when not computed (e.g. a plain FromRow without counts).
	Reality *Reality `json:"reality,omitempty"`
}

// FromRow maps a database job row to the public wire shape. It is a thin shim: it
// hydrates the row into the Job aggregate (plus its read-only Extras) via
// job.FromRow, then delegates to FromDomain — the projection source of truth. It
// stays so existing read callers that hold a db.Job are untouched.
func FromRow(j db.Job) (Job, error) {
	domain, extras, err := job.FromRow(j)
	if err != nil {
		return Job{}, err
	}
	return FromDomain(domain, extras)
}

// FromDomain projects the Job aggregate (plus its read-only Extras) to the public
// wire shape. It is the projection source of truth; FromRow is a thin shim that
// hydrates a db row into the domain and delegates here. The facet handling mirrors
// FromRow exactly — the dictionary columns are served, the LLM's copies are folded
// out of the nested enrichment, and countries/regions/cities stay dict-then-LLM
// hybrids — so the wire output is byte-equivalent to the pre-aggregate projection.
func FromDomain(j job.Job, x job.Extras) (Job, error) {
	f := j.Fields()
	// e is the raw decoded LLM enrichment; the dictionary columns are folded over it
	// below (dict wins) and the multi-valued LLM facets are folded out.
	e := f.Enrichment

	countries, regions := geoFacet(f.Countries, f.Regions, e.Countries, e.Regions)
	workMode := f.WorkMode
	// Seniority/category and the synthetic facets are the dictionary column value,
	// always — kept nested under enrichment so the wire shape is unchanged.
	e.Seniority = f.Seniority
	e.Category = f.Category
	e.PostingLanguage = f.PostingLanguage
	e.EmploymentType = f.EmploymentType
	e.EducationLevel = f.EducationLevel
	e.EnglishLevel = f.EnglishLevel
	e.ExperienceYearsMin = f.ExperienceYearsMin
	skills := normalizeSet(f.Skills)
	collections := normalizeSet(x.Collections)
	cities := cityFacet(f.Cities, e.Cities)
	e.Countries, e.Regions, e.WorkMode = nil, nil, ""
	e.Skills = nil
	e.Cities = nil

	return Job{
		PublicSlug:        f.PublicSlug,
		Source:            f.Source,
		ManuallyAdded:     f.ManuallyAdded,
		ExternalID:        f.ExternalID,
		URL:               f.URL,
		Title:             f.Title,
		Company:           f.Company,
		CompanySlug:       f.CompanySlug,
		Location:          f.Location,
		Description:       f.Description,
		Countries:         countries,
		Regions:           regions,
		WorkMode:          workMode,
		Skills:            skills,
		Cities:            cities,
		Collections:       collections,
		PostedAt:          rfc3339Ptr(effectivePosted(f.PostedAt, f.CreatedAt)),
		CreatedAt:         rfc3339Ptr(f.CreatedAt),
		UpdatedAt:         rfc3339Ptr(f.UpdatedAt),
		ClosedAt:          rfc3339Ptr(f.ClosedAt),
		Enrichment:        e,
		EnrichedAt:        rfc3339Ptr(f.EnrichedAt),
		EnrichmentVersion: f.EnrichmentVersion,
		ViewCount:         x.ViewCount,
		AppliedCount:      x.AppliedCount,
	}, nil
}

// effectivePosted is EffectivePostedAt over domain *time.Time: the source posted
// time when present and not in the future, otherwise the ingest time.
func effectivePosted(posted, created *time.Time) *time.Time {
	if posted == nil || posted.After(time.Now()) {
		return created
	}
	return posted
}

// rfc3339Ptr renders a nullable domain timestamp as an RFC3339 UTC string, or nil.
func rfc3339Ptr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

// normalizeSet returns the sorted, deduplicated, lowercased form of a facet
// column. Case-folding keeps the facet in one canonical bucket; the result is
// always non-nil so the facet serializes as a JSON array (matching the text[]
// columns' empty-array default) rather than null.
func normalizeSet(a []string) []string {
	set := make(map[string]struct{}, len(a))
	for _, v := range a {
		set[strings.ToLower(v)] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// nonCityFallback are LLM-emitted "city" values that are really work-mode or
// open-anywhere markers, not places. The deterministic dictionary never produces
// these (it emits only real beacon cities); they leak in only through the
// enrichment.cities fallback, so they are dropped there. A bare-remote job's
// geography is carried by regions=global instead (see location.Parse), and its
// remoteness by the work_mode facet.
var nonCityFallback = map[string]struct{}{
	"remote": {}, "remote-first": {}, "fully remote": {}, "worldwide": {},
	"anywhere": {}, "global": {}, "distributed": {}, "hybrid": {},
	"onsite": {}, "on-site": {}, "work from home": {}, "wfh": {},
}

// geoFacet builds the served country/region facets. The deterministic dictionary
// columns win whenever they pinned a place — a country, or a region more specific
// than the open-anywhere "global" bucket. Only when the dictionary left geography
// unpinned (no country, and at most the bare-"Remote" "global" region) does it fall
// back wholesale to the LLM's enrichment.countries/regions, which read a restriction
// stated only in the prose ("Remote (SPAIN only)") that the location string never
// carried. This mirrors cityFacet's dict-then-LLM hybrid; a pinned dictionary place
// is never overridden, so the LLM can only fill the global/unspecified bucket — never
// corrupt a resolved facet. Both outputs are lowercased/sorted/deduped (non-nil).
func geoFacet(dictCountries, dictRegions, llmCountries, llmRegions []string) (countries, regions []string) {
	countries = normalizeSet(dictCountries)
	regions = normalizeSet(dictRegions)
	if geoPinned(countries, regions) {
		return countries, regions
	}
	llmC, llmR := normalizeSet(llmCountries), normalizeSet(llmRegions)
	if len(llmC) == 0 && len(llmR) == 0 {
		return countries, regions // the LLM knows no more than the dict — keep it
	}
	return llmC, llmR
}

// geoPinned reports whether the dictionary resolved a concrete place: a country, or a
// region more specific than the open-anywhere "global" bucket.
func geoPinned(countries, regions []string) bool {
	if len(countries) > 0 {
		return true
	}
	for _, r := range regions {
		if r != "global" {
			return true
		}
	}
	return false
}

// cityFacet builds the served city facet: the deterministic dictionary cities when
// present (canonical display names), otherwise a normalized fallback to the LLM's
// enrichment.cities. Case is preserved (the value is its own display label); the
// fallback trims each value, drops a ", Country" suffix, and dedupes.
func cityFacet(dict, llm []string) []string {
	if len(dict) > 0 {
		return dedupeSortedPreserveCase(dict)
	}
	cleaned := make([]string, 0, len(llm))
	for _, c := range llm {
		c = strings.TrimSpace(c)
		if i := strings.IndexByte(c, ','); i >= 0 {
			c = strings.TrimSpace(c[:i])
		}
		if c == "" {
			continue
		}
		if _, bad := nonCityFallback[strings.ToLower(c)]; bad {
			continue
		}
		cleaned = append(cleaned, c)
	}
	return dedupeSortedPreserveCase(cleaned)
}

// dedupeSortedPreserveCase sorts and dedupes case-insensitively while keeping the
// first-seen spelling, returning a non-nil slice so the facet serializes as [] not
// null. Used for cities, whose values are display names (not lowercased codes).
func dedupeSortedPreserveCase(a []string) []string {
	seen := make(map[string]struct{}, len(a))
	out := make([]string, 0, len(a))
	for _, v := range a {
		k := strings.ToLower(v)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// FromRows maps a batch of database rows to the public wire shape.
func FromRows(jobs []db.Job) ([]Job, error) {
	out := make([]Job, len(jobs))
	for i, j := range jobs {
		v, err := FromRow(j)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// EffectivePostedAt is the timestamp a job reads as "posted" for both display and the
// freshest-first sort: the source's posted_at when present and not in the future, otherwise
// the ingest time (created_at). A missing posted_at (undated source) or a future one (e.g. a
// Workday startDate set to a future go-live) would otherwise leave the job undated or sort it
// above genuinely recent postings; falling back to created_at gives an honest, recent
// freshness. The raw created_at stays exposed separately, so the substitution is visible.
//
// It is exported so the search document derives its numeric posted_ts (epoch) from the same
// definition the display posted_at (RFC3339) uses — one fallback rule, two encodings.
func EffectivePostedAt(posted, created pgtype.Timestamptz) pgtype.Timestamptz {
	if !posted.Valid || posted.Time.After(time.Now()) {
		return created
	}
	return posted
}
