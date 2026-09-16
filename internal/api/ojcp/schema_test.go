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

func TestValidateAgainstSchemaRejectsAMalformedFormattedValue(t *testing.T) {
	// `datePosted` is declared `format: date`. In draft 2020-12 `format` is annotation-only
	// unless the validator opts in, so without that opt-in every value below is accepted and
	// the oracle is blind to exactly the field the posting projection writes first.
	for _, datePosted := range []string{
		"16/09/2026",           // day-first, not ISO
		"2026-09-16T00:00:00Z", // a timestamp where the schema wants a date
		"",                     // empty
	} {
		t.Run(datePosted, func(t *testing.T) {
			posting := map[string]any{
				"ojcp_id":    "senior-go-engineer-at-acme",
				"title":      "Senior Go Engineer",
				"employer":   map[string]any{"name": "Acme Corp"},
				"datePosted": datePosted,
			}

			if err := validateAgainstSchema(t, schemaJobPosting, posting); err == nil {
				t.Fatalf("datePosted %q was accepted; the oracle does not assert `format`", datePosted)
			}
		})
	}
}

func TestValidateAgainstSchemaRejectsAMalformedURL(t *testing.T) {
	posting := map[string]any{
		"ojcp_id":    "senior-go-engineer-at-acme",
		"title":      "Senior Go Engineer",
		"employer":   map[string]any{"name": "Acme Corp"},
		"datePosted": "2026-09-16",
		"url":        "not a url at all",
	}

	if err := validateAgainstSchema(t, schemaJobPosting, posting); err == nil {
		t.Fatal("a malformed `url` was accepted; a relative or empty origin would ship unnoticed")
	}
}

func TestVendoredSchemasCompileWithoutReachingTheNetwork(t *testing.T) {
	// The offline guarantee is a stated requirement, not a library default we inherit: the
	// compiler is given a loader that errors on every URL, so any $ref the vendored set does
	// not itself satisfy fails the compile instead of quietly fetching ojcp.dev.
	compileOnce.Do(compileVendoredSchemas)
	if compileErr != nil {
		t.Fatalf("vendored schemas did not compile offline: %v", compileErr)
	}
	if _, err := (refusingLoader{}).Load(schemaJobPosting); err == nil {
		t.Fatal("the loader wired into the compiler does not refuse a URL; offline is not pinned")
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
