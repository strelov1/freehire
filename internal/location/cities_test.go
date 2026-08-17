package location

import "testing"

func TestLoadCityDict(t *testing.T) {
	tsv := "# header comment\n" +
		"# second comment\n" +
		"Moscow\tru\tmoscow|москва\t12000000\n" +
		"Moscow\tus\tmoscow|paradise valley\t5000000\n" + // lower-pop duplicate: first-wins keeps ru
		"Florianópolis\tbr\tflorianópolis|floripa\t500000\n" +
		// Below statingPopulation: contests "floripa" without ever claiming it, and
		// its own alias never enters the dictionary at all.
		"Floripa\tpt\tfloripa|tiny hamlet\t900\n"
	overrides := map[string]cityEntry{
		"zurich": {Name: "Zurich", Country: "ch"}, // override wins even though absent from the TSV
	}
	dict := loadCityDict(tsv, overrides)

	// "moscow" is claimed by ru and us: the name still resolves most-populous-first,
	// but the alias is contested so its country must never be emitted as geography.
	if got := dict["moscow"]; got.Name != "Moscow" || got.Country != "ru" || !got.Contested {
		t.Errorf(`dict["moscow"] = %+v, want {Moscow ru contested}`, got)
	}
	// An alias only the ru row carries is not contested by the us row above it.
	if got := dict["москва"]; got.Country != "ru" || got.Contested {
		t.Errorf(`dict["москва"] = %+v, want {ru, uncontested}`, got)
	}
	// Nor is an alias unique to the us row.
	if got := dict["paradise valley"]; got.Country != "us" || got.Contested {
		t.Errorf(`dict["paradise valley"] = %+v, want {us, uncontested}`, got)
	}
	// A hamlet in another country contests the alias even though it never registers.
	if got := dict["floripa"]; got.Name != "Florianópolis" || got.Country != "br" || !got.Contested {
		t.Errorf(`dict["floripa"] = %+v, want {Florianópolis br contested by the hamlet}`, got)
	}
	// ...and the hamlet's own alias is absent entirely: it is evidence, not a claim.
	if _, ok := dict["tiny hamlet"]; ok {
		t.Error(`dict["tiny hamlet"] exists; a sub-threshold place must never register an alias`)
	}
	// An override is hand-asserted, so it lands uncontested even for a shared name.
	if got := dict["zurich"]; got.Name != "Zurich" || got.Country != "ch" || got.Contested {
		t.Errorf(`dict["zurich"] = %+v, want override {Zurich ch uncontested}`, got)
	}
	if _, ok := dict["# header comment"]; ok {
		t.Error("comment line was parsed as an entry")
	}
}

// TestEmbeddedCityDict guards the real embedded dataset: the cities motivating this
// change must resolve to their canonical name and country.
func TestEmbeddedCityDict(t *testing.T) {
	cases := map[string]cityEntry{
		"florianópolis": {Name: "Florianópolis", Country: "br"},
		"florianopolis": {Name: "Florianópolis", Country: "br"},
		"são paulo":     {Name: "São Paulo", Country: "br"},
		"cologne":       {Name: "Cologne", Country: "de"}, // curated override spelling
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
