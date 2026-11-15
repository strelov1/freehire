package resumeextract

import (
	"fmt"
	"sync"

	"github.com/strelov1/freehire/internal/platform/llmschema"
)

// schemaName labels the shape in the provider's logs.
const schemaName = "structured_cv"

var (
	schemaOnce sync.Once
	schema     llmschema.Schema
	schemaErr  error
)

// requestSchema derives the schema once; it is a pure function of Structured.
func requestSchema() (llmschema.Schema, error) {
	schemaOnce.Do(func() {
		schema, schemaErr = llmschema.Of[Structured](
			// Asked as text, not an integer: the model rounds "5.9 years" to 6, while
			// truncInt keeps 5 (see flexdecode.go).
			llmschema.AsText("total_years"),
		)
		if schemaErr != nil {
			schemaErr = fmt.Errorf("resumeextract: build schema: %w", schemaErr)
		}
	})

	return schema, schemaErr
}
