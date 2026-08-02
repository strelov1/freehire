package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoEditorIsConstructedWithoutAnEvidenceGate makes a fixture's configuration answerable to
// the same rule production obeys.
//
// newCVHandlers always receives bankGate{bank} (handler.go), so PATCH /me/cvs/:id edits as the
// AGENT for any API-key caller and an uncited claim is refused. Fixtures that built the editor
// with a nil gate were asserting a configuration that does not ship — one of them recorded the
// divergence in a comment as though it were a decision, and it survived the change that made the
// gate a constructor argument precisely because it bypassed the constructor.
//
// The rule is written against the nil rather than against the assembly because that is the shape
// the mistake takes: a struct literal with `editor:` set by hand. A fixture that genuinely has no
// bank should pass an empty one — the gate is then present and refuses, which is what a user with
// nothing banked actually experiences.
func TestNoEditorIsConstructedWithoutAnEvidenceGate(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	var scanned int
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		body, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(body)
		if !strings.Contains(text, "cvedit.NewEditor(") {
			continue
		}
		scanned++
		if strings.Contains(text, ", nil)") && strings.Contains(text, "cvedit.NewEditor(") {
			for _, line := range strings.Split(text, "\n") {
				if strings.Contains(line, "cvedit.NewEditor(") && strings.HasSuffix(strings.TrimSpace(line), ", nil)") {
					t.Errorf("%s constructs an editor with a nil evidence gate: %s\n"+
						"Production never does — pass bankGate{bank} (an empty bank is fine), or the "+
						"fixture asserts behaviour nobody can reach.", name, strings.TrimSpace(line))
				}
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no file constructs an editor — the scan is not seeing the package")
	}
}
