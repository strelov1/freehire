package llmschema

import (
	"slices"
	"strings"
	"testing"
)

// contract mirrors the shapes the real contracts use: required and optional scalars,
// an optional slice, a nested struct, a slice of structs, and the two kinds of field
// that must never be asked of a model — unexported and explicitly skipped.
type contract struct {
	Name     string     `json:"name"`
	Nickname string     `json:"nickname,omitempty"`
	Years    int        `json:"years"`
	Tags     []string   `json:"tags,omitempty"`
	Address  address    `json:"address"`
	Jobs     []position `json:"jobs,omitempty"`
	secret   string     //nolint:unused // deliberately unexported: must not reach the schema
	Skip     string     `json:"-"`
}

type address struct {
	City string `json:"city"`
}

type position struct {
	Title string `json:"title"`
	Level string `json:"level,omitempty"`
}

func TestOf_PropertiesAreExactlyTheContractsJSONTags(t *testing.T) {
	s, err := Of[contract]()
	if err != nil {
		t.Fatalf("Of: %v", err)
	}

	want := []string{"name", "nickname", "years", "tags", "address", "jobs"}
	got := propertyNames(t, s)
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("properties = %v, want %v", got, want)
	}
}

func TestOf_UnexportedAndSkippedFieldsAreAbsent(t *testing.T) {
	s, err := Of[contract]()
	if err != nil {
		t.Fatalf("Of: %v", err)
	}

	for _, absent := range []string{"secret", "Secret", "Skip", "-"} {
		if _, ok := properties(t, s)[absent]; ok {
			t.Errorf("property %q is in the schema; a model must never be asked for it", absent)
		}
	}
}

func TestOf_NestedStructsAreWalked(t *testing.T) {
	s, err := Of[contract]()
	if err != nil {
		t.Fatalf("Of: %v", err)
	}

	city := propertyNames(t, object(t, properties(t, s)["address"]))
	if !slices.Equal(city, []string{"city"}) {
		t.Fatalf("address properties = %v, want [city]", city)
	}

	item := object(t, object(t, properties(t, s)["jobs"])["items"])
	got := propertyNames(t, item)
	slices.Sort(got)
	if !slices.Equal(got, []string{"level", "title"}) {
		t.Fatalf("jobs item properties = %v, want [level title]", got)
	}
}

// Strict mode rejects a schema that leaves additionalProperties open or a property out
// of required — and it rejects them on nested objects too, which reflection alone does
// not produce.
func TestOf_EveryObjectIsStrict(t *testing.T) {
	s, err := Of[contract]()
	if err != nil {
		t.Fatalf("Of: %v", err)
	}

	walkObjects(t, s, func(path string, obj map[string]any) {
		if obj["additionalProperties"] != false {
			t.Errorf("%s: additionalProperties = %v, want false", path, obj["additionalProperties"])
		}

		names := propertyNames(t, obj)
		required := stringSlice(t, obj["required"])
		slices.Sort(names)
		slices.Sort(required)
		if !slices.Equal(required, names) {
			t.Errorf("%s: required = %v, want every property %v", path, required, names)
		}
	})
}

// Optionality cannot be expressed by omission under strict mode, so an omitempty field
// must instead admit null. A required field must NOT — otherwise the contract's
// mandatory fields silently become optional.
func TestOf_OmitemptyFieldsAdmitNullAndOthersDoNot(t *testing.T) {
	s, err := Of[contract]()
	if err != nil {
		t.Fatalf("Of: %v", err)
	}

	props := properties(t, s)
	for field, wantNullable := range map[string]bool{
		"name":     false,
		"years":    false,
		"address":  false,
		"nickname": true,
		"tags":     true,
		"jobs":     true,
	} {
		if got := propAdmitsNull(t, props[field]); got != wantNullable {
			t.Errorf("%s admits null = %v, want %v", field, got, wantNullable)
		}
	}
}

