package jobview

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
)

func ptr[T any](v T) *T { return &v }

// TestEffectivePostedAt covers the exported single-source-of-truth helper directly:
// it is what both the display posted_at (RFC3339) and the index posted_ts (epoch)
// derive from, so the null/future fallback must hold independent of FromRow.
func TestEffectivePostedAt(t *testing.T) {
	created := pgtype.Timestamptz{Time: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC), Valid: true}

	// A real, past posted_at is returned unchanged.
	past := pgtype.Timestamptz{Time: time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC), Valid: true}
	if got := EffectivePostedAt(past, created); !got.Time.Equal(past.Time) {
		t.Errorf("past posted_at: got %v, want %v", got.Time, past.Time)
	}

	// A future posted_at falls back to the ingest time (created_at).
	future := pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true}
	if got := EffectivePostedAt(future, created); !got.Time.Equal(created.Time) {
		t.Errorf("future posted_at: got %v, want created %v", got.Time, created.Time)
	}

	// A missing (invalid) posted_at also falls back to created_at.
	if got := EffectivePostedAt(pgtype.Timestamptz{}, created); !got.Time.Equal(created.Time) {
		t.Errorf("missing posted_at: got %v, want created %v", got.Time, created.Time)
	}
}

func TestFromRow_PostedAtFallsBackToCreatedAtWhenFutureOrMissing(t *testing.T) {
	created := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	createdTS := pgtype.Timestamptz{Time: created, Valid: true}
	const wantCreated = "2026-06-18T12:00:00Z"
	base := db.Job{Source: "workday", ExternalID: "1", PublicSlug: "s", CreatedAt: createdTS, Enrichment: []byte("{}")}

	// A future posted_at (e.g. a Workday startDate set to a go-live date) must not sort the
	// job to the top of the freshest-first browse — it reads as its ingest time instead.
	future := base
	future.PostedAt = pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true}
	v, err := FromRow(future)
	if err != nil {
		t.Fatalf("FromRow(future): %v", err)
	}
	if v.PostedAt == nil || *v.PostedAt != wantCreated {
		t.Errorf("future posted_at: PostedAt = %v, want created_at %s", v.PostedAt, wantCreated)
	}
	if v.CreatedAt == nil || *v.CreatedAt != wantCreated {
		t.Errorf("CreatedAt should still be exposed raw, got %v", v.CreatedAt)
	}

	// A missing posted_at (undated source) also falls back to the ingest time.
	missing := base
	missing.PostedAt = pgtype.Timestamptz{}
	v2, err := FromRow(missing)
	if err != nil {
		t.Fatalf("FromRow(missing): %v", err)
	}
	if v2.PostedAt == nil || *v2.PostedAt != wantCreated {
		t.Errorf("missing posted_at: PostedAt = %v, want created_at %s", v2.PostedAt, wantCreated)
	}

	// A real, past posted_at is left untouched.
	past := base
	past.PostedAt = pgtype.Timestamptz{Time: time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC), Valid: true}
	v3, err := FromRow(past)
	if err != nil {
		t.Fatalf("FromRow(past): %v", err)
	}
	if v3.PostedAt == nil || *v3.PostedAt != "2026-06-10T09:00:00Z" {
		t.Errorf("past posted_at should stay unchanged, got %v", v3.PostedAt)
	}
}

