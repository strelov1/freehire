package ojcp

import "testing"

// The vendored schemas are the oracle for every projection in this package, so the
// oracle itself is tested first: a loader that silently accepted everything would
// make every later test green and meaningless.

func TestValidateAgainstSchemaAcceptsAConformingValue(t *testing.T) {
	// A minimal JobPosting carrying exactly the schema's four required fields.
	posting := map[string]any{
		"ojcp_id":    "senior-go-engineer-at-acme",
		"title":      "Senior Go Engineer",
		"employer":   map[string]any{"name": "Acme Corp"},
		"datePosted": "2026-09-16",
	}

	if err := validateAgainstSchema(t, schemaJobPosting, posting); err != nil {
		t.Fatalf("conforming JobPosting rejected: %v", err)
	}
}

func TestValidateAgainstSchemaRejectsAMissingRequiredField(t *testing.T) {
	// Same value with `title` removed. The schema requires it, so the loader must say so.
	posting := map[string]any{
		"ojcp_id":    "senior-go-engineer-at-acme",
		"employer":   map[string]any{"name": "Acme Corp"},
		"datePosted": "2026-09-16",
	}

	if err := validateAgainstSchema(t, schemaJobPosting, posting); err == nil {
		t.Fatal("JobPosting missing the required `title` was accepted; the oracle is blind")
	}
}

func TestValidateAgainstSchemaResolvesCrossFileReferencesOffline(t *testing.T) {
	// responses/job-detail.json $refs job-posting.json by its absolute ojcp.dev $id.
	// Compiling it proves the loader resolves that reference from the vendored copy
	// rather than reaching for the network.
	detail := map[string]any{
		"ojcp_version": "0.1",
		"job": map[string]any{
			"ojcp_id":    "senior-go-engineer-at-acme",
			"title":      "Senior Go Engineer",
			"employer":   map[string]any{"name": "Acme Corp"},
			"datePosted": "2026-09-16",
		},
	}

	if err := validateAgainstSchema(t, schemaJobDetailResponse, detail); err != nil {
		t.Fatalf("job-detail response rejected: %v", err)
	}
}
