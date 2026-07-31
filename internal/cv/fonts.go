package cv

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// fontFS holds the fonts bundled for templates that need a face the Typst binary does not
// embed. Typst ships only Libertinus Serif / New Computer Modern / DejaVu Sans Mono, so the
// sans templates rely on Liberation Sans (SIL OFL) staged here and exposed via --font-path.
//
//go:embed fonts/*.ttf
var fontFS embed.FS

// FontInfo is a registered typeface's display metadata, as the font picker sees it. Note
// names the familiar face it matches where one applies — the point of this list is not
// variety but letting a candidate match a firm's house style, so "Calibri metrics" is the
// useful thing to say about Carlito. CSS is the stack the live HTML preview renders with; it
// ships with the list so the web never grows a second copy of this registry.
type FontInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Note  string `json:"note,omitempty"`
	CSS   string `json:"css"`
}

// fontEntry is a registry row: the public metadata plus what only the server needs — the
// family name Typst asks for, and how that face reaches a compile run.
type fontEntry struct {
	FontInfo
	// Typst is the family name a template's #set text(font:) must use. It is never stored in
	// a Document: ResolveFontFamily maps an id to it at render time.
	Typst string
	// typstBuiltin marks a face the Typst binary already carries, so nothing is bundled for
	// it. Everything else names its .ttf files under fonts/, and TestEveryRegisteredFontIsResolvable
	// fails if one is missing — under --ignore-system-fonts a missing face is not an error,
	// it is a silently wrong CV.
	typstBuiltin bool
	files        []string
}

// fontRegistry is the set of typefaces a CV may choose. The seam to add one is: drop its
// .ttf files into fonts/ with their licence, then append an entry here.
//
// The list is deliberately short and conservative. Three of the four are metric-compatible
// with the faces résumé-scanning software and recruiters expect, which is what a CV font
// picker is actually for.
var fontRegistry = []fontEntry{
	{
		FontInfo:     FontInfo{ID: "libertinus-serif", Label: "Libertinus Serif", Note: "book serif", CSS: `"Libertinus Serif", "Linux Libertine", Georgia, serif`},
		Typst:        "Libertinus Serif",
		typstBuiltin: true,
	},
	{
		FontInfo: FontInfo{ID: "tinos", Label: "Tinos", Note: "Times New Roman metrics", CSS: `Tinos, "Times New Roman", Times, serif`},
		Typst:    "Tinos",
		files:    []string{"Tinos-Regular.ttf", "Tinos-Bold.ttf"},
	},
	{
		FontInfo: FontInfo{ID: "liberation-sans", Label: "Liberation Sans", Note: "Arial metrics", CSS: `"Liberation Sans", Arial, Helvetica, sans-serif`},
		Typst:    "Liberation Sans",
		files:    []string{"LiberationSans-Regular.ttf", "LiberationSans-Bold.ttf"},
	},
	{
		FontInfo: FontInfo{ID: "carlito", Label: "Carlito", Note: "Calibri metrics", CSS: `Carlito, Calibri, "Segoe UI", sans-serif`},
		Typst:    "Carlito",
		files:    []string{"Carlito-Regular.ttf", "Carlito-Bold.ttf"},
	},
}

// Fonts returns the registered typefaces' display metadata, for the picker and the API.
func Fonts() []FontInfo {
	out := make([]FontInfo, len(fontRegistry))
	for i, e := range fontRegistry {
		out[i] = e.FontInfo
	}
	return out
}

// FontIDs returns the registered font ids, for validation.
func FontIDs() []string {
	ids := make([]string, len(fontRegistry))
	for i, e := range fontRegistry {
		ids[i] = e.ID
	}
	return ids
}

// ResolveFontFamily maps a registry id to the family name Typst asks for. The empty id does
// NOT resolve: it means "whatever the template uses", and the renderer must leave the
// template's own font declaration alone rather than overriding it with a default.
func ResolveFontFamily(id string) (string, bool) {
	if id == "" {
		return "", false
	}
	i := slices.IndexFunc(fontRegistry, func(e fontEntry) bool { return e.ID == id })
	if i < 0 {
		return "", false
	}
	return fontRegistry[i].Typst, true
}

// knownFontID reports whether id is in the registry. Sanitize uses it to drop a family it
// could not render.
func knownFontID(id string) bool {
	return slices.ContainsFunc(fontRegistry, func(e fontEntry) bool { return e.ID == id })
}

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
