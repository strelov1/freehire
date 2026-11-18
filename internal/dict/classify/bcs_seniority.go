package classify

import (
	"regexp"
)

// matchBCSSeniority resolves the grade of a lowercased title the other tables left
// unresolved: the main table once more on the transliterated, gender-cut title, then
// the BCS table with the assistants of a lead cut (bcsAssistantOfLead), then the
// Slovenian table with its own (slovenianAssistantOfLead).
func matchBCSSeniority(lower string) string {
	g := foldBCSGender(lower)
	if g != lower {
		if s := matchOrdered(g, seniorityTable); s != "" {
			return s
		}
	}
	if s := matchBCSOrdered(bcsWords(bcsAssistantOfLead.ReplaceAllString(g, " ")), bcsSeniority); s != "" {
		return s
	}
	return matchBCSOrdered(bcsWords(slovenianAssistantOfLead.ReplaceAllString(g, " ")), slovenianSeniority)
}

// bcsSeniorityTable lists the BCS grade aliases on the folded title, in precedence
// order. An alias opening with "=" is matched on the title with its diacritics kept:
// "viša" is the senior grade, ASCII "visa" an English word, and "vođa" is the BCS
// leader where the Slovenian "vodja" is any manager.
//
// Only the general, executive and financial directors and the management board are the
// executive, as in the other tables; a "Direktor prodaje" states no grade, as "Sales
// Director" states none. The trainee grades outrank the lead. "Voditelj" alone is the
// manager and states none; the team's and the shift's own lead does.
var bcsSeniorityTable = []aliasEntry{
	{"generaln* direktor*", "c_level"},
	{"izvrsn* direktor*", "c_level"},
	{"financijsk* direktor*", "c_level"},
	{"finansijsk* direktor*", "c_level"},
	{"predsjednik* uprav*", "c_level"},
	{"predsednik* uprav*", "c_level"},
	{"clan* uprav*", "c_level"},
	{"pripravnik*", "intern"},
	{"pripravnic*", "intern"},
	{"praks*", "intern"},
	{"praktikant*", "intern"},
	{"stazist*", "intern"},
	{"=vođ*", "lead"},
	{"vodja tim*", "lead"},
	{"vodja smjen*", "lead"},
	{"vodja smen*", "lead"},
	{"voditelj* tim*", "lead"},
	{"voditelj* smjen*", "lead"},
	{"voditelj* smen*", "lead"},
	{"tim lider*", "lead"},
	{"timlider*", "lead"},
	{"=šef*", "lead"},
	{"=vodeći", "lead"},
	{"=vodeća", "lead"},
	{"vodeci", "lead"},
	{"=viši", "senior"},
	{"=viša", "senior"},
	{"=višeg", "senior"},
	{"visi", "senior"},
	{"stariji", "senior"},
	{"starija", "senior"},
	{"mladj*", "junior"},
	{"=niži", "junior"},
	{"=niža", "junior"},
	{"nizi", "junior"},
	{"pocetnik*", "junior"},
	{"pocetnic*", "junior"},
}

// bcsAssistantOfLead is the assistant or deputy OF a lead or an executive — "Pomoćnik
// izvršnog direktora", "Zamjenik voditelja smjene", "Asistent šefa kuhinje" — which
// states no grade of its own. It is cut, on the folded title, before the grade match.
var bcsAssistantOfLead = regexp.MustCompile(`\b(?:pomo[cć]ni(?:k|ca)|asistent(?:ica)?|zamjeni(?:k|ca)|zameni(?:k|ca)|sekretar(?:ica)?|tajni(?:k|ca))\s+(?:(?:glavnog|generalnog|izvr[sš]nog|financijskog|finansijskog)\s+)?(?:direktor|voditelj|vo(?:dj|đ)|[sš]ef|predsjednik|predsednik)\p{L}*`)
