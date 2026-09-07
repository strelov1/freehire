package main

import (
	"testing"

	"github.com/strelov1/freehire/internal/job/jobhash"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
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

// TestDeriveRow_PreservesProfessionITTechHint guards against a real regression the
// review after issue #2601 caught: deriveRow re-derives is_tech via jobderive.Derive
// with no IsTechHint set, so a Profession itdev/itops row whose is_tech was set true
// by the ingest-time hint (or by cmd/backfill-profession-it-tech) would be silently
// recomputed back to unknown the next time this routine ~15h pass touches it — undoing
// the fix for exactly the rows it exists to reach. The board is recoverable from the
// stored external_id's namespace prefix (internal/platform/externalid.Namespace),
// which is all deriveRow has: it never re-crawls.
func TestDeriveRow_PreservesProfessionITTechHint(t *testing.T) {
	job := db.Job{
		ID: 1, Title: "Windows rendszermérnök", Company: "Acme",
		Source: "profession", ExternalID: "itdev:1",
	}

	params, _, _ := deriveRow(job, nil)

	want := pgconv.Bool(boolp(true))
	if params.IsTech != want {
		t.Errorf("IsTech = %+v, want %+v (confirmed by the Profession itdev board, not resolvable by title alone)", params.IsTech, want)
	}
}

// TestDeriveRow_DoesNotConfuseAnotherSourceForProfessionsITBoards guards the other
// direction: the external_id prefix alone must not be trusted without the source too.
func TestDeriveRow_DoesNotConfuseAnotherSourceForProfessionsITBoards(t *testing.T) {
	job := db.Job{
		ID: 1, Title: "Windows rendszermérnök", Company: "Acme",
		Source: "greenhouse", ExternalID: "itdev:1",
	}

	params, _, _ := deriveRow(job, nil)

	if params.IsTech.Valid {
		t.Errorf("IsTech = %+v, want unknown — the external_id prefix coincides with a Profession board name but the source does not match", params.IsTech)
	}
}

func boolp(b bool) *bool { return &b }

// TestDeriveRow_WithNoRegistryIsUnchanged: an empty registry must leave the derivation exactly
// as it was, so the guard cannot alter a catalogue that has never been merged.
func TestDeriveRow_WithNoRegistryIsUnchanged(t *testing.T) {
	job := db.Job{ID: 1, Title: "Backend Engineer", Company: "DollarTree", Source: "adzuna", ExternalID: "1"}
	params, _, _ := deriveRow(job, nil)
	if params.CompanySlug != "dollartree" {
		t.Errorf("CompanySlug = %q, want dollartree", params.CompanySlug)
	}
}
