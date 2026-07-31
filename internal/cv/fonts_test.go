package cv

import (
	"io/fs"
	"testing"
)

// The renderer runs Typst with --ignore-system-fonts, so a face is available only if the
// Typst binary embeds it or its TTF is bundled here. A registry entry naming neither would
// silently fall back to some other face at render time — no error, just the wrong CV. This
// test is the gate that stops such an entry being added.
func TestEveryRegisteredFontIsResolvable(t *testing.T) {
	bundled, err := fs.ReadDir(fontFS, "fonts")
	if err != nil {
		t.Fatalf("read bundled fonts: %v", err)
	}
	present := map[string]bool{}
	for _, e := range bundled {
		present[e.Name()] = true
	}

	for _, e := range fontRegistry {
		if e.ID == "" {
			t.Errorf("registry entry %q has no id", e.Label)
		}
		if e.Typst == "" {
			t.Errorf("font %q names no Typst family", e.ID)
		}
		if e.CSS == "" {
			t.Errorf("font %q has no CSS stack for the HTML preview", e.ID)
		}
		if e.typstBuiltin {
			if len(e.files) != 0 {
				t.Errorf("font %q is marked built into Typst but also bundles %v", e.ID, e.files)
			}
			continue
		}
		if len(e.files) == 0 {
			t.Errorf("font %q is not a Typst built-in and bundles no files", e.ID)
		}
		for _, f := range e.files {
			if !present[f] {
				t.Errorf("font %q names %s, which is not in internal/cv/fonts/", e.ID, f)
			}
		}
	}
}

func TestFontsAreDiscoverable(t *testing.T) {
	fonts := Fonts()
	if len(fonts) == 0 {
		t.Fatal("Fonts() returned nothing")
	}
	seen := map[string]bool{}
	for _, f := range fonts {
		if seen[f.ID] {
			t.Errorf("duplicate font id %q", f.ID)
		}
		seen[f.ID] = true
	}
	// The registry is metadata for a picker; it must not leak the renderer's own face name,
	// which is what the document would then be tempted to store.
	if len(FontIDs()) != len(fonts) {
		t.Errorf("FontIDs() has %d entries, Fonts() has %d", len(FontIDs()), len(fonts))
	}
}

func TestResolveFontFamily(t *testing.T) {
	id := fontRegistry[0].ID
	got, ok := ResolveFontFamily(id)
	if !ok {
		t.Fatalf("registered id %q did not resolve", id)
	}
	if got != fontRegistry[0].Typst {
		t.Errorf("resolved %q to %q, want %q", id, got, fontRegistry[0].Typst)
	}

	if _, ok := ResolveFontFamily("no-such-font"); ok {
		t.Error("an unregistered id resolved")
	}
	// An unset family is not an error: it means "whatever the template uses", and the
	// renderer must leave the template's own #set text(font:) alone.
	if _, ok := ResolveFontFamily(""); ok {
		t.Error("the empty id resolved to a face; it must mean 'inherit from the template'")
	}
}

func TestSanitizeDropsAnUnregisteredFontFamily(t *testing.T) {
	doc := Document{Style: Style{FontFamily: "comic-sans-ms"}}
	doc.Sanitize()
	if doc.Style.FontFamily != "" {
		t.Errorf("unregistered family: got %q, want unset", doc.Style.FontFamily)
	}

	keep := Document{Style: Style{FontFamily: fontRegistry[0].ID}}
	keep.Sanitize()
	if keep.Style.FontFamily != fontRegistry[0].ID {
		t.Errorf("registered family was dropped: got %q", keep.Style.FontFamily)
	}
}
