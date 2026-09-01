package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// skill is one canonical as the prompt sees it: the slug, how it is written for a
// reader, and the spellings the parser accepts for it.
//
// The slug alone is a poor prompt. "1c", "as400" and "dbt" are unrecognisable without
// the label, and the aliases are what disambiguate a short slug from the several things
// it could name — "ml" is machine learning here and not a markup language, and the
// alias list is the evidence for that.
type skill struct {
	canonical string
	label     string
	aliases   []string
}

// drafter is the slice of the LLM client this worker uses. Narrow on purpose: the
// generator is a prompt and a sanitiser, and taking the concrete client would make the
// tests need an endpoint to say anything about either.
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
// for the TSV, or an error when the answer cannot be one.
//
// Nothing here writes to descriptions.tsv. The worker prints, a human edits, and the
// edited text is what ships — which is the only thing that makes "curated" a property
// of the file rather than a claim about it.
func draft(ctx context.Context, d drafter, s skill) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "slug: %s\nwritten as: %s\n", s.canonical, s.label)
	if len(s.aliases) > 0 {
		fmt.Fprintf(&b, "spellings that resolve to it: %s\n", strings.Join(s.aliases, ", "))
	}

	// A schema when one can be built, and the call still goes out without it if not: the
	// schema forecloses the shape drift wave 1 met, and failing the whole wave because a
	// schema could not be derived would be a worse trade than sending the prompt bare.
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
	// The dictionary is one row per skill, so a wrapped answer would break the file it
	// is destined for. Collapsing beats rejecting: a model putting a sentence on two
	// lines is not a reason to lose the sentence.
	line := strings.Join(strings.Fields(description), " ")
	if line == "" {
		return "", fmt.Errorf("drafting %s: model returned no description in %q", s.canonical, raw)
	}
	return line, nil
}

// wrapperKeys are the field names a gateway has been seen to hand the answer back under.
// A closed list on purpose: it is the difference between unwrapping an envelope and
// treating any single-field object as one.
var wrapperKeys = map[string]bool{"answer": true, "response": true, "result": true, "output": true}

// describedIn pulls the description out of the model's answer, tolerating one layer of
// gateway envelope.
//
// The wave-1 run met exactly that: a gateway returned
// `{"answer": "{\"description\": \"…\"}"}` — the model's object, correct in itself,
// handed back as a STRING under the gateway's own key. WithSchema is the first line
// against it, and internal/platform/llm/AGENTS.md is explicit that it is not a proof:
// a gateway that stops honouring a schema still answers 200.
//
// One layer, not any number. A wrapper around a wrapper is a shape this has never seen,
// and unwrapping until something parses would turn an unknown response into a plausible
// sentence — the operator should get an error they can read instead.
func describedIn(raw string) (string, error) {
	var answer answerShape
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		return "", fmt.Errorf("decoding %q: %w", raw, err)
	}
	if answer.Description != "" {
		return answer.Description, nil
	}

	// No description at the top level, so look one field down. One production run
	// produced all three shapes below, which is why this is a list of what was seen
	// rather than a general unwrapper.
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil || len(envelope) != 1 {
		return "", nil // not an envelope shape; the caller reports the empty answer
	}
	for key, inner := range envelope {
		// Only a key a gateway is known to wrap in. Accepting any single key would make
		// {"error": "I cannot help with that"} a glossary entry — the model's refusal
		// printed as a definition, which is exactly the shape a reviewer skims past.
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
		// {"answer": "…"} — the sentence itself under the gateway's key. Accepted
		// because the schema asked for exactly one string and this is it; anything
		// deeper is a shape nobody has seen and stays an error the operator can read.
		return text, nil
	}
	return "", nil
}
