package main

import (
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobhash"
)

// TestDeriveRow_ResolvesThroughTheAliasRegistry guards the merges against this worker.
//
// deriveRow re-derives every column from jobderive, which is a PURE function with no
// knowledge of company_slug_aliases. Left alone it would rewrite every posting a merge moved
// back to the spelling its source happened to use — silently undoing the whole spelling class,
// and churning role_fingerprint with it, since the fingerprint is computed from the company
// slug. A backfill is a routine ~15h run, so this is not a hypothetical.
func TestDeriveRow_ResolvesThroughTheAliasRegistry(t *testing.T) {
	canon := map[string]string{"dollartree": "dollar-tree"}
	job := db.Job{
		ID: 1, Title: "Backend Engineer", Company: "DollarTree",
		Source: "adzuna", ExternalID: "1", CompanySlug: "dollar-tree",
	}

	params, changed, slugMoved := deriveRow(job, canon)

	if params.CompanySlug != "dollar-tree" {
		t.Errorf("CompanySlug = %q, want dollar-tree — the merge must survive a re-derive",
			params.CompanySlug)
	}
	// The company slug is the point; the fixture leaves other derived columns empty, so
	// `changed` says nothing here. What must hold is that the SLUG did not move: the stored
	// value already is the canonical one.
	_ = changed
	if slugMoved && params.CompanySlug != job.CompanySlug {
		t.Error("the company slug moved away from the canonical one it already held")
	}
}

// TestDeriveRow_FingerprintFollowsTheCanonicalSlug: role_fingerprint is computed from the
// company slug, so it has to be computed from the RESOLVED one or every merged posting's
// repost identity churns on the next backfill.
func TestDeriveRow_FingerprintFollowsTheCanonicalSlug(t *testing.T) {
	canon := map[string]string{"dollartree": "dollar-tree"}
	job := db.Job{
		ID: 1, Title: "Backend Engineer", Company: "DollarTree",
		Source: "adzuna", ExternalID: "1", CompanySlug: "dollartree",
	}

	params, _, _ := deriveRow(job, canon)

	want := jobhash.RoleFingerprint(db.UpsertJobParams{
		CompanySlug: "dollar-tree", Title: job.Title, Description: job.Description,
	})
	if params.RoleFingerprint.String != want {
		t.Errorf("RoleFingerprint was computed from the unresolved slug")
	}
}

// TestDeriveRow_WithNoRegistryIsUnchanged: an empty registry must leave the derivation exactly
// as it was, so the guard cannot alter a catalogue that has never been merged.
func TestDeriveRow_WithNoRegistryIsUnchanged(t *testing.T) {
	job := db.Job{ID: 1, Title: "Backend Engineer", Company: "DollarTree", Source: "adzuna", ExternalID: "1"}
	params, _, _ := deriveRow(job, nil)
	if params.CompanySlug != "dollartree" {
		t.Errorf("CompanySlug = %q, want dollartree", params.CompanySlug)
	}
}
