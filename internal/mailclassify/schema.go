package mailclassify

import (
	"fmt"
	"sync"

	"github.com/strelov1/freehire/internal/llmschema"
)

// schemaName labels the shape in the provider's logs.
const schemaName = "mail_classification"

var (
	schemaOnce sync.Once
	schema     llmschema.Schema
	schemaErr  error
)

// requestSchema derives the classification schema once: it is a pure function of the
// Classification type and the signal vocabulary, so re-deriving could only repeat it.
func requestSchema() (llmschema.Schema, error) {
	schemaOnce.Do(func() {
		schema, schemaErr = llmschema.Of[Classification](
			llmschema.Enum("signal", SignalValues),
		)
		if schemaErr != nil {
			schemaErr = fmt.Errorf("mailclassify: build schema: %w", schemaErr)
		}
	})

	return schema, schemaErr
}
