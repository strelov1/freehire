package resumeextract

import (
	"fmt"
	"sync"

	"github.com/strelov1/freehire/internal/llmschema"
)

// schemaName labels the shape in the provider's logs; it reads like the contract it
// came from rather than like this package.
const schemaName = "structured_cv"

// contactFields are supplied by deterministic PII detection over text the model never
// sees, so they are cut from what the model is asked for. Under strict mode every
// property is required — left in, they would be an instruction to invent a name,
// email, phone, residence or links for a CV that reaches the model with those redacted.
var contactFields = []string{"full_name", "email", "phone", "location", "links"}

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
			llmschema.Omit(contactFields...),
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
