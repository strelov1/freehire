package telegram

import (
	"fmt"
	"sync"

	"github.com/strelov1/freehire/internal/llmschema"
)

// schemaName labels the shape in the provider's logs.
const schemaName = "telegram_extraction"

var (
	schemaOnce sync.Once
	schema     llmschema.Schema
	schemaErr  error
)

// requestSchema derives the extraction schema once from the Extraction contract.
func requestSchema() (llmschema.Schema, error) {
	schemaOnce.Do(func() {
		schema, schemaErr = llmschema.Of[Extraction]()
		if schemaErr != nil {
			schemaErr = fmt.Errorf("telegram: build schema: %w", schemaErr)
		}
	})

	return schema, schemaErr
}
