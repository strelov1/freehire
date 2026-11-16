package classify

import (
	"regexp"
)

// polishSeniorityTable lists the Polish grade aliases, stems on the folded title as in
// polishCategoryTable, in precedence order.
//
// The executive is the board's president and the general, executive and financial
// director, as in French and Spanish; a plain "dyrektor" and the "dyrektor
// zarządzający" of a bank's division state none, as "Director" and "Managing Director"
// state none in the main table. The trainee grades outrank the lead, as in German:
// "Stażysta w zespole" is an intern. The team's own lead is the "kierownik zespołu" and
// the "lider"; a "kierownik" alone is a manager, which is no grade. "Specjalista" is
// the ladder's middle rung by name only and states no grade either; "Regular" is the
// middle grade Polish IT postings write between Junior and Senior. A range ("Młodszy
// specjalista / Specjalista", "Specjalista/Starszy specjalista") keeps its upper bound:
// the senior grade is listed before the junior.
var polishSeniorityTable = []aliasEntry{
	{"prezes*", "c_level"},
	{"wiceprezes*", "c_level"},
	{"dyrektor* generaln*", "c_level"},
	{"dyrektork* generaln*", "c_level"},
	{"dyrektor* wykonawcz*", "c_level"},
	{"dyrektor* finansow*", "c_level"},
	{"stazyst*", "intern"},
	{"praktykant*", "intern"},
	{"staz", "intern"},
	{"stazu", "intern"},
	{"stazow*", "intern"},
	{"praktyki", "intern"},
	{"kierownik* zespol*", "lead"},
	{"kierowniczk* zespol*", "lead"},
	{"lider* zespol*", "lead"},
	{"liderk* zespol*", "lead"},
	{"lider* techniczn*", "lead"},
	{"lider* zmiany", "lead"},
	{"lider* programist*", "lead"},
	{"liderk* programist*", "lead"},
	{"lider* testow", "lead"},
	{"wiodac*", "lead"},
	{"starszy", "senior"},
	{"starsza", "senior"},
	{"starszego", "senior"},
	{"starszej", "senior"},
	{"regular", "middle"},
	{"mlodszy", "junior"},
	{"mlodsza", "junior"},
	{"mlodszego", "junior"},
	{"mlodszej", "junior"},
	{"absolwent*", "junior"},
}

// assistantOfPolishLead is the assistant or deputy OF a lead or an executive —
// "Asystent kierownika zespołu", "Zastępca Dyrektora Generalnego", "Asystentka Prezesa
// Zarządu" — which states no grade of its own, and a desk that RUNS the trainee
// programmes ("Koordynator ds. staży", "Specjalista ds. praktyk i staży"). Both are cut
// before the grade match.
var assistantOfPolishLead = regexp.MustCompile(`\b(?:asystent\p{L}*|zastepc\p{L}*|zastepczyni|sekretar\p{L}*)\s+(?:(?:i|lub)\s+\p{L}+\s+)?(?:prezes\p{L}*|wiceprezes\p{L}*|dyrektor\p{L}*|kierownik\p{L}*|kierowniczk\p{L}*|lider\p{L}*|koordynator\p{L}*)(?:\s+(?:zespol\p{L}*|generaln\p{L}*|finansow\p{L}*|wykonawcz\p{L}*|zarzadu))?|\bds\s+(?:\p{L}+\s+i\s+)?(?:staz\p{L}*|praktyk\p{L}*)`)

// polishElderly is the elderly person a carer looks after — "Opiekunka osoby starszej",
// "Opiekun osób starszych", "Opieka nad starszą panią" — whose "starszy" is an age, not
// the grade.
var polishElderly = regexp.MustCompile(`\b(?:osob\p{L}*|pan\p{L}*|ludzi|pacjent\p{L}*|senior\p{L}*)\s+starsz\p{L}*|\bstarsz\p{L}*\s+(?:osob\p{L}*|pan\p{L}*|ludzi|pacjent\p{L}*|wiek\p{L}*|dzieci|mieszkan\p{L}*)`)

// matchPolishSeniority resolves the grade of a lowercased title the other tables left
// unresolved.
func matchPolishSeniority(lower string) string {
	if !looksPolish(lower) {
		return ""
	}
	folded := assistantOfPolishLead.ReplaceAllString(foldPolish(lower), " ")
	folded = polishElderly.ReplaceAllString(folded, " ")
	if openingPolishLider.MatchString(folded) {
		return "lead"
	}
	return matchPolish(polishWords(folded), polishSeniorityTable)
}
