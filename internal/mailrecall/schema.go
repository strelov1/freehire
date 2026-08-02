package mailrecall

import (
	"fmt"
	"sync"

	"github.com/strelov1/freehire/internal/llmschema"
)

// schemaName labels the shape in the provider's logs.
const schemaName = "mail_recall"

var (
	schemaOnce sync.Once
	schema     llmschema.Schema
	schemaErr  error
)

// requestSchema derives the batched answer's schema once. It is a pure function of the
// answer type, so re-deriving could only repeat it.
func requestSchema() (llmschema.Schema, error) {
	schemaOnce.Do(func() {
		schema, schemaErr = llmschema.Of[answer]()
		if schemaErr != nil {
			schemaErr = fmt.Errorf("mailrecall: build schema: %w", schemaErr)
		}
	})
	return schema, schemaErr
}
