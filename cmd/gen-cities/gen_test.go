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

func TestContestedAliases(t *testing.T) {
	places := []place{
		{name: "Moscow", country: "ru", pop: 12_000_000, aliases: []string{"moscow", "москва"}},
		{name: "Moscow", country: "us", pop: 25_000, aliases: []string{"moscow", "paradise valley"}},
		// A hamlet far below statingPopulation. It is never written out, but it is
		// exactly what makes "taft" contested — the whole reason the generator reads
		// the pop>=1,000 dump rather than the pop>=15,000 one.
		{name: "Taft", country: "us", pop: 9_000, aliases: []string{"taft"}},
		{name: "Taft", country: "ir", pop: 20_000, aliases: []string{"taft"}},
		// Two places in the SAME country agreeing on a name is not a contest.
		{name: "Springfield", country: "us", pop: 160_000, aliases: []string{"springfield"}},
		{name: "Springfield", country: "us", pop: 120_000, aliases: []string{"springfield"}},
	}
	got := contestedAliases(places)
	for _, alias := range []string{"moscow", "taft"} {
		if !got[alias] {
			t.Errorf("alias %q should be contested", alias)
		}
	}
	for _, alias := range []string{"москва", "paradise valley", "springfield"} {
		if got[alias] {
			t.Errorf("alias %q should not be contested", alias)
		}
	}
}

func TestAliasesWithContestMarks(t *testing.T) {
	got := aliasesWithContestMarks([]string{"moscow", "москва"}, map[string]bool{"moscow": true})
	if want := "moscow*|москва"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