func TestFromRow_MapsCoreAndNestedEnrichment(t *testing.T) {
	posted := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	// LLM-only fields (domains, visa, salary) stay nested; the six dictionary
	// facets are served from the jobs columns, so they live there in the fixture.
	raw, err := json.Marshal(enrich.Enrichment{
		Domains:         []string{"fintech"},
		VisaSponsorship: ptr(true),
		SalaryMin:       ptr(100000),
	})
	if err != nil {
		t.Fatalf("marshal enrichment: %v", err)
	}

	view, err := FromRow(db.Job{
		ID:          42,
		Source:      "manual",
		ExternalID:  "ext-1",
		Title:       "Senior Go Developer",
		Company:     "Acme",
		CompanySlug: "acme",
		Location:    "Berlin",
		Remote:      true,
		Description: "Build durable systems",
		Seniority:   "senior",                   // dictionary
		Category:    "backend",                  // dictionary
		Skills:      []string{"go", "postgres"}, // dictionary
		PostedAt:    pgtype.Timestamptz{Time: posted, Valid: true},
		PublicSlug:  "senior-go-developer-acme-abcd1234",
		Enrichment:  raw,
	})
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}

	if view.PublicSlug != "senior-go-developer-acme-abcd1234" {
		t.Errorf("PublicSlug = %q", view.PublicSlug)
	}
	if view.Title != "Senior Go Developer" || view.Company != "Acme" || view.Source != "manual" {
		t.Errorf("core fields not mapped: %+v", view)
	}
	if view.PostedAt == nil || *view.PostedAt != "2025-01-02T03:04:05Z" {
		t.Errorf("PostedAt = %v, want RFC3339 UTC", view.PostedAt)
	}
	// Enrichment stays nested and typed.
	if view.Enrichment.Seniority != "senior" || view.Enrichment.Category != "backend" {
		t.Errorf("nested enrichment not mapped: %+v", view.Enrichment)
	}
	if view.Enrichment.SalaryMin == nil || *view.Enrichment.SalaryMin != 100000 {
		t.Errorf("nested salary_min = %v", view.Enrichment.SalaryMin)
	}
	// Skills are folded into the top-level facet; VisaSponsorship stays nested.
	if len(view.Skills) != 2 || view.Skills[0] != "go" || view.Skills[1] != "postgres" {
		t.Errorf("top-level skills = %v, want [go postgres]", view.Skills)
	}
	if view.Enrichment.Skills != nil {
		t.Errorf("enrichment.skills should be folded out, got %#v", view.Enrichment.Skills)
	}
	if view.Enrichment.VisaSponsorship == nil || !*view.Enrichment.VisaSponsorship {
		t.Errorf("nested visa not mapped: %+v", view.Enrichment)
	}
}

