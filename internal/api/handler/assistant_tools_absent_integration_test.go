//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// A tool whose surface is not wired must REFUSE, and every preset is offered the discovery
// and tracking tools — including the tailoring autopilot, which is where this was found.
//
// The guards read `h.search.search == nil` and `h.tracking.tracking == nil`, which
// dereference the outer handler before testing anything. When that handler is itself nil —
// which the assistant's own constructor permits, guarding `cvH != nil` three lines below —
// the check panics instead of refusing. It lands in the SSE stream's goroutine, so the
// candidate sees a turn that died mid-answer.
//
// Found by the model bake-off: a real model calls search_jobs during a tailoring run, which
// no scripted stand-in in this package ever did.
func TestAToolWithNoSurfaceRefusesInsteadOfPanicking(t *testing.T) {
	// Everything CORE present, every OPTIONAL surface absent. That is the shape the guards
	// claim to handle and the shape the assistant's own constructor permits: search,
	// tracking, profile, cv and mail arrive as pointers that may be nil, and it guards cvH
	// itself three lines below. queries is not in that set — a nil one means the handler
	// was never assembled at all, which no deployment produces — so it is real here and
	// the tools that read it fail on their data rather than on their wiring.
	pool := startPostgres(t)
	h := &assistantHandlers{queries: db.New(pool), jobs: db.New(pool)}

	for _, tool := range append(h.assistantDiscoveryTools(), h.assistantTrackingTools()...) {
		t.Run(tool.Name, func(t *testing.T) {
			// Arguments built from the tool's OWN schema, for two reasons. A hand-written
			// map would cover only the tools whoever wrote it remembered — the same trap
			// that hid get_profile from the grep that found the rest. And DecodeArgs
			// disallows unknown fields, so one superset blob refuses at every tool and the
			// test passes having exercised nothing.
			args := argsFromSchema(t, tool.Schema)
			// The panic is the finding: it happens inside tool.Run, and without a recover
			// here the whole test binary dies on the first one rather than reporting which
			// tools are affected.
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s panicked with no surface wired: %v", tool.Name, r)
				}
			}()
			_, err := tool.Run(context.Background(), 1, args)
			if err == nil {
				t.Fatalf("%s answered with no surface wired; it must refuse", tool.Name)
			}
			// The wording is checked only where the surface is what refused. A tool that
			// still declined on its arguments under this blob is reported rather than
			// asserted on: its guard was not reached, so this run says nothing about it.
			// A refusal that is still about the ARGUMENTS never reached the guard under
			// test, so it is a hole in this measurement and is reported as one rather than
			// passing quietly.
			if strings.Contains(err.Error(), badArgumentsPrefix) {
				t.Errorf("%s refused with %q — the schema-built arguments do not satisfy it, "+
					"so its surface guard was never reached", tool.Name, err)
			}
		})
	}
}

// argsFromSchema builds one minimal valid argument object from a tool's declared JSON
// schema: every declared property, filled with a value of the type it declares.
//
// It emits only DECLARED properties, which is what makes it usable at all — DecodeArgs
// disallows unknown fields. Filling every property rather than only the required ones is
// deliberate: `required` is absent from most of these schemas, and a tool that validates a
// field beyond decoding it (a slug that must be non-empty) needs the value present.
func argsFromSchema(t *testing.T, schema map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(valueForSchema(schema))
	if err != nil {
		t.Fatalf("build arguments from schema: %v", err)
	}
	return raw
}

// valueForSchema produces one value of the type a schema fragment declares.
func valueForSchema(schema map[string]any) any {
	switch schema["type"] {
	case "object":
		props, _ := schema["properties"].(map[string]any)
		out := make(map[string]any, len(props))
		for name, sub := range props {
			s, ok := sub.(map[string]any)
			if !ok {
				continue
			}
			out[name] = valueForProperty(name, s)
		}
		return out
	case "array":
		items, _ := schema["items"].(map[string]any)
		if items == nil {
			return []any{}
		}
		// Exactly one element: a tool that requires a non-empty list is satisfied, and one
		// that does not is unaffected.
		return []any{valueForSchema(items)}
	case "integer", "number":
		return 1
	case "boolean":
		return false
	default:
		return "x"
	}
}

// valueForProperty fills one property, preferring a value the schema itself names.
//
// A closed enum is the case that matters: a filter or a stage validated against its own
// vocabulary would refuse "x", and that refusal is about the argument rather than about the
// missing surface — exactly the hole this test reports.
func valueForProperty(name string, schema map[string]any) any {
	if enum, ok := schema["enum"].([]string); ok && len(enum) > 0 {
		return enum[0]
	}
	if enum, ok := schema["enum"].([]any); ok && len(enum) > 0 {
		return enum[0]
	}
	// A description naming the accepted words is how these schemas actually spell an enum
	// (see the tracking tools' `filter`), and a value outside it is refused as an argument.
	if desc, ok := schema["description"].(string); ok && schema["type"] == "string" {
		if word := firstQuotedVocabularyWord(name, desc); word != "" {
			return word
		}
	}
	return valueForSchema(schema)
}

// firstQuotedVocabularyWord reads "Which slice to list: all, viewed, saved, …" and returns
// the first listed word. It is a narrow reader for the one shape these schemas use, and
// returns "" when it does not recognise one — the test then reports the resulting argument
// refusal rather than silently passing.
func firstQuotedVocabularyWord(_ string, desc string) string {
	_, after, ok := strings.Cut(desc, ": ")
	if !ok {
		return ""
	}
	first, _, ok := strings.Cut(after, ",")
	if !ok {
		return ""
	}
	first = strings.TrimSpace(first)
	// One lowercase word, or it is prose rather than a vocabulary.
	if first == "" || strings.ContainsAny(first, " .") || first != strings.ToLower(first) {
		return ""
	}
	return first
}
