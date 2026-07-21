package cv

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

// fontFS holds the fonts bundled for templates that need a face the Typst binary does not
// embed. Typst ships only Libertinus Serif / New Computer Modern / DejaVu Sans Mono, so the
// sans templates rely on Liberation Sans (SIL OFL) staged here and exposed via --font-path.
//
//go:embed fonts/*.ttf
var fontFS embed.FS

// writeFonts materializes the bundled .ttf files into dir so a Typst compile run under
// --ignore-system-fonts can pick them up via --font-path dir. It returns the number written.
func writeFonts(dir string) error {
	entries, err := fs.ReadDir(fontFS, "fonts")
	if err != nil {
		return err
	}
	for _, e := range entries {
		b, err := fontFS.ReadFile("fonts/" + e.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o600); err != nil {
			return err
		}
	}
	return nil
}