// Doctrine: the six dictionary facets are served from the jobs columns ONLY. The
// LLM's geography/work_mode values are not unioned or substituted in — they remain
// in the stored enrichment JSONB but are folded out of the served object.
func TestFromRow_GeographyAndWorkModeAreDictOnly(t *testing.T) {
	raw, err := json.Marshal(enrich.Enrichment{
		Regions:   []string{"emea"}, // LLM-only region — must NOT be unioned in
		Countries: []string{"de"},   // LLM-only country — must NOT be unioned in
		WorkMode:  "remote",         // must NOT override the dict value
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	view, err := FromRow(db.Job{
		ID:         1,
		Title:      "Dev",
		PublicSlug: "dev-1",
		Countries:  []string{"us"}, // dictionary
		Regions:    []string{"us"}, // dictionary
		WorkMode:   "onsite",       // dictionary — served as-is, LLM ignored
		Enrichment: raw,
	})
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}

	if got := view.Countries; len(got) != 1 || got[0] != "us" {
		t.Errorf("Countries = %v, want [us] (dict only, no LLM union)", got)
	}
	if got := view.Regions; len(got) != 1 || got[0] != "us" {
		t.Errorf("Regions = %v, want [us] (dict only, no LLM union)", got)
	}
	if view.WorkMode != "onsite" {
		t.Errorf("WorkMode = %q, want onsite (dict only, LLM ignored)", view.WorkMode)
	}
	// Folded out of enrichment so geography/work_mode are reported once.
	if len(view.Enrichment.Regions) != 0 || len(view.Enrichment.Countries) != 0 || view.Enrichment.WorkMode != "" {
		t.Errorf("enrichment still carries folded fields: %+v", view.Enrichment)
	}
}

// A dictionary-silent facet is served empty, never filled from the LLM — the
// load-bearing case that lets the LLM later run free without leaking into
// production facets.
func TestFromRow_DictSilentFacetsAreEmptyNotLLM(t *testing.T) {
	raw, err := json.Marshal(enrich.Enrichment{
		Countries: []string{"fr"},
		Regions:   []string{"eu"},
		WorkMode:  "remote",
		Skills:    []string{"rust"},
		Seniority: "senior",
		Category:  "backend",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// jobs columns all empty (the dictionaries resolved nothing).
	view, err := FromRow(db.Job{ID: 1, Title: "x", PublicSlug: "x-1", Enrichment: raw})
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	if len(view.Countries) != 0 || len(view.Regions) != 0 || len(view.Skills) != 0 {
		t.Errorf("multi-valued facets should be empty, got countries=%v regions=%v skills=%v", view.Countries, view.Regions, view.Skills)
	}
	if view.WorkMode != "" {
		t.Errorf("WorkMode = %q, want empty (dict silent, LLM ignored)", view.WorkMode)
	}
	if view.Enrichment.Seniority != "" || view.Enrichment.Category != "" {
		t.Errorf("seniority/category should be empty, got {%q, %q}", view.Enrichment.Seniority, view.Enrichment.Category)
	}
}

func TestFromRow_WorkModeIsTheDictValue(t *testing.T) {
	view, err := FromRow(db.Job{ID: 1, Title: "x", PublicSlug: "x-1", WorkMode: "hybrid"})
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	if view.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid (dict value)", view.WorkMode)
	}
}

func TestFromRow_CopiesEngagementCounts(t *testing.T) {
	view, err := FromRow(db.Job{ID: 1, Title: "x", PublicSlug: "x-1", ViewCount: 42, AppliedCount: 7})
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	if view.ViewCount != 42 {
		t.Errorf("ViewCount = %d, want 42", view.ViewCount)
	}
	if view.AppliedCount != 7 {
		t.Errorf("AppliedCount = %d, want 7", view.AppliedCount)
	}
}

func TestFromRow_EmptyEnrichmentIsZero(t *testing.T) {
	// An unenriched job's column arrives as "{}" (the table default) or, in
	// edge cases, a nil byte slice. Both must decode to the zero Enrichment,
	// never fail.
	for _, payload := range [][]byte{[]byte("{}"), nil} {
		view, err := FromRow(db.Job{ID: 1, Title: "x", PublicSlug: "x-1", Enrichment: payload})
		if err != nil {
			t.Fatalf("FromRow with enrichment %q: %v", payload, err)
		}
		if view.Enrichment.Seniority != "" || view.Enrichment.SalaryMin != nil || len(view.Enrichment.Skills) != 0 {
			t.Errorf("enrichment %q: expected zero enrichment, got %+v", payload, view.Enrichment)
		}
	}
}

// Job's JSON encoding IS the API contract for every jobs read endpoint. These
// tests lock two requirements: the internal numeric id is never exposed and the
// public slug is (specs/job-public-identity), and the enrichment payload
// survives the mapping (specs/job-enrichment): an unenriched job serializes
// enrichment as {} (not null), and an enriched payload keeps its fields.

func TestJobJSON_HidesIDExposesSlug(t *testing.T) {
	fields := marshalToFields(t, db.Job{
		ID:         123,
		Title:      "Go Developer",
		PublicSlug: "go-developer-acme-t35nijto",
	})

	if _, leaked := fields["id"]; leaked {
		t.Error("wire shape leaks the internal numeric id")
	}
	if got := string(fields["public_slug"]); got != `"go-developer-acme-t35nijto"` {
		t.Errorf("public_slug: want the slug, got %s", got)
	}
}

// The engagement counters are part of the public wire contract (served on every
// job read, displayed on the detail page) and serialize as plain integers.
func TestJobJSON_ExposesEngagementCounts(t *testing.T) {
	fields := marshalToFields(t, db.Job{ID: 1, Title: "x", PublicSlug: "x-1", ViewCount: 12, AppliedCount: 3})

	if got := string(fields["view_count"]); got != "12" {
		t.Errorf("view_count: want 12, got %s", got)
	}
	if got := string(fields["applied_count"]); got != "3" {
		t.Errorf("applied_count: want 3, got %s", got)
	}
}

// The raw remote flag is demoted to an internal enrichment hint and must not
// appear in the public job object — "remote" is expressed solely through
// enrichment.work_mode / regions.
func TestJobJSON_OmitsRawRemoteFlag(t *testing.T) {
	fields := marshalToFields(t, db.Job{ID: 1, Title: "x", PublicSlug: "x-1", Remote: true})

	if _, present := fields["remote"]; present {
		t.Error("public job object must not include the raw remote flag")
	}
}

// Un-enriched job: enrichment is {} (not null), enriched_at is null,
// enrichment_version is 0.
func TestJobJSON_Unenriched(t *testing.T) {
	fields := marshalToFields(t, db.Job{ID: 1, Title: "Go Developer"})

	if got := string(fields["enrichment"]); got != "{}" {
		t.Errorf("enrichment: want {}, got %s", got)
	}
	if got := string(fields["posted_at"]); got != "null" {
		t.Errorf("posted_at: want null for an unset timestamp, got %s", got)
	}
	if got := string(fields["enriched_at"]); got != "null" {
		t.Errorf("enriched_at: want null, got %s", got)
	}
	if got := string(fields["enrichment_version"]); got != "0" {
		t.Errorf("enrichment_version: want 0, got %s", got)
	}
}

// Enriched job: the JSONB payload survives the typed decode/encode round-trip,
// enriched_at is the RFC3339 UTC timestamp, version is set.
func TestJobJSON_Enriched(t *testing.T) {
	enrichedAt := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	fields := marshalToFields(t, db.Job{
		ID:                2,
		Title:             "Senior Go Developer",
		Seniority:         "senior", // dictionary — surfaces under enrichment.seniority
		WorkMode:          "remote", // dictionary — surfaces top-level
		Enrichment:        json.RawMessage(`{"seniority":"senior","work_mode":"remote","domains":["fintech"]}`),
		EnrichedAt:        pgtype.Timestamptz{Time: enrichedAt, Valid: true},
		EnrichmentVersion: 1,
	})

	var enrichment map[string]any
	if err := json.Unmarshal(fields["enrichment"], &enrichment); err != nil {
		t.Fatalf("enrichment is not a JSON object: %v", err)
	}
	// LLM-only fields survive the typed round-trip.
	if domains, ok := enrichment["domains"].([]any); !ok || len(domains) != 1 || domains[0] != "fintech" {
		t.Errorf("enrichment payload not preserved: %v", enrichment)
	}
	// seniority is the dictionary value, served nested under enrichment.
	if enrichment["seniority"] != "senior" {
		t.Errorf("enrichment.seniority = %v, want senior (dict value)", enrichment["seniority"])
	}
	// work_mode is folded into the top-level facet, not duplicated under enrichment.
	if got := string(fields["work_mode"]); got != `"remote"` {
		t.Errorf("work_mode: want top-level \"remote\", got %s", got)
	}
	if _, dup := enrichment["work_mode"]; dup {
		t.Error("work_mode must not also appear under enrichment")
	}
	if got := string(fields["enriched_at"]); got != `"2026-06-09T12:00:00Z"` {
		t.Errorf("enriched_at: want the timestamp, got %s", got)
	}
	if got := string(fields["enrichment_version"]); got != "1" {
		t.Errorf("enrichment_version: want 1, got %s", got)
	}
}

// marshalToFields maps a db.Job through the wire shape and returns its
// top-level JSON fields — the actual public contract.
func marshalToFields(t *testing.T, job db.Job) map[string]json.RawMessage {
	t.Helper()
	view, err := FromRow(job)
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return fields
}

// Skills are served from the jobs column only; the LLM's extra skills are not
// unioned in (they remain in the stored enrichment JSONB for later discovery).
func TestFromRow_SkillsAreDictOnly(t *testing.T) {
	enr, _ := json.Marshal(enrich.Enrichment{Skills: []string{"go", "docker"}})
	j := db.Job{
		ID: 1, Skills: []string{"go", "kubernetes"}, Enrichment: enr,
	}
	got, err := FromRow(j)
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	want := []string{"go", "kubernetes"} // dict column only, lowercased + sorted
	if !reflect.DeepEqual(got.Skills, want) {
		t.Fatalf("Skills = %#v, want %#v (dict only, no LLM union)", got.Skills, want)
	}
	if got.Enrichment.Skills != nil {
		t.Errorf("enrichment.skills should be folded out, got %#v", got.Enrichment.Skills)
	}
}

// Collections is a top-level facet served straight from the jobs column (a
// denormalized copy of the owning company's membership). There is no LLM
// counterpart, so it is simply normalized (lowercased, sorted, deduped) and
// always non-nil so it serializes as [] not null.
func TestFromRow_CollectionsFromColumn(t *testing.T) {
	got, err := FromRow(db.Job{ID: 1, Collections: []string{"YC", "bigtech", "yc"}})
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	want := []string{"bigtech", "yc"} // lowercased, sorted, deduped
	if !reflect.DeepEqual(got.Collections, want) {
		t.Fatalf("Collections = %#v, want %#v", got.Collections, want)
	}
}

func TestFromRow_CollectionsEmptyIsNonNil(t *testing.T) {
	got, err := FromRow(db.Job{ID: 1})
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	if got.Collections == nil {
		t.Fatalf("Collections should be non-nil empty slice, got nil")
	}
}

// Seniority/category are the dictionary column value, always — the LLM never wins
// and never fills a dict-silent field. They stay nested under enrichment so the
// existing facet path is unchanged.
func TestFromRow_ClassificationIsDictOnly(t *testing.T) {
	// LLM present but the dictionary value wins.
	raw, err := json.Marshal(enrich.Enrichment{Seniority: "lead", Category: "devops"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	view, err := FromRow(db.Job{
		ID: 1, Title: "x", PublicSlug: "x-1",
		Seniority: "senior", Category: "backend", // dictionary — wins
		Enrichment: raw,
	})
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	if view.Enrichment.Seniority != "senior" || view.Enrichment.Category != "backend" {
		t.Errorf("dict should win: got {%q, %q}", view.Enrichment.Seniority, view.Enrichment.Category)
	}
}

// The synthetic facets (posting_language/employment_type/education_level/
// english_level/experience_years_min) are dict-only too: the column value wins over
// the LLM's, and a silent column drops the LLM value rather than falling back to it.
func TestFromRow_SyntheticFacetsAreDictOnly(t *testing.T) {
	exp := 8
	// LLM present with different values; the columns must win (and the silent
	// experience column must drop the LLM's 8).
	raw, err := json.Marshal(enrich.Enrichment{
		PostingLanguage:    "fr",
		EmploymentType:     "contract",
		EducationLevel:     "phd",
		EnglishLevel:       "b1",
		ExperienceYearsMin: &exp,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	view, err := FromRow(db.Job{
		ID: 1, Title: "x", PublicSlug: "x-1",
		PostingLanguage: "en", EmploymentType: "internship", EducationLevel: "bachelor", EnglishLevel: "c1",
		ExperienceYearsMin: pgtype.Int4{}, // silent column → served as nil, not the LLM's 8
		Enrichment:         raw,
	})
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	if view.Enrichment.PostingLanguage != "en" || view.Enrichment.EmploymentType != "internship" ||
		view.Enrichment.EducationLevel != "bachelor" || view.Enrichment.EnglishLevel != "c1" {
		t.Errorf("dict should win: got {%q,%q,%q,%q}", view.Enrichment.PostingLanguage,
			view.Enrichment.EmploymentType, view.Enrichment.EducationLevel, view.Enrichment.EnglishLevel)
	}
	if view.Enrichment.ExperienceYearsMin != nil {
		t.Errorf("silent experience column should drop the LLM value, got %v", *view.Enrichment.ExperienceYearsMin)
	}
}

// Cities is the one hybrid facet: the deterministic column wins, but a dict-silent
// job falls back to the LLM's enrichment.cities (normalized), and the LLM value is
// folded out of the nested enrichment either way.
func TestFromRow_CitiesHybrid(t *testing.T) {
	t.Run("dict column wins over the LLM", func(t *testing.T) {
		raw, _ := json.Marshal(enrich.Enrichment{Cities: []string{"Springfield"}})
		view, err := FromRow(db.Job{ID: 1, PublicSlug: "x-1", Cities: []string{"Berlin"}, Enrichment: raw})
		if err != nil {
			t.Fatalf("FromRow: %v", err)
		}
		if len(view.Cities) != 1 || view.Cities[0] != "Berlin" {
			t.Errorf("Cities = %v, want [Berlin] (dict wins)", view.Cities)
		}
		if view.Enrichment.Cities != nil {
			t.Errorf("LLM cities must be folded out, got %v", view.Enrichment.Cities)
		}
	})
	t.Run("dict-silent falls back to normalized LLM cities", func(t *testing.T) {
		raw, _ := json.Marshal(enrich.Enrichment{Cities: []string{"Kyiv, Ukraine", " kyiv ", "Lviv"}})
		view, err := FromRow(db.Job{ID: 2, PublicSlug: "x-2", Cities: nil, Enrichment: raw})
		if err != nil {
			t.Fatalf("FromRow: %v", err)
		}
		// ", Ukraine" suffix dropped, "kyiv"/"Kyiv, Ukraine" deduped, sorted.
		want := []string{"Kyiv", "Lviv"}
		if !reflect.DeepEqual(view.Cities, want) {
			t.Errorf("Cities = %v, want %v (LLM fallback normalized)", view.Cities, want)
		}
	})
	t.Run("work-mode markers leaked as LLM cities are dropped", func(t *testing.T) {
		raw, _ := json.Marshal(enrich.Enrichment{Cities: []string{"Remote", "Berlin", "Worldwide", "Anywhere"}})
		view, err := FromRow(db.Job{ID: 4, PublicSlug: "x-4", Cities: nil, Enrichment: raw})
		if err != nil {
			t.Fatalf("FromRow: %v", err)
		}
		if !reflect.DeepEqual(view.Cities, []string{"Berlin"}) {
			t.Errorf("Cities = %v, want [Berlin] (work-mode markers filtered)", view.Cities)
		}
	})
	t.Run("neither dict nor LLM yields an empty array, not null", func(t *testing.T) {
		view, err := FromRow(db.Job{ID: 3, PublicSlug: "x-3"})
		if err != nil {
			t.Fatalf("FromRow: %v", err)
		}
		if view.Cities == nil {
			t.Errorf("Cities must be non-nil ([] not null)")
		}
	})
}

// closed_at rides the wire shape so the SPA can render the closed state on the
// detail page (lists never serve closed jobs — see the job-lifecycle spec).
func TestFromRow_CarriesClosedAt(t *testing.T) {
	closedAt := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	v, err := FromRow(db.Job{ClosedAt: pgtype.Timestamptz{Time: closedAt, Valid: true}})
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	if v.ClosedAt == nil || *v.ClosedAt != "2026-06-12T10:00:00Z" {
		t.Fatalf("ClosedAt = %v, want 2026-06-12T10:00:00Z", v.ClosedAt)
	}

	open, err := FromRow(db.Job{})
	if err != nil {
		t.Fatalf("FromRow open: %v", err)
	}
	if open.ClosedAt != nil {
		t.Fatalf("open job ClosedAt = %v, want nil", *open.ClosedAt)
	}
}
