package mailpreview_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/mailpreview"
)

// repoRoot walks up from the package directory to the module root, so the test can
// read the committed previews regardless of where `go test` was invoked.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find the module root")
		}
		dir = parent
	}
}

// TestPreviewsAreCurrent is the reason the rendered files are committed rather than
// generated on the fly: Storybook shows whatever is on disk, so a template change
// nobody re-previewed would leave the design review looking at the previous design
// while every test passed. This turns that into a failure with the fix in the message.
func TestPreviewsAreCurrent(t *testing.T) {
	samples, err := mailpreview.Samples(mailpreview.DefaultBaseURL)
	if err != nil {
		t.Fatalf("rendering samples: %v", err)
	}
	dir := filepath.Join(repoRoot(t), mailpreview.DefaultDir)

	for _, s := range samples {
		for suffix, want := range map[string]string{
			".html":       s.HTML,
			".light.html": s.LightHTML,
			".dark.html":  s.DarkHTML,
		} {
			path := filepath.Join(dir, s.Name+suffix)
			onDisk, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("%s%s: %v — run `make mail-preview`", s.Name, suffix, err)
				continue
			}
			if string(onDisk) != want {
				t.Errorf("%s%s is stale — run `make mail-preview`", s.Name, suffix)
			}
		}
	}
}

// TestPinnedSchemesDiffer guards the pinning itself. PinScheme rewrites exact
// literals, so a change to how the shell emits its stylesheet could leave it
// matching nothing — and a preview that silently ignored the toggle would look
// like the dark design was simply never applied.
func TestPinnedSchemesDiffer(t *testing.T) {
	samples, err := mailpreview.Samples(mailpreview.DefaultBaseURL)
	if err != nil {
		t.Fatalf("rendering samples: %v", err)
	}
	for _, s := range samples {
		if s.LightHTML == s.DarkHTML {
			t.Errorf("%s: the pinned light and dark documents are identical", s.Name)
		}
		if strings.Contains(s.LightHTML, "prefers-color-scheme") {
			t.Errorf("%s: the light copy still defers to the reader's preference", s.Name)
		}
		if !strings.Contains(s.HTML, "prefers-color-scheme") {
			t.Errorf("%s: the sent copy lost its dark-mode rules", s.Name)
		}
	}
}

// TestEveryMailCarriesTheBranding checks the one thing that is easy to lose by
// writing a new mail the old way: bypassing the shared shell. A body assembled with
// fmt.Sprintf still sends, still reads fine, and silently arrives unbranded.
func TestEveryMailCarriesTheBranding(t *testing.T) {
	samples, err := mailpreview.Samples(mailpreview.DefaultBaseURL)
	if err != nil {
		t.Fatalf("rendering samples: %v", err)
	}
	if len(samples) == 0 {
		t.Fatal("no samples registered")
	}

	for _, s := range samples {
		if !strings.Contains(s.HTML, "/email-logo.png") {
			t.Errorf("%s: no logo — is it rendered through mailtpl.Layout?", s.Name)
		}
		if !strings.Contains(s.HTML, "<!DOCTYPE html>") {
			t.Errorf("%s: body is a fragment, not a document", s.Name)
		}
		if strings.TrimSpace(s.Subject) == "" {
			t.Errorf("%s: empty subject", s.Name)
		}
		// The plain-text alternative is what non-HTML clients and spam scorers read;
		// an empty one costs deliverability and is invisible in a visual review.
		if strings.TrimSpace(s.Text) == "" {
			t.Errorf("%s: empty plain-text body", s.Name)
		}
	}
}
