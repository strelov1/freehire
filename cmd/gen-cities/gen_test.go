package main

import (
	"reflect"
	"testing"
)

func TestNormalizeAlias(t *testing.T) {
	cases := map[string]string{
		"  Florianópolis ": "florianópolis",
		"São  Paulo":       "são paulo",
		"MOSCOW":           "moscow",
	}
	for in, want := range cases {
		if got := normalizeAlias(in); got != want {
			t.Errorf("normalizeAlias(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKeepAlias(t *testing.T) {
	keep := []string{"florianópolis", "são paulo", "nice", "ufa", "москва", "kraków"}
	drop := []string{
		"",          // empty
		"ob",        // too short (2 runes) — collides with codes
		"12345",     // digits only, no letter
		"tx 76135",  // contains digits
		"remote",    // work-mode marker
		"worldwide", // open-anywhere marker
		"europe",    // macro-region word
		"上海",        // CJK — outside Latin/Cyrillic, unused in IT location fields
		"شانغهاي",   // Arabic
		"σανγκάη",   // Greek
	}
	for _, a := range keep {
		if !keepAlias(a) {
			t.Errorf("keepAlias(%q) = false, want true", a)
		}
	}
	for _, a := range drop {
		if keepAlias(a) {
			t.Errorf("keepAlias(%q) = true, want false", a)
		}
	}
}

// buildAliases turns a GeoNames row's name/ascii/alternatenames into the deduped,
// filtered, lowercased alias set that will key the dictionary.
func TestBuildAliases(t *testing.T) {
	name := "Florianópolis"
	ascii := "Florianopolis"
	alt := "Florianopolis,Desterro,FLN,Флорианополис,"
	got := buildAliases(name, ascii, alt)
	want := []string{"desterro", "florianopolis", "florianópolis", "флорианополис"}
	// FLN is a 3-letter all-uppercase code -> dropped; empty trailing -> dropped;
	// duplicate florianopolis collapses.
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildAliases = %v, want %v", got, want)
	}
}
