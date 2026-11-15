// Package llmschema derives the JSON Schema a model is constrained by from the Go contract
// type the response is decoded into, so the two cannot drift (a hand-written schema missing a
// field silently stops the model returning it).
//
// The output targets OpenAI-style strict mode: `additionalProperties: false` on every object,
// nested ones included, and every property `required`. Optional fields are therefore widened
// to admit null, which decodes to the same zero value an absent key gave.
package llmschema

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/google/jsonschema-go/jsonschema"
)

// Schema is a JSON Schema document for the `schema` member of a `json_schema` response format.
// It is a plain map because strict mode needs `["string", "null"]` types, which the reflected
// struct's single-string Type cannot hold.
type Schema map[string]any

// Override adjusts a derived schema. It must error when it cannot apply: a silent miss leaves
// the field generating freely and only the downstream validator would notice.
type Override func(Schema) error

// Of derives a strict-mode schema from contract type T and applies the overrides in
// order. T must be a struct.
func Of[T any](overrides ...Override) (Schema, error) {
	var zero T
	if k := reflect.TypeOf(&zero).Elem().Kind(); k != reflect.Struct {
		return nil, fmt.Errorf("llmschema: contract type must be a struct, got %s", k)
	}

	// For inlines nested types (no $defs/$ref), keeping the strict pass local.
	reflected, err := jsonschema.For[T](nil)
	if err != nil {
		return nil, fmt.Errorf("llmschema: reflect schema: %w", err)
	}

	raw, err := json.Marshal(reflected)
	if err != nil {
		return nil, fmt.Errorf("llmschema: marshal reflected schema: %w", err)
	}

	schema := Schema{}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("llmschema: decode reflected schema: %w", err)
	}

	// The reflector marks every slice and pointer nullable; strict re-adds null only for optional fields.
	dropNull(schema)

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

// Enum constrains one top-level field to a controlled vocabulary, so a value outside it cannot
// be generated. The vocabulary stays where it is defined, not copied into the contract struct.
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

		// On an array it constrains each element, not the whole list.
		if items, ok := prop["items"].(map[string]any); ok {
			items["enum"] = allowed

			return nil
		}

		// A nullable field needs null in the enum too, or enum forbids the absence the type permits.
		if admitsNull(prop) {
			allowed = append(allowed, nil)
		}
		prop["enum"] = allowed

		return nil
	}
}

// AsText retypes top-level fields to string, keeping their nullability. Use it where an integer
// type would hand rounding to the model: "5.9 years" becomes 6, while our truncating decoder
// gives 5, and the difference is experience the candidate does not have.
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

// Omit drops top-level fields the model is not the source of. Strict mode requires every
// property, so a field left in is one the model must produce, even a redacted contact detail
// it cannot honestly know.
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

// strict rewrites node and everything below it for strict mode: no additional properties,
// every property required, optional ones widened to admit null. It errors on a node it cannot
// make strict (a map field), which the provider would otherwise reject at call time.
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

// dropNull removes "null" from every type list in the tree, collapsing a one-element
// list back to a plain string.
func dropNull(node map[string]any) {
	if types, ok := node["type"].([]any); ok {
		kept := slices.DeleteFunc(slices.Clone(types), func(t any) bool { return t == "null" })
		if len(kept) == 1 {
			node["type"] = kept[0]
		} else if len(kept) > 1 {
			node["type"] = kept
		}
	}
	if props, ok := node["properties"].(map[string]any); ok {
		for _, child := range props {
			if prop, ok := child.(map[string]any); ok {
				dropNull(prop)
			}
		}
	}
	if items, ok := node["items"].(map[string]any); ok {
		dropNull(items)
	}
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

// optionalFields returns the properties the reflector did NOT mark required (`omitempty`).
// Read it before required is rewritten.
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

// widenToNull adds null to a property's type, whether written as a string or a list.
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
