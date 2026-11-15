package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// skill is one canonical as the prompt sees it: the slug, its reader-facing label, and the
// spellings the parser accepts. The slug alone is a poor prompt ("1c", "as400"); the label
// and aliases disambiguate it ("ml" is machine learning here, not a markup language).
type skill struct {
	canonical string
	label     string
	aliases   []string
}

// drafter is the slice of the LLM client this worker uses, so tests need no endpoint.
type drafter interface {
	GenerateJSON(ctx context.Context, system, user string, opts ...llm.GenOption) (string, error)
}

const draftSystem = `You write glossary entries for an IT job board.

Given one skill from a controlled vocabulary, write ONE or TWO sentences saying what it
IS, for a reader who has never heard of it. Name the category of thing it is (a
language, a database, a certification, a cloud service, an ERP module) and what it is
used for.

Rules:
- BEGIN WITH THE TERM. "Kubernetes is an open-source system that…", not "It is an
  open-source system that…" or "This platform…". The entry is quoted on its own in
  search results and by assistants, where "It is a project management methodology"
  names nothing.
- Plain language. No marketing, no adjectives like "powerful" or "popular".
- Naming the term is not restating it: say what CATEGORY it belongs to and what it does.
  "Kubernetes is a Kubernetes tool" is the failure to avoid, not "Kubernetes is a…".
- Do not say how in-demand it is, what it pays, or who should learn it.
- If the name is ambiguous, describe the meaning the listed spellings point at.
- English. No markdown, no line breaks.

Answer as JSON: {"description": "..."}`

// draft asks the model for one skill's description and returns it as a single line fit
// for the TSV, or an error when the answer cannot be one. Nothing here writes
// descriptions.tsv: a human edits the output, which is what keeps it curated.
func draft(ctx context.Context, d drafter, s skill) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "slug: %s\nwritten as: %s\n", s.canonical, s.label)
	if len(s.aliases) > 0 {
		fmt.Fprintf(&b, "spellings that resolve to it: %s\n", strings.Join(s.aliases, ", "))
	}

	// Send the call bare when no schema can be built: failing the whole wave would be worse.
	var opts []llm.GenOption
	if schema, err := requestSchema(); err == nil {
		opts = append(opts, llm.WithSchema(schemaName, schema))
	} else {
		fmt.Fprintln(os.Stderr, err)
	}

	raw, err := d.GenerateJSON(ctx, draftSystem, b.String(), opts...)
	if err != nil {
		return "", fmt.Errorf("drafting %s: %w", s.canonical, err)
	}

	description, err := describedIn(raw)
	if err != nil {
		return "", fmt.Errorf("drafting %s: %w", s.canonical, err)
	}
	// The dictionary is one row per skill, so collapse a wrapped answer rather than reject it.
	line := strings.Join(strings.Fields(description), " ")
	if line == "" {
		return "", fmt.Errorf("drafting %s: model returned no description in %q", s.canonical, raw)
	}
	return line, nil
}

// wrapperKeys are the field names a gateway has been seen to hand the answer back under.
// A closed list, so only a known envelope is unwrapped, not any single-field object.
var wrapperKeys = map[string]bool{"answer": true, "response": true, "result": true, "output": true}

// describedIn pulls the description out of the model's answer, tolerating one layer of
// gateway envelope, e.g. `{"answer": "{\"description\": \"…\"}"}` (the model's object
// handed back as a string). WithSchema is the first line against that but not a proof: a
// gateway that stops honouring a schema still answers 200 (internal/platform/llm/AGENTS.md).
// One layer only: unwrapping until something parses would turn an unknown response into a
// plausible sentence.
func describedIn(raw string) (string, error) {
	var answer answerShape
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		return "", fmt.Errorf("decoding %q: %w", raw, err)
	}
	if answer.Description != "" {
		return answer.Description, nil
	}

	// No description at the top level, so look one field down. A list of seen shapes, not a
	// general unwrapper.
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil || len(envelope) != 1 {
		return "", nil // not an envelope shape; the caller reports the empty answer
	}
	for key, inner := range envelope {
		// Only a known wrapper key: accepting any would turn {"error": "I cannot help with
		// that"} into a glossary entry.
		if !wrapperKeys[key] {
			return "", nil
		}
		// {"answer": {"description": "…"}} — the object, nested.
		if err := json.Unmarshal(inner, &answer); err == nil && answer.Description != "" {
			return answer.Description, nil
		}
		var text string
		if err := json.Unmarshal(inner, &text); err != nil {
			return "", nil
		}
		// {"answer": "{\"description\": \"…\"}"} — the object, stringified.
		if err := json.Unmarshal([]byte(text), &answer); err == nil && answer.Description != "" {
			return answer.Description, nil
		}
		// {"answer": "…"} — the sentence itself under the gateway's key. Accepted because the
		// schema asked for exactly one string; anything deeper stays an error.
		return text, nil
	}
	return "", nil
}
