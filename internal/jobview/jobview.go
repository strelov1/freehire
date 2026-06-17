// Package jobview defines the single public wire shape of a job — the JSON
// representation served by the list, detail, and search endpoints and stored in
// the search index. Keeping one type (instead of parallel per-endpoint structs)
// makes drift between the API surfaces impossible.
package jobview

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
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
	// Countries/Regions/WorkMode/Skills are served from the jobs dictionary columns
	// ONLY — the deterministic dictionaries are the sole production source for these
	// facets. The LLM's enrichment values for them are deliberately excluded from
	// the served object (they remain raw in the stored enrichment JSONB), so the LLM
	// can later run free as a discovery signal without corrupting production data.
	// They are served top-level and once; the same fields are folded out of the
	// nested Enrichment to avoid duplication.
	Countries []string `json:"countries"`
	Regions   []string `json:"regions"`
	WorkMode  string   `json:"work_mode,omitempty"`
	Skills    []string `json:"skills"`
	PostedAt  *string  `json:"posted_at"`
	CreatedAt *string  `json:"created_at"`
	UpdatedAt *string  `json:"updated_at"`
	// ClosedAt is non-null when the posting is no longer open. Lists and the
	// search index never contain closed jobs; only the detail endpoint serves
	// them, and the SPA renders the closed state from this field.
	ClosedAt          *string           `json:"closed_at"`
	Enrichment        enrich.Enrichment `json:"enrichment"`
	EnrichedAt        *string           `json:"enriched_at"`
	EnrichmentVersion int32             `json:"enrichment_version"`
}

// FromRow maps a database job row to the public wire shape. The enrichment
// JSONB is decoded into the typed Enrichment; an empty or absent payload yields
// the zero Enrichment.
func FromRow(j db.Job) (Job, error) {
	var e enrich.Enrichment
	if len(j.Enrichment) > 0 {
		if err := json.Unmarshal(j.Enrichment, &e); err != nil {
			return Job{}, fmt.Errorf("jobview: decode enrichment for job %d: %w", j.ID, err)
		}
	}

	// The six dictionary-derived facets are sourced from the jobs columns ONLY (the
	// deterministic dictionaries are the production source); the LLM's values for
	// them are excluded from the served object so the LLM can later run free as a
	// discovery signal without corrupting production data. The LLM values stay in
	// the stored enrichment JSONB (untouched) but are folded out of the served copy
	// here. normalizeSet lowercases/sorts/dedups each column and guarantees a
	// non-nil slice so the facet serializes as [] not null.
	countries := normalizeSet(j.Countries)
	regions := normalizeSet(j.Regions)
	workMode := j.WorkMode
	// Seniority/category are the dictionary column value, always — never the LLM's,
	// and never a dict-silent fill. They stay nested under enrichment, so the
	// existing enrichment.seniority/category facets are unchanged.
	e.Seniority = j.Seniority
	e.Category = j.Category
	// The synthetic facets (posting_language/employment_type/education_level/
	// experience_years_min) follow the same dict-only rule: the deterministic column
	// value always wins (the LLM's stays raw in the JSONB), kept nested under
	// enrichment so the wire shape is unchanged.
	e.PostingLanguage = j.PostingLanguage
	e.EmploymentType = j.EmploymentType
	e.EducationLevel = j.EducationLevel
	e.ExperienceYearsMin = int4ToPtr(j.ExperienceYearsMin)
	skills := normalizeSet(j.Skills)
	e.Countries, e.Regions, e.WorkMode = nil, nil, ""
	e.Skills = nil

	return Job{
		PublicSlug:        j.PublicSlug,
		Source:            j.Source,
		ManuallyAdded:     j.CreatedBy.Valid,
		ExternalID:        j.ExternalID,
		URL:               j.URL,
		Title:             j.Title,
		Company:           j.Company,
		CompanySlug:       j.CompanySlug,
		Location:          j.Location,
		Description:       j.Description,
		Countries:         countries,
		Regions:           regions,
		WorkMode:          workMode,
		Skills:            skills,
		PostedAt:          rfc3339(j.PostedAt),
		CreatedAt:         rfc3339(j.CreatedAt),
		UpdatedAt:         rfc3339(j.UpdatedAt),
		ClosedAt:          rfc3339(j.ClosedAt),
		Enrichment:        e,
		EnrichedAt:        rfc3339(j.EnrichedAt),
		EnrichmentVersion: j.EnrichmentVersion,
	}, nil
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

// rfc3339 renders a nullable Postgres timestamp as an RFC3339 UTC string, or nil
// when unset. UTC keeps the lexicographic order chronological for sorting.
func rfc3339(ts pgtype.Timestamptz) *string {
	if !ts.Valid {
		return nil
	}
	s := ts.Time.UTC().Format(time.RFC3339)
	return &s
}

// int4ToPtr renders a nullable Postgres integer (experience_years_min) as *int, or
// nil when unset — so an unknown value is omitted from the served enrichment rather
// than reported as 0.
func int4ToPtr(n pgtype.Int4) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int32)
	return &v
}
