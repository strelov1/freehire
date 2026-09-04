package enrich

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// TestCompanyTypeHintsAreValidEnumValues guards against a typo'd hint silently never
// matching an enum check (Enrichment.Validate rejects unknown company_type values, so a
// bad hint here would train the model on a value it also gets stripped for downstream).
func TestCompanyTypeHintsAreValidEnumValues(t *testing.T) {
	for slug, ct := range CompanyTypeHints {
		if !slices.Contains(vocab.CompanyTypeValues, ct) {
			t.Errorf("CompanyTypeHints[%q] = %q, not a member of vocab.CompanyTypeValues", slug, ct)
		}
	}
}

// TestCompanyTypeHintsKeysLookLikeSlugs catches an entry keyed by a raw display name
// instead of its normalized company_slug, which would silently never match a job.
func TestCompanyTypeHintsKeysLookLikeSlugs(t *testing.T) {
	for slug := range CompanyTypeHints {
		if slug == "" {
			t.Error("CompanyTypeHints has an empty key")
			continue
		}
		for _, r := range slug {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
				continue
			}
			t.Errorf("CompanyTypeHints key %q is not a lowercase company_slug", slug)
			break
		}
	}
}
