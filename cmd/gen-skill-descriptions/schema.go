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

// requestSchema derives the schema once and reuses it. A schema is the first line against a
// gateway wrapping the model's object in a string, but not a proof
// (internal/platform/llm/AGENTS.md), which is why describedIn unwraps an envelope anyway.
func requestSchema() (llmschema.Schema, error) {
	schemaOnce.Do(func() {
		schema, schemaErr = llmschema.Of[answerShape]()
		if schemaErr != nil {
			schemaErr = fmt.Errorf("gen-skill-descriptions: build schema: %w", schemaErr)
		}
	})
	return schema, schemaErr
}
