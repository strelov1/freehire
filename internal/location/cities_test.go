package location

import "testing"

func TestLoadCityDict(t *testing.T) {
	// The generator marks a contested alias with a trailing "*"; the loader only reads
	// that mark. Which aliases earn one is cmd/gen-cities' problem — see its own test.
	tsv := "# header comment\n" +
		"# second comment\n" +
		"Moscow\tru\tmoscow*|москва\n" +
		"Moscow\tus\tmoscow*|paradise valley\n" + // lower-pop duplicate: first-wins keeps ru
		"Florianópolis\tbr\tflorianópolis|floripa*\n"
	overrides := map[string]cityEntry{
		"zurich": {Name: "Zurich", Country: "ch"}, // override wins even though absent from the TSV
	}
	dict := loadCityDict(tsv, overrides)

	// A marked alias keeps its most-populous name and country — isRecognizedUSCACity
	// still wants them — but Contested is what stops the country being stated.
	if got := dict["moscow"]; got.Name != "Moscow" || got.Country != "ru" || !got.Contested {
		t.Errorf(`dict["moscow"] = %+v, want {Moscow ru contested}`, got)
	}
	// An unmarked alias on the same row is unaffected: the mark is per-alias.
	if got := dict["москва"]; got.Country != "ru" || got.Contested {
		t.Errorf(`dict["москва"] = %+v, want {ru, uncontested}`, got)
	}
	if got := dict["paradise valley"]; got.Country != "us" || got.Contested {
		t.Errorf(`dict["paradise valley"] = %+v, want {us, uncontested}`, got)
	}
	// The mark is stripped from the key, so lookups use the bare alias.
	if got := dict["floripa"]; got.Name != "Florianópolis" || got.Country != "br" || !got.Contested {
		t.Errorf(`dict["floripa"] = %+v, want {Florianópolis br contested}`, got)
	}
	if _, ok := dict["floripa*"]; ok {
		t.Error(`dict["floripa*"] exists; the contest mark must not survive into the key`)
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
