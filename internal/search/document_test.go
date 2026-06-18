package search

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/db"
)

func TestFromJob_DocumentFlattensIDAndViewToTopLevelJSON(t *testing.T) {
	// Meilisearch reads the primary key "id" from the top level of the document,
	// and the embedded jobview.Job must flatten (no nesting) so its fields are
	// the searchable attributes. A json tag on the embedded field would break
	// this. Enrichment itself stays a nested object (filtered via dot paths).
	doc, err := FromJob(db.Job{ID: 42, Title: "Go Dev", PublicSlug: "go-dev-acme-x"})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := string(m["id"]); got != "42" {
		t.Errorf("top-level id = %s, want 42 in %s", got, raw)
	}
	if got := string(m["public_slug"]); got != `"go-dev-acme-x"` {
		t.Errorf("public_slug not flattened to top level: %s", raw)
	}
	if _, ok := m["enrichment"]; !ok {
		t.Errorf("enrichment should be a nested object in %s", raw)
	}
}

func TestFromJob_CapsIndexedDescription(t *testing.T) {
	// The search document caps the description so the inverted index (and a full
	// rebuild's transient disk footprint) stays small; the detail endpoint still
	// serves the full text from Postgres. A long description is trimmed to at most
	// maxIndexedDescriptionRunes runes; a short one is left verbatim.
	long := strings.Repeat("alpha beta ", 600) // ~6000 runes
	doc, err := FromJob(db.Job{ID: 1, Title: "Go Dev", Description: long})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if n := utf8.RuneCountInString(doc.Description); n > maxIndexedDescriptionRunes {
		t.Errorf("indexed description = %d runes, want <= %d", n, maxIndexedDescriptionRunes)
	}
	if !strings.HasPrefix(long, doc.Description) {
		t.Errorf("truncated description is not a prefix of the original")
	}

	short := "Build backend services in Go."
	docShort, err := FromJob(db.Job{ID: 2, Title: "Go Dev", Description: short})
	if err != nil {
		t.Fatalf("FromJob short: %v", err)
	}
	if docShort.Description != short {
		t.Errorf("short description altered: got %q, want %q", docShort.Description, short)
	}
}
