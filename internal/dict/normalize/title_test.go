package normalize

import "testing"

func TestCleanTitleStripsFormatCharacters(t *testing.T) {
	for in, want := range map[string]string{
		"Rei\u00adni\u00adgungs\u00adkraft (m/w/d)": "Reinigungskraft (m/w/d)", // soft hyphen
		"Sach\u00adbe\u00adar\u00adbei\u00adter/in": "Sachbearbeiter/in",
		"Pflege\u200bfachkraft":                     "Pflegefachkraft", // zero-width space
		"\ufeffKoch (m/w/d)":                        "Koch (m/w/d)",    // byte-order mark
		"Erzieher\u2060in":                          "Erzieherin",      // word joiner
		"Software Engineer":                         "Software Engineer",
		"Gesundheits- und Krankenpfleger":           "Gesundheits- und Krankenpfleger",
	} {
		if got := CleanTitle(in); got != want {
			t.Errorf("CleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
