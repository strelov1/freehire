package main

import (
	"context"
	"encoding/json"
	"fmt"
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
- Plain language. No marketing, no adjectives like "powerful" or "popular".
- Do not restate the name. "Kubernetes is a Kubernetes tool" says nothing.
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

	raw, err := d.GenerateJSON(ctx, draftSystem, b.String())
	if err != nil {
		return "", fmt.Errorf("drafting %s: %w", s.canonical, err)
	}

	var answer struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		return "", fmt.Errorf("drafting %s: decoding %q: %w", s.canonical, raw, err)
	}
	// The dictionary is one row per skill, so a wrapped answer would break the file it
	// is destined for. Collapsing beats rejecting: a model putting a sentence on two
	// lines is not a reason to lose the sentence.
	line := strings.Join(strings.Fields(answer.Description), " ")
	if line == "" {
		return "", fmt.Errorf("drafting %s: model returned no description in %q", s.canonical, raw)
	}
	return line, nil
}
