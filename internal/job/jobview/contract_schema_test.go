package jobview

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ai/enrich"
)

// contractFile is the compiled HTTP contract, relative to this package. jobview (layer 5)
// cannot import the API block, so it reads the document the handlers are generated from.
const contractFile = "../../../openapi/api.openapi.json"

// The contract describes the wire shape for readers only: `Job` carries `x-raw-json`, so
// ogen hands the handlers jx.Raw and encoding/json writes the bytes. Nothing then ties
// jobview.Job to the schema, and a field added here without a typespec/job-wire.tsp edit
// is served but undocumented — which is how source_url, ai_interview_reports,
// referral_available, recruiter_contacts_available and auto_apply_status all went missing
// at once. This fails on the next one.
//
// One direction only: the schema may promise less than is served (an optional field, a
// narrower enum). The reverse — a key on the wire the contract does not name — is what a
// client generated from the contract cannot see.
//
// FromDomain folds the model's own geography, work mode and skills out of the nested
// enrichment (they are served top-level from the dictionaries); those keys are listed as
// unpublished per schema, and are the only ones allowed to be absent.
func TestJobSchemaCoversWireShape(t *testing.T) {
	raw, err := os.ReadFile(contractFile)
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}

	// Each served struct against the contract model that names it: a projection typed as
	// another model's shape (a Card as a Job: no blurb, seniority nested) is the same drift.
	for _, tc := range []struct {
		schema string
		typ    reflect.Type
		// Keys the struct can write that FromDomain strips before serving, so they are
		// not on the wire and must stay out of the contract.
		unpublished []string
	}{
		{"Job", reflect.TypeOf(Job{}), nil},
		// The listing card /me/tracking serves: its own projection, not a Job subset.
		{"JobCard", reflect.TypeOf(Card{}), nil},
		// The nested objects are served inside Job, so they are part of the same promise.
		{"JobEnrichment", reflect.TypeOf(enrich.Enrichment{}), []string{"work_mode", "regions", "countries", "cities", "skills"}},
		{"JobReality", reflect.TypeOf(Reality{}), nil},
		{"JobGhost", reflect.TypeOf(Ghost{}), nil},
	} {
		schema, ok := spec.Components.Schemas[tc.schema]
		if !ok {
			t.Errorf("contract has no %s schema", tc.schema)
			continue
		}
		for _, key := range wireKeys(tc.typ) {
			if slices.Contains(tc.unpublished, key) {
				continue
			}
			if _, documented := schema.Properties[key]; !documented {
				t.Errorf("%s serves %q (json tag on %s.%s) but the contract's %s schema does not: add it to typespec/job-wire.tsp and run make api-contract",
					tc.schema, key, tc.typ, fieldName(tc.typ, key), tc.schema)
			}
		}
	}
}

// wireKeys lists the JSON object keys encoding/json can write for a struct: its own json
// tags plus those of any anonymous structs it embeds. A `json:"-"` field is never written.
func wireKeys(typ reflect.Type) []string {
	var keys []string
	for i := range typ.NumField() {
		f := typ.Field(i)
		if f.Anonymous {
			keys = append(keys, wireKeys(f.Type)...)
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	return keys
}

// fieldName is the Go field behind a wire key, for the failure message.
func fieldName(typ reflect.Type, key string) string {
	for i := range typ.NumField() {
		f := typ.Field(i)
		if f.Anonymous {
			if n := fieldName(f.Type, key); n != "" {
				return f.Name + "." + n
			}
			continue
		}
		if name, _, _ := strings.Cut(f.Tag.Get("json"), ","); name == key {
			return f.Name
		}
	}
	return "?"
}

// Guards the guard: a key the schema lacks must fail, and the helper must see an
// omitempty field a zero value would hide. blurb is the key a Job-typed card lacked.
func TestWireKeysIncludeOmittedFields(t *testing.T) {
	keys := wireKeys(reflect.TypeOf(Job{}))
	for _, want := range []string{"public_slug", "source_url", "auto_apply_status", "closed_at"} {
		if !slices.Contains(keys, want) {
			t.Errorf("wireKeys(Job) lacks %q: %v", want, keys)
		}
	}
	if keys := wireKeys(reflect.TypeOf(Card{})); !slices.Contains(keys, "blurb") {
		t.Errorf("wireKeys(Card) lacks %q: %v", "blurb", keys)
	}
}
