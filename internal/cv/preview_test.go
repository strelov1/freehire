package cv

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
)

// TestGeneratePreviewsEmitsOneSVGPerTemplate proves the preview generator writes exactly one
// SVG per registered template (so the gallery never has a missing thumbnail) and nothing else.
func TestGeneratePreviewsEmitsOneSVGPerTemplate(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping preview generation test")
	}
	dir := t.TempDir()

	written, err := GeneratePreviews(context.Background(), NewTypstRenderer(bin), dir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	var wantIDs []string
	for _, ti := range Templates() {
		wantIDs = append(wantIDs, ti.ID)
	}
	sort.Strings(wantIDs)
	sort.Strings(written)
	if len(written) != len(wantIDs) {
		t.Fatalf("wrote %v, want one per template %v", written, wantIDs)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != len(wantIDs) {
		t.Fatalf("dir has %d files, want %d (no extras)", len(entries), len(wantIDs))
	}
	for _, ti := range Templates() {
		p := filepath.Join(dir, ti.ID+".svg")
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("missing preview for %q: %v", ti.ID, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("preview %q is empty", ti.ID)
		}
	}
}
