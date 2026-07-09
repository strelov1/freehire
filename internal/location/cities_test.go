package location

import "testing"

func TestLoadCityDict(t *testing.T) {
	tsv := "# header comment\n" +
		"# second comment\n" +
		"Moscow\tru\tmoscow|москва\n" +
		"Moscow\tus\tmoscow|paradise valley\n" + // lower-pop duplicate: first-wins keeps ru
		"Florianópolis\tbr\tflorianópolis|floripa\n"
	overrides := map[string]cityEntry{
		"zurich": {"Zurich", "ch"}, // override wins even though absent from the TSV
	}
	dict := loadCityDict(tsv, overrides)

	if got := dict["moscow"]; got.Name != "Moscow" || got.Country != "ru" {
		t.Errorf(`dict["moscow"] = %+v, want {Moscow ru} (most-populous first-wins)`, got)
	}
	if got := dict["москва"]; got.Country != "ru" {
		t.Errorf(`dict["москва"] country = %q, want ru`, got.Country)
	}
	if got := dict["floripa"]; got.Name != "Florianópolis" || got.Country != "br" {
		t.Errorf(`dict["floripa"] = %+v, want {Florianópolis br}`, got)
	}
	if got := dict["zurich"]; got.Name != "Zurich" || got.Country != "ch" {
		t.Errorf(`dict["zurich"] = %+v, want override {Zurich ch}`, got)
	}
	if _, ok := dict["# header comment"]; ok {
		t.Error("comment line was parsed as an entry")
	}
}

// TestEmbeddedCityDict guards the real embedded dataset: the cities motivating this
// change must resolve to their canonical name and country.
func TestEmbeddedCityDict(t *testing.T) {
	cases := map[string]cityEntry{
		"florianópolis": {"Florianópolis", "br"},
		"florianopolis": {"Florianópolis", "br"},
		"são paulo":     {"São Paulo", "br"},
		"cologne":       {"Cologne", "de"}, // curated override spelling
	}
	for alias, want := range cases {
		got, ok := cityDict[alias]
		if !ok {
			t.Errorf("cityDict[%q] missing", alias)
			continue
		}
		if got.Name != want.Name || got.Country != want.Country {
			t.Errorf("cityDict[%q] = %+v, want %+v", alias, got, want)
		}
	}
}
