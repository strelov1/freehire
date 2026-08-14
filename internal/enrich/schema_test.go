package enrich

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/vocab"
)

func schemaProps(t *testing.T, askGeo bool) map[string]any {
	t.Helper()

	s, _, err := requestSchema(askGeo)
	if err != nil {
		t.Fatalf("requestSchema: %v", err)
	}

	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema carries no properties")
	}

	return props
}

func enumOf(t *testing.T, prop any) []string {
	t.Helper()

	obj, ok := prop.(map[string]any)
	if !ok {
		t.Fatalf("property is not an object: %#v", prop)
	}

	// On an array the vocabulary sits on the items.
	raw := obj["enum"]
	if items, ok := obj["items"].(map[string]any); ok {
		raw = items["enum"]
	}

	values, ok := raw.([]any)
	if !ok {
		t.Fatalf("property carries no enum: %#v", obj)
	}

	out := make([]string, 0, len(values))
	for _, v := range values {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}

	return out
}

// The schema must ask for exactly what the prompt asks for. A field left in is a field
// strict mode orders the model to produce, for a value jobview then discards.
func TestRequestSchema_DoesNotAskForDictionaryCoveredFacets(t *testing.T) {
	props := schemaProps(t, true)

	for _, field := range unaskedFields {
		if _, ok := props[field]; ok {
			t.Errorf("schema asks for %q, which the deterministic dictionaries serve", field)
		}
	}
}

func TestRequestSchema_CarriesTheFieldsThePromptAsksFor(t *testing.T) {
	props := schemaProps(t, true)

	for _, field := range []string{
		"summary", "relocation", "visa_sponsorship", "cities", "timezone_note",
		"salary_min", "salary_max", "salary_currency", "salary_period",
		"domains", "company_type", "company_size", "regions", "countries", "skills",
	} {
		if _, ok := props[field]; !ok {
			t.Errorf("schema is missing %q, so the model would stop returning it", field)
		}
	}
}

// GeoPinned means the dictionary already settled the geography and geoFacet discards
// the model's copy; the schema drops the fields exactly as the prompt does.
func TestRequestSchema_DropsGeographyWhenThePromptDoes(t *testing.T) {
	props := schemaProps(t, false)

	for _, field := range geoFields {
		if _, ok := props[field]; ok {
			t.Errorf("the geo-pinned schema still asks for %q", field)
		}
	}
	if _, ok := props["cities"]; !ok {
		t.Error("cities was dropped with the geo fields; the prompt still asks for it")
	}
}

func TestRequestSchema_PinsServedEnumsToTheirVocabularies(t *testing.T) {
	props := schemaProps(t, true)

	for field, want := range map[string][]string{
		"relocation":    vocab.RelocationValues,
		"salary_period": vocab.SalaryPeriodValues,
		"domains":       vocab.DomainValues,
		"company_type":  vocab.CompanyTypeValues,
		"company_size":  vocab.CompanySizeValues,
	} {
		got := enumOf(t, props[field])
		if !slices.Equal(got, want) {
			t.Errorf("%s enum = %v, want the vocabulary %v", field, got, want)
		}
	}
}

// Regions and countries are the dict-then-LLM discovery facets: the prompt explicitly
// allows the model a label of its own when none fits, and an enum would foreclose the
// novel value that mechanism exists to collect.
func TestRequestSchema_LeavesDiscoveryFacetsUnconstrained(t *testing.T) {
	props := schemaProps(t, true)

	for _, field := range geoFields {
		obj, ok := props[field].(map[string]any)
		if !ok {
			t.Fatalf("%s is not an object", field)
		}
		if _, ok := obj["enum"]; ok {
			t.Errorf("%s carries an enum, which would foreclose a discovered label", field)
		}
		if items, ok := obj["items"].(map[string]any); ok {
			if _, ok := items["enum"]; ok {
				t.Errorf("%s items carry an enum, which would foreclose a discovered label", field)
			}
		}
	}
}

// Strict mode has no absent key, so an unstated field arrives as null. It must land
// exactly where an omitted key landed: unset, not written back.
func TestParseEnrichment_TreatsExplicitNullAsUnset(t *testing.T) {
	raw := `{"summary":"A backend role.","relocation":null,"salary_min":null,` +
		`"visa_sponsorship":null,"domains":null,"company_size":null}`

	got, err := parseEnrichment(raw)
	if err != nil {
		t.Fatalf("parseEnrichment: %v", err)
	}

	if got.Summary != "A backend role." {
		t.Errorf("summary = %q, want the stated value", got.Summary)
	}
	if got.Relocation != "" || got.CompanySize != "" {
		t.Errorf("null strings became %q/%q, want empty", got.Relocation, got.CompanySize)
	}
	if got.SalaryMin != nil || got.VisaSponsorship != nil {
		t.Errorf("null pointers became %v/%v, want nil", got.SalaryMin, got.VisaSponsorship)
	}
	if len(got.Domains) != 0 {
		t.Errorf("null array became %v, want empty", got.Domains)
	}
}
