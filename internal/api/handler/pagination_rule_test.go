package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helperFile is where the shared pagination parse lives, and the one file allowed to read the
// raw query params.
const helperFile = "handler.go"

// TestOffsetIsParsedOnlyByTheSharedHelper pins a rule that a comment failed to hold.
//
// Every paginated endpoint must read its offset through pageParams/pageParamsBounded, which
// clamps it into int32 range. Fiber's QueryInt is a plain strconv.Atoi, so on a 64-bit build
// `?offset=3000000000` parses fine — and the int32 conversion every paginated column needs
// (they bind as Postgres int4) then wraps it NEGATIVE, which Postgres rejects. /jobs/:slug/copies
// hand-rolled the parse and answered 500 on a public, unauthenticated URL where every other list
// endpoint returns an empty page.
//
// The helper's own doc comment already named that overflow. Naming it did not stop a second call
// site from re-implementing the parse without the clamp, which is why the rule is a test now.
func TestOffsetIsParsedOnlyByTheSharedHelper(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	// The population is every file that paginates at all — one that reads the offset itself and
	// one that calls the helper both count. Counting only the violators would let the suite look
	// healthy the moment it was empty.
	var population int
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		body := string(src)

		readsRaw := strings.Contains(body, `QueryInt("offset"`) || strings.Contains(body, `Query("offset"`)
		usesHelper := strings.Contains(body, "pageParams(") ||
			strings.Contains(body, "pageParamsBounded(") ||
			strings.Contains(body, "pageParamsWindowed(")
		if !readsRaw && !usesHelper {
			continue
		}
		population++

		if readsRaw && name != helperFile {
			t.Errorf("%s reads the offset query param itself; use pageParams/pageParamsBounded, "+
				"which clamps into int32 range. Without the clamp, ?offset=3000000000 becomes a "+
				"negative int4 and the endpoint 500s instead of serving an empty page.", name)
		}
	}

	if population < 5 {
		t.Fatalf("only %d paginating files found — the scan is not seeing the package, so a "+
			"violation would pass unnoticed", population)
	}
}