func TestEnum_ConstrainsOneFieldAndLeavesSiblingsAlone(t *testing.T) {
	levels := []string{"junior", "middle", "senior"}

	s, err := Of[contract](Enum("name", levels))
	if err != nil {
		t.Fatalf("Of: %v", err)
	}

	props := properties(t, s)
	if got := stringSlice(t, object(t, props["name"])["enum"]); !slices.Equal(got, levels) {
		t.Fatalf("name enum = %v, want %v", got, levels)
	}
	if _, ok := object(t, props["years"])["enum"]; ok {
		t.Error("years carries an enum; an override must touch only the field it names")
	}
}

// On an array field the vocabulary constrains each element. Put on the array itself it
// would say the whole list equals one of the values, which no model could satisfy.
func TestEnum_OnAnArrayFieldConstrainsTheItems(t *testing.T) {
	regions := []string{"eu", "apac"}

	s, err := Of[contract](Enum("tags", regions))
	if err != nil {
		t.Fatalf("Of: %v", err)
	}

	tags := object(t, properties(t, s)["tags"])
	if _, ok := tags["enum"]; ok {
		t.Error("the array itself carries an enum; the constraint belongs on its items")
	}

	items, ok := tags["items"].(map[string]any)
	if !ok {
		t.Fatal("tags carries no items schema")
	}
	if got := stringSlice(t, items["enum"]); !slices.Equal(got, regions) {
		t.Errorf("items enum = %v, want %v", got, regions)
	}
}

// enum is the narrower of the two constraints: on a field whose type admits null, a
// vocabulary that omitted null would forbid the very absence the contract permits,
// leaving the model no legal way to decline.
func TestEnum_NullableFieldKeepsNullAmongItsValues(t *testing.T) {
	s, err := Of[contract](Enum("nickname", []string{"junior", "senior"}))
	if err != nil {
		t.Fatalf("Of: %v", err)
	}

	values, ok := object(t, properties(t, s)["nickname"])["enum"].([]any)
	if !ok {
		t.Fatal("nickname carries no enum")
	}
	if !slices.Contains(values, nil) {
		t.Fatalf("nickname enum = %v, want null among the values", values)
	}
}

// A contract can hold fields the model must never supply — résumé contacts come from
// deterministic PII detection over text the model never sees. Under strict mode every
// property is required, so leaving such a field in the schema would order the model to
// invent it.
func TestOmit_RemovesAFieldFromPropertiesAndRequired(t *testing.T) {
	s, err := Of[contract](Omit("name", "years"))
	if err != nil {
		t.Fatalf("Of: %v", err)
	}

	for _, gone := range []string{"name", "years"} {
		if _, ok := properties(t, s)[gone]; ok {
			t.Errorf("property %q survived Omit", gone)
		}
		if slices.Contains(stringSlice(t, object(t, s)["required"]), gone) {
			t.Errorf("required still lists the omitted %q, which strict mode would demand", gone)
		}
	}
	if _, ok := properties(t, s)["nickname"]; !ok {
		t.Error("Omit removed a field it was not given")
	}
}

// Constrained decoding coerces a number to the declared type but rounds the value.
// Where the receiving decoder truncates on purpose — a fractional year count must not
// round up into experience the candidate does not have — the field is asked for as
// text and the arithmetic stays on this side.
func TestAsText_RetypesAFieldToStringKeepingItsNullability(t *testing.T) {
	s, err := Of[contract](AsText("years"))
	if err != nil {
		t.Fatalf("Of: %v", err)
	}

	years := object(t, properties(t, s)["years"])
	if years["type"] != "string" {
		t.Errorf("years type = %v, want string", years["type"])
	}
	if propAdmitsNull(t, properties(t, s)["years"]) {
		t.Error("years became nullable; AsText must not change whether a field is optional")
	}
}

func TestAsText_KeepsAnOptionalFieldOptional(t *testing.T) {
	s, err := Of[contract](AsText("tags"))
	if err != nil {
		t.Fatalf("Of: %v", err)
	}

	if !propAdmitsNull(t, properties(t, s)["tags"]) {
		t.Error("an omitempty field lost its null after AsText, leaving the model no way to decline")
	}
}

