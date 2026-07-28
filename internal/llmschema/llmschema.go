// Package llmschema derives the JSON Schema a model is constrained by from the Go
// contract type the response is decoded into, so the two cannot drift. A schema
// written by hand beside a contract is a second description of the same shape, free
// to fall behind it silently: a schema missing a field simply stops the model
// returning it, with nothing to fail.
//
// The output targets OpenAI-style structured outputs in strict mode, which reflection
// alone does not satisfy. Strict mode demands `additionalProperties: false` on every
// object — nested ones included — and every property listed in `required`, leaving no
// absent key to express optionality with. Optional fields are therefore widened to
// admit null, which decodes into the same zero value an absent key produced.
package llmschema

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/invopop/jsonschema"
)

// Schema is a JSON Schema document, shaped for transport as the `schema` member of a
// `json_schema` response format. It is a plain map because strict mode needs types
// expressed as `["string", "null"]`, which the reflected struct's single-string Type
// field cannot hold.
type Schema map[string]any

// Override adjusts a derived schema before it is returned. It reports an error rather
// than doing nothing when it cannot apply — an override that silently misses is the
// dangerous case, because the field goes on generating freely and only the validator
// downstream would ever notice.
type Override func(Schema) error

// Of derives a strict-mode schema from contract type T and applies the overrides in
// order. T must be a struct.
func Of[T any](overrides ...Override) (Schema, error) {
	var zero T
	if k := reflect.TypeOf(&zero).Elem().Kind(); k != reflect.Struct {
		return nil, fmt.Errorf("llmschema: contract type must be a struct, got %s", k)
	}

	reflector := jsonschema.Reflector{
		// A flat document keeps the strict pass local: with $defs and $ref the same
		// nested type could be reached through several paths, and a shared definition
		// would have to be strict for all of them at once.
		DoNotReference: true,
		ExpandedStruct: true,
	}

	raw, err := json.Marshal(reflector.Reflect(zero))
	if err != nil {
		return nil, fmt.Errorf("llmschema: marshal reflected schema: %w", err)
	}

	schema := Schema{}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("llmschema: decode reflected schema: %w", err)
	}

	// Metadata the reflector adds about itself; a response format carries the schema alone.
	delete(schema, "$schema")
	delete(schema, "$id")

	if err := strict(schema); err != nil {
		return nil, err
	}

	for _, override := range overrides {
		if err := override(schema); err != nil {
			return nil, err
		}
	}

	return schema, nil
}

// Enum constrains one top-level field to a controlled vocabulary, so a value outside
// it cannot be generated rather than being discarded after the fact. The vocabulary
// stays wherever it is defined; naming it here keeps contract structs free of a
// second copy that no compiler would catch drifting.
func Enum(field string, values []string) Override {
	return func(schema Schema) error {
		prop, ok := propertyOf(schema, field)
		if !ok {
			return fmt.Errorf("llmschema: enum override names %q, which the contract type has no field for", field)
		}

		allowed := make([]any, 0, len(values)+1)
		for _, v := range values {
			allowed = append(allowed, v)
		}

		// On an array the vocabulary constrains each element: put on the array itself
		// it would demand the whole list equal one of the values.
		if items, ok := prop["items"].(map[string]any); ok {
			items["enum"] = allowed

			return nil
		}

		// A nullable field needs null inside the enum too: enum is the narrower
		// constraint, and would otherwise forbid the very absence the type permits.
		if admitsNull(prop) {
			allowed = append(allowed, nil)
		}
		prop["enum"] = allowed

		return nil
	}
}

// AsText retypes top-level fields to string, keeping whatever nullability they had.
// Use it where the contract's Go type would hand the arithmetic to the model: asked
// for an integer, a model given "5.9 years" returns 6, while a decoder that truncates
// on purpose returns 5 — and the difference is experience the candidate does not have.
func AsText(fields ...string) Override {
	return func(schema Schema) error {
		for _, field := range fields {
			prop, ok := propertyOf(schema, field)
			if !ok {
				return fmt.Errorf("llmschema: text override names %q, which the contract type has no field for", field)
			}

			// Read the nullability before overwriting the type, or it is gone.
			optional := admitsNull(prop)

			prop["type"] = "string"
			if optional {
				widenToNull(prop)
			}
		}

		return nil
	}
}

