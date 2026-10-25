package location

import "testing"

// TestDutchWorkMode pins the Dutch arrangement prose. The sentences are cut from live
// Dutch postings (eures, recruitee, whatjobs-nl), measured 2026-10-25.
func TestDutchWorkMode(t *testing.T) {
	cases := []struct {
		name, desc, want string
	}{
		// Home working offered beside an office is hybrid, as a benefit or a model.
		{"model", "<li>Hybride werken: gemiddeld 60% vanuit huis en 40% op kantoor</li>", "hybrid"},
		{"days", "zeg jij ja tegen tot 4 dagen thuiswerken en 26 vakantiedagen", "hybrid"},
		{"one day", "4 dagen per week op kantoor in Tilburg. 1 dag per week thuiswerken.", "hybrid"},
		{"possibility", "<ul><li>Flexibele werktijden + mogelijkheid tot thuiswerken</li></ul>", "hybrid"},
		{"allowance", "Je ontvangt een thuiswerkvergoeding en een reiskostenvergoeding.", "hybrid"},
		{"partly", "Je werkt op onze locatie in Weert en hebt de mogelijkheid tot deels thuiswerken.", "hybrid"},
		{"from home", "de mogelijkheid om 2 dagen in de week vanuit huis te werken", "hybrid"},
		{"place and time", "Wij bieden plaats- en tijdonafhankelijk werken.", "hybrid"},
		// Remote needs a totality, and is read before the hybrid phrase it contains.
		{"fully remote", "Deze functie is volledig remote, je werkt waar je wilt.", "remote"},
		{"100% home", "Je kunt 100% thuiswerken met een thuiswerkvergoeding.", "remote"},
		{"fully from home", "Je werkt volledig vanuit huis en bezoekt soms een klant.", "remote"},
		// An explicit refusal is onsite; a negated or withdrawn offer is not an offer.
		{"refused", "Let op: thuiswerken is niet mogelijk in deze functie.", "onsite"},
		{"no home working", "Dit is een functie op locatie, geen thuiswerk.", "onsite"},
		{"withdrawn", "Hybride werken is in deze rol helaas niet mogelijk.", ""},
		// IT and support senses of the trigger words state no arrangement.
		{"hybrid cloud", "Ervaring met hybride cloud en hybride infrastructuren.", ""},
		{"remote support", "Je lost storingen op afstand op en biedt remote support.", ""},
		{"feel at home", "Bij ons voel je je volledig thuis.", ""},
		{"abroad perk", "Je mag tot vier weken per jaar vanuit het buitenland werken.", ""},
		{"remote perk", "Twee weken per jaar remote werken vanuit het buitenland.", ""},
		{"remote abroad refused", "Woonachtig in Nederland (volledig remote werken vanuit het buitenland of relocatie is niet mogelijk)", ""},
		{"remote incident", "Je lost IT-incidenten op die niet remote verholpen kunnen worden.", ""},
		{"not negotiable", "Thuiswerken is niet mogelijk; hybride werken is niet bespreekbaar.", "onsite"},
		{"field role", "Het gaat om een hybride functie en je bent daarom mobiel.", ""},
	}
	for _, c := range cases {
		if got := WorkModeFromDescription(c.desc); got != c.want {
			t.Errorf("%s: WorkModeFromDescription(%q) = %q, want %q", c.name, c.desc, got, c.want)
		}
	}
}
