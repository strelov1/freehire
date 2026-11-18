package jobview

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/platform/db"
)

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