// Omit drops top-level fields from the schema, for the parts of a contract the model
// is not the source of. Strict mode requires every property, so a field left in is a
// field the model is ordered to produce — for a value it may have no honest way to
// know, such as a contact detail redacted out of the text it was given.
func Omit(fields ...string) Override {
	return func(schema Schema) error {
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			return fmt.Errorf("llmschema: omit override on a schema with no properties")
		}

		for _, field := range fields {
			if _, ok := props[field]; !ok {
				return fmt.Errorf("llmschema: omit override names %q, which the contract type has no field for", field)
			}
			delete(props, field)
		}

		required, ok := schema["required"].([]string)
		if !ok {
			return fmt.Errorf("llmschema: omit override on a schema whose required list is not the strict pass's")
		}
		schema["required"] = slices.DeleteFunc(required, func(name string) bool {
			return slices.Contains(fields, name)
		})

		return nil
	}
}

// strict rewrites node and everything below it for strict mode: no additional
// properties, every property required, and each property the reflector left optional
// widened to admit null.
//
// It reports an error for a node it cannot make strict — a map field, say, which
// reflects to an object with an open additionalProperties and no property list. Left
// alone that document would be rejected by the provider at call time, hundreds of
// lines from the contract that produced it.
func strict(node map[string]any) error {
	props, ok := node["properties"].(map[string]any)
	if !ok {
		if items, ok := node["items"].(map[string]any); ok {
			return strict(items)
		}
		if err := rejectOpenObject(node); err != nil {
			return err
		}

		return nil
	}

	optional := optionalFields(node, props)

	names := make([]string, 0, len(props))
	for name, child := range props {
		names = append(names, name)

		prop, ok := child.(map[string]any)
		if !ok {
			continue
		}
		if optional[name] {
			widenToNull(prop)
		}
		if err := strict(prop); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	slices.Sort(names)

	node["required"] = names
	node["additionalProperties"] = false

	return nil
}

// rejectOpenObject fails on an object node the strict pass cannot close: one whose
// additionalProperties is a schema (a Go map) rather than the bool it would set.
func rejectOpenObject(node map[string]any) error {
	if node["type"] != "object" {
		return nil
	}
	if _, isSchema := node["additionalProperties"].(map[string]any); !isSchema {
		return nil
	}

	return errors.New("llmschema: contract holds a map-typed field, which strict mode cannot express")
}

// optionalFields returns the properties the reflector did NOT mark required — the
// `omitempty` fields. It must be read before required is rewritten, since afterwards
// every field is required and the distinction is gone.
func optionalFields(node map[string]any, props map[string]any) map[string]bool {
	required := map[string]bool{}
	if list, ok := node["required"].([]any); ok {
		for _, item := range list {
			if name, ok := item.(string); ok {
				required[name] = true
			}
		}
	}

	optional := make(map[string]bool, len(props))
	for name := range props {
		optional[name] = !required[name]
	}

	return optional
}

// widenToNull adds null to a property's type, in whichever of the two forms JSON
// Schema allows it to already be written.
func widenToNull(prop map[string]any) {
	switch typ := prop["type"].(type) {
	case string:
		if typ != "null" {
			prop["type"] = []any{typ, "null"}
		}
	case []any:
		if !slices.Contains(typ, any("null")) {
			prop["type"] = append(typ, "null")
		}
	}
}

func admitsNull(prop map[string]any) bool {
	switch typ := prop["type"].(type) {
	case string:
		return typ == "null"
	case []any:
		return slices.Contains(typ, any("null"))
	default:
		return false
	}
}

func propertyOf(schema Schema, field string) (map[string]any, bool) {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil, false
	}

	prop, ok := props[field].(map[string]any)

	return prop, ok
}