func TestAsText_UnknownFieldIsAnError(t *testing.T) {
	if _, err := Of[contract](AsText("no_such_field")); err == nil {
		t.Fatal("Of returned no error for an AsText naming a field the contract lacks")
	}
}

func TestOmit_UnknownFieldIsAnError(t *testing.T) {
	if _, err := Of[contract](Omit("no_such_field")); err == nil {
		t.Fatal("Of returned no error for an omit naming a field the contract lacks")
	}
}

// An override that silently does nothing is the dangerous failure: the field keeps
// generating freely and only the validator downstream would ever notice.
func TestEnum_UnknownFieldIsAnError(t *testing.T) {
	if _, err := Of[contract](Enum("no_such_field", []string{"a"})); err == nil {
		t.Fatal("Of returned no error for an override naming a field the contract lacks")
	}
}

// A map reflects to an object with an open additionalProperties and no property list,
// which strict mode cannot express. Failing here names the contract; letting it through
// would surface as a 400 from the gateway, far from the type that caused it.
func TestOf_MapTypedFieldIsAnError(t *testing.T) {
	type withMap struct {
		Name  string            `json:"name"`
		Extra map[string]string `json:"extra"`
	}

	_, err := Of[withMap]()
	if err == nil {
		t.Fatal("Of accepted a map-typed field, which strict mode cannot close")
	}
	if !strings.Contains(err.Error(), "extra") {
		t.Errorf("error = %q, want it to name the offending field", err)
	}
}

func TestOf_NonStructTypeIsAnError(t *testing.T) {
	if _, err := Of[string](); err == nil {
		t.Fatal("Of returned no error for a non-struct contract type")
	}
}

// --- helpers ---

func properties(t *testing.T, s any) map[string]any {
	t.Helper()

	props, ok := object(t, s)["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema has no properties object: %v", s)
	}

	return props
}

func propertyNames(t *testing.T, s any) []string {
	t.Helper()

	names := []string{}
	for name := range properties(t, s) {
		names = append(names, name)
	}

	return names
}

func object(t *testing.T, v any) map[string]any {
	t.Helper()

	switch m := v.(type) {
	case Schema:
		return m
	case map[string]any:
		return m
	default:
		t.Fatalf("value is not an object: %#v", v)
		return nil
	}
}

// stringSlice reads an array of strings in either form the schema uses: a []string
// the strict pass wrote itself, or the []any that survives a JSON round trip.
func stringSlice(t *testing.T, v any) []string {
	t.Helper()

	switch arr := v.(type) {
	case []string:
		return arr
	case []any:
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			s, ok := item.(string)
			if !ok {
				t.Fatalf("array item is not a string: %#v", item)
			}
			out = append(out, s)
		}

		return out
	default:
		t.Fatalf("value is not an array: %#v", v)
		return nil
	}
}

// propAdmitsNull reports whether a property's type accepts null, in either the scalar or
// the array form JSON Schema allows.
func propAdmitsNull(t *testing.T, prop any) bool {
	t.Helper()

	switch typ := object(t, prop)["type"].(type) {
	case string:
		return typ == "null"
	case []any:
		return slices.Contains(stringSlice(t, typ), "null")
	default:
		t.Fatalf("property has no type: %#v", prop)
		return false
	}
}

// walkObjects visits the root schema and every nested object schema, including array
// item schemas, so a strictness assertion covers the whole document rather than its top level.
func walkObjects(t *testing.T, s any, fn func(path string, obj map[string]any)) {
	t.Helper()

	var walk func(path string, node map[string]any)
	walk = func(path string, node map[string]any) {
		if node["type"] == "object" || node["properties"] != nil {
			fn(path, node)
		}

		if props, ok := node["properties"].(map[string]any); ok {
			for name, child := range props {
				walk(path+"."+name, object(t, child))
			}
		}
		if items, ok := node["items"]; ok {
			walk(path+"[]", object(t, items))
		}
	}

	walk("$", object(t, s))
}
