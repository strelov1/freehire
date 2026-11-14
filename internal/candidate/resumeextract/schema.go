package resumeextract

import (
	"fmt"
	"sync"

	"github.com/strelov1/freehire/internal/platform/llmschema"
)

// schemaName labels the shape in the provider's logs; it reads like the contract it
// came from rather than like this package.
const schemaName = "structured_cv"

var (
	schemaOnce sync.Once
	schema     llmschema.Schema
	schemaErr  error
)

// requestSchema derives the schema once and reuses it: it is a pure function of the
// Structured type, so a second derivation could only produce the same document.
func requestSchema() (llmschema.Schema, error) {
	schemaOnce.Do(func() {
		schema, schemaErr = llmschema.Of[Structured](
			// total_years is an int on the contract, but asking the model for an
			// integer hands it the rounding: given "5.9 years" it returns 6, while
			// truncInt returns 5. The difference is experience the candidate does
			// not have, so the field is asked for as text (see flexdecode.go).
			llmschema.AsText("total_years"),
		)
		if schemaErr != nil {
			schemaErr = fmt.Errorf("resumeextract: build schema: %w", schemaErr)
		}
	})

	return schema, schemaErr
}
