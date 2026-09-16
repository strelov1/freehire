package ojcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// The vendored schemas, named by the $id they publish themselves under. Tests refer to
// these constants rather than to file paths: a schema is identified by its $id in the
// standard, and the file it happens to sit in is an implementation detail of the vendoring.
const (
	schemaManifest                = "https://ojcp.dev/schemas/v0.1/manifest.json"
	schemaJobPosting              = "https://ojcp.dev/schemas/v0.1/job-posting.json"
	schemaSearchJobsResponse      = "https://ojcp.dev/schemas/v0.1/responses/search-jobs.json"
	schemaJobDetailResponse       = "https://ojcp.dev/schemas/v0.1/responses/job-detail.json"
	schemaEmployerContextResponse = "https://ojcp.dev/schemas/v0.1/responses/employer-context.json"
	schemaErrorResponse           = "https://ojcp.dev/schemas/v0.1/responses/error.json"
)

var vendoredSchemaDir = filepath.Join("testdata", "schemas")

var (
	compileOnce sync.Once
	compiled    map[string]*jsonschema.Schema
	compileErr  error
)

// validateAgainstSchema reports whether value conforms to the vendored OJCP schema
// published under schemaID. Every vendored schema is registered before anything is
// compiled, so the cross-file $refs resolve from disk and a test run never reaches the
// network — ojcp.dev being down must not turn into a red build.
func validateAgainstSchema(t *testing.T, schemaID string, value any) error {
	t.Helper()

	compileOnce.Do(compileVendoredSchemas)
	if compileErr != nil {
		t.Fatalf("compiling vendored OJCP schemas: %v", compileErr)
	}

	schema, ok := compiled[schemaID]
	if !ok {
		t.Fatalf("no vendored schema published under %q; see %s/README.md", schemaID, vendoredSchemaDir)
	}

	// The validator works over the generic JSON tree, not over Go values: a projection
	// must be judged as the bytes an agent receives, which is what a round-trip produces.
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshalling value for validation: %v", err)
	}
	decoded, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("re-reading marshalled value: %v", err)
	}

	return schema.Validate(decoded)
}

func compileVendoredSchemas() {
	files, err := vendoredSchemaPaths()
	if err != nil {
		compileErr = err
		return
	}

	// Every resource is registered before any is compiled. Compiling as we go would fail
	// on the first schema whose $ref names a file later in the walk.
	c := jsonschema.NewCompiler()

	// Draft 2020-12 treats `format` as an annotation unless the validator opts in, so
	// without this a `datePosted` of "16/09/2026" — or a `url` that is not one — validates
	// clean, and the oracle is blind to exactly the fields the posting projection writes.
	c.AssertFormat()

	// The offline guarantee is asserted here, not inherited from whatever loader the
	// library happens to default to. A $ref the vendored set does not satisfy must fail the
	// compile; silently fetching it would make CI depend on ojcp.dev being reachable, and
	// the day it is not, the red build lands in an unrelated PR with no hint where it
	// came from.
	c.UseLoader(refusingLoader{})
	ids := make([]string, 0, len(files))
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			compileErr = err
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			compileErr = err
			return
		}
		id, err := readSchemaID(doc, path)
		if err != nil {
			compileErr = err
			return
		}
		if err := c.AddResource(id, doc); err != nil {
			compileErr = err
			return
		}
		ids = append(ids, id)
	}

	compiled = make(map[string]*jsonschema.Schema, len(ids))
	for _, id := range ids {
		schema, err := c.Compile(id)
		if err != nil {
			compileErr = err
			return
		}
		compiled[id] = schema
	}
}

// vendoredSchemaPaths walks the vendored tree rather than listing the files, so a schema
// added to it joins the oracle by existing. A hand-kept list here would be one more place
// for a re-vendoring to be half-done.
func vendoredSchemaPaths() ([]string, error) {
	var paths []string
	err := filepath.WalkDir(vendoredSchemaDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && filepath.Ext(path) == ".json" {
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}

// refusingLoader is the compiler's whole view of the outside world: it answers every URL
// with an error. Anything the vendored tree does not itself register is therefore a compile
// failure naming the missing $id, which is the diagnosis a re-vendoring that forgot a file
// actually needs.
type refusingLoader struct{}

func (refusingLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("refusing to fetch %s: OJCP schemas resolve only from %s", url, vendoredSchemaDir)
}

// readSchemaID reads the $id a vendored schema publishes itself under. A file without one
// is an error rather than a skip: it would compile fine and then be unreachable by every
// test, which looks exactly like a schema nobody thought to assert against.
func readSchemaID(doc any, path string) (string, error) {
	obj, ok := doc.(map[string]any)
	if !ok {
		return "", fmt.Errorf("%s: not a JSON object", path)
	}
	id, ok := obj["$id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("%s: no $id", path)
	}
	return id, nil
}
