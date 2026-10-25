package jobderive

import (
	"testing"

	"github.com/strelov1/freehire/internal/dict/normalize"
)

// TestDerive_FormatCharactersInTitle pins where the title's invisible characters are
// cut. A German employer hyphenates its title for line breaking with U+00AD
// ("Rei-ni-gungs-kraft"), which the dictionaries read as a word boundary and so
// match nothing. The dictionaries read the cleaned title; the public slug is still
// minted from the title as stored, because it is a published URL and a stored row
// re-derived by backfill-derive must keep the slug it was given.
func TestDerive_FormatCharactersInTitle(t *testing.T) {
	raw := "Rei\u00adni\u00adgungs\u00adkraft (m/w/d)"
	got := Derive(Input{Title: raw, Company: "Acme", Source: "coveto", ExternalID: "1"})
	if got.Category != "personal_services" {
		t.Errorf("Category = %q, want personal_services", got.Category)
	}
	if want := normalize.JobSlug(raw, "Acme", "coveto", "1"); got.PublicSlug != want {
		t.Errorf("PublicSlug = %q, want %q (minted from the stored title)", got.PublicSlug, want)
	}
}
