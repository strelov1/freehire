package main

import (
	"fmt"
	"sync"

	"github.com/strelov1/freehire/internal/platform/llmschema"
)

// answer is the shape asked of the model, and the only reason it is a named type: the
// schema is derived from it, so the contract and the decoder cannot drift apart.
type answerShape struct {
	Description string `json:"description"`
}

const schemaName = "skill_description"

var (
	schemaOnce sync.Once
	schema     llmschema.Schema
	schemaErr  error
)

// requestSchema derives the schema once and reuses it: it is a pure function of the
// type, so a second derivation could only produce the same document.
//
// It exists because the wave-1 run met a gateway that handed back the model's object
// wrapped in a string under its own key. A schema is the first line against that. It is
// NOT a proof — internal/platform/llm/AGENTS.md is explicit that a gateway which stops
// honouring one still answers 200 — which is why describedIn unwraps an envelope anyway.
func requestSchema() (llmschema.Schema, error) {
	schemaOnce.Do(func() {
		schema, schemaErr = llmschema.Of[answerShape]()
		if schemaErr != nil {
			schemaErr = fmt.Errorf("gen-skill-descriptions: build schema: %w", schemaErr)
		}
	})
	return schema, schemaErr
}
