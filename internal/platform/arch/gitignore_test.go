package arch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/modroot"
)

// TestEveryCmdBinaryIsGitignored keeps .gitignore in step with cmd/.
//
// release.sh builds every cmd/ target into the checkout ROOT, so each one lands there
// as an untracked file. A target missing from .gitignore is one `git add -A` away from
// being committed as a ~26 MB blob — and once committed, it blocks `git pull` on every
// deploy host that already has its own build sitting there, which fails the release
// outright.
//
// That has now happened twice: `prune` in #1212, and `search-drain` in #2220, which
// found 30 further targets the hand-maintained list had never caught up with. The list
// is deliberately explicit rather than a glob (see .gitignore's own comment: a bare
// name matches at any depth and would shadow the package directory of the same name),
// so it needs something to keep it honest. This is that something.
func TestEveryCmdBinaryIsGitignored(t *testing.T) {
	root, err := modroot.Find()
	if err != nil {
		t.Fatalf("locate module root: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	ignored := make(map[string]bool)
	for _, line := range strings.Split(string(raw), "\n") {
		ignored[strings.TrimSpace(line)] = true
	}

	entries, err := os.ReadDir(filepath.Join(root, "cmd"))
	if err != nil {
		t.Fatalf("read cmd/: %v", err)
	}
	var missing []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Anchored: an unanchored name would also ignore internal/<name>/ and silently
		// swallow new source files there.
		if !ignored["/"+e.Name()] {
			missing = append(missing, "/"+e.Name())
		}
	}
	if len(missing) > 0 {
		t.Errorf("cmd/ targets with no anchored .gitignore entry — a build in the checkout root can be committed as a multi-MB blob and break every deploy:\n\t%s",
			strings.Join(missing, "\n\t"))
	}
}
