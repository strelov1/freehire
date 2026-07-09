package job

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// jobFromRow is the anti-corruption mapping: it turns a persistence row (pgtype
// scalars, JSONB enrichment) into a domain Job (domain types, decoded enrichment).
// This unit test pins the mapping without a database.
func TestJobFromRow_MapsPersistenceToDomain(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	posted := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	closed := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	row := db.Job{
		ID:                 55,
		Source:             "greenhouse",
		ExternalID:         "acme:42",
		Title:              "Senior Go Developer",
		Company:            "Acme",
		CompanySlug:        "acme",
		PublicSlug:         "senior-go-developer-acme-abc",
		Location:           "Berlin, Germany",
		Countries:          []string{"de"},
		Skills:             []string{"go", "postgresql"},
		WorkMode:           "onsite",
		Seniority:          "senior",
		Category:           "backend",
		ExperienceYearsMin: pgtype.Int4{Int32: 5, Valid: true},
		Enrichment:         json.RawMessage(`{"summary":"Great role","countries":["es"],"work_mode":"remote"}`),
		EnrichmentVersion:  1,
		CreatedBy:          pgtype.Int8{Int64: 7, Valid: true},
		PostedAt:           pgtype.Timestamptz{Time: posted, Valid: true},
		CreatedAt:          pgtype.Timestamptz{Time: created, Valid: true},
		EnrichedAt:         pgtype.Timestamptz{Time: created, Valid: true},
		ClosedAt:           pgtype.Timestamptz{Time: closed, Valid: true},
	}

	j, err := jobFromRow(row)
	if err != nil {
		t.Fatalf("jobFromRow: %v", err)
	}
	f := j.Fields()

	if f.ID != 55 || f.Source != "greenhouse" || f.ExternalID != "acme:42" {
		t.Errorf("identity = %d/%q/%q", f.ID, f.Source, f.ExternalID)
	}
	if f.Seniority != "senior" || f.Category != "backend" || f.WorkMode != "onsite" {
		t.Errorf("dict facets = %q/%q/%q", f.Seniority, f.Category, f.WorkMode)
	}
	if f.ExperienceYearsMin == nil || *f.ExperienceYearsMin != 5 {
		t.Errorf("ExperienceYearsMin = %v, want 5", f.ExperienceYearsMin)
	}
	// Enrichment JSONB is decoded into the typed, RAW LLM payload (pre-fold).
	if f.Enrichment.Summary != "Great role" || len(f.Enrichment.Countries) != 1 || f.Enrichment.Countries[0] != "es" {
		t.Errorf("decoded enrichment = %+v", f.Enrichment)
	}
	if f.EnrichmentVersion != 1 {
		t.Errorf("EnrichmentVersion = %d, want 1", f.EnrichmentVersion)
	}
	// Provenance: created_by set → manually added.
	if !f.ManuallyAdded {
		t.Error("ManuallyAdded should be true when created_by is set")
	}
	// Nullable timestamps become *time.Time.
	if f.PostedAt == nil || !f.PostedAt.Equal(posted) {
		t.Errorf("PostedAt = %v, want %v", f.PostedAt, posted)
	}
	if f.CreatedAt == nil || !f.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v", f.CreatedAt)
	}
	// A closed row maps to a closed aggregate.
	if j.IsOpen() {
		t.Error("row with closed_at should map to a closed Job")
	}
	if f.ClosedAt == nil || !f.ClosedAt.Equal(closed) {
		t.Errorf("ClosedAt = %v, want %v", f.ClosedAt, closed)
	}
}

// An empty enrichment JSONB decodes to the zero Enrichment, not an error.
func TestJobFromRow_EmptyEnrichment(t *testing.T) {
	j, err := jobFromRow(db.Job{ID: 1, Source: "manual", ExternalID: "1", Title: "Engineer"})
	if err != nil {
		t.Fatalf("jobFromRow: %v", err)
	}
	if s := j.Fields().Enrichment.Summary; s != "" {
		t.Errorf("empty enrichment Summary = %q, want empty", s)
	}
	if !j.IsOpen() {
		t.Error("row with null closed_at should map to an open Job")
	}
	if j.Fields().ManuallyAdded {
		t.Error("ManuallyAdded should be false when created_by is null")
	}
}
