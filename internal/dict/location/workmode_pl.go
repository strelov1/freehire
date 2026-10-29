package location

import (
	"regexp"
	"strings"
)

// polishWorkModePhrases is the Polish counterpart of frenchWorkModePhrases, built from the
// Polish-language crawl (erecruiter, traffit, nofluffjobs, solidjobs, smartrecruiters,
// teamtailor, recruitee, eures, whatjobs-pl: 9,835 open postings on 2026-10-29, of which
// three quarters carried no structured work mode and 0.5% stated one any reader
// understood).
//
// Polish states the model in a labelled line more often than not ("Tryb pracy: zdalny",
// "Model pracy: hybrydowy", "Lokalizacja: 100% zdalnie"), and one applicant-tracking
// template writes it as "Praca zdalna: w całości / hybrydowo / możliwa częściowo / brak".
// Home working is still listed as a BENEFIT too — "możliwość pracy zdalnej", "2 dni pracy
// zdalnej" — which offers it beside an office, so it is hybrid; remote needs an explicit
// totality ("100% zdalnie", "w pełni zdalna", "praca zdalna: w całości"). The order is
// remote, hybrid, onsite, as in German.
//
// Every phrase is matched against the folded text with its diacritics dropped and "ł"
// written "l" ("w pełni" → "w pelni", "całości" → "calosci"), so each is spelled that way.
// Polish inflects, so each phrase lists the case forms the crawl carries.
//
// Absent on purpose:
//   - bare "praca zdalna" and "zdalnie": "praca zdalna: brak" is the template's refusal,
//     "rekrutacja zdalna" and "onboarding prowadzony jest zdalnie" are the hiring process,
//     "zdalne wsparcie techniczne" is the product. Only the labelled and totality forms count.
//   - bare "stacjonarnie"/"stacjonarny": a tutoring platform's "lekcje online lub
//     stacjonarnie" (~300 postings), "szkolenia stacjonarne", "sprzedaż stacjonarna" (shops,
//     not offices), "spotkanie online/stacjonarne" (the interview).
//   - bare "hybrydowy/hybrydowe": "środowiska on-premises i hybrydowe" (a cloud), hybrid
//     cars. "hybrydowo" stays: the adverb only describes how someone works.
//   - bare "home office": German and English prose reach this list too.
//   - "w biurze" alone: "zajęcia sportowe w biurze", "spotkanie w biurze z kierownikiem".
var polishWorkModePhrases = []struct {
	mode    string
	phrases []string
}{
	{"remote", []string{
		"100% zdalnie", "100 % zdalnie", "100% zdalna", "100% zdalny", "100% zdalnym",
		"100% zdalnej", "100% zdalnego", "100% pracy zdalnej", "100% praca zdalna",
		"w pelni zdalnie", "w pelni zdalna", "w pelni zdalny", "w pelni zdalnym",
		"w pelni zdalnej", "w pelni zdalnego",
		"calkowicie zdalnie", "calkowicie zdalna", "wylacznie zdalnie", "wylacznie zdalna",
		"praca zdalna: w calosci", "praca zdalna: mozliwa w calosci",
		"model pracy: zdalny", "model pracy: zdalnie", "model pracy: zdalna",
		"model pracy: praca zdalna", "model pracy: remote",
		"tryb pracy: zdalny", "tryb pracy: zdalnie", "tryb pracy: zdalna",
		"tryb pracy: praca zdalna", "tryb pracy: remote",
		"forma pracy: zdalna", "forma pracy: praca zdalna", "forma pracy: zdalnie",
		"lokalizacja: zdalnie", "lokalizacja: praca zdalna", "lokalizacja: remote",
		"miejsce pracy: zdalnie", "miejsce pracy: praca zdalna",
		"praca: zdalna", "praca w trybie zdalnym", "pracy w trybie zdalnym",
		"pracujemy w modelu zdalnym", "wspolpracy w modelu zdalnym",
		"praca zdalna z dowolnego miejsca", "pracy zdalnej z dowolnego miejsca",
	}},
	{"hybrid", []string{
		"praca zdalna: hybrydowo", "praca zdalna: mozliwa czesciowo",
		"praca zdalna: czesciowo",
		"praca hybrydowa", "pracy hybrydowej", "prace hybrydowa", "praca w trybie hybrydowym",
		"pracy w trybie hybrydowym", "w trybie hybrydowym", "w modelu hybrydowym",
		"modelu pracy hybrydowej", "model hybrydowy", "w systemie hybrydowym",
		"hybrydowy system pracy", "wspolpraca hybrydowa", "wspolpracy hybrydowej",
		"model pracy: hybrydowy", "model pracy: hybrydowa", "model pracy: hybrydowo",
		"tryb pracy: hybrydowy", "tryb pracy: hybrydowa", "tryb pracy: hybrydowo",
		"forma pracy: hybrydowa", "lokalizacja: hybrydowo", "hybrydowo",
		"praca zdalna i hybrydowa", "mozliwosc pracy zdalnej", "mozliwoscia pracy zdalnej",
		"mozliwosc czesciowej pracy zdalnej", "czesciowo zdalnie", "czesciowej pracy zdalnej",
		"dni w biurze", "dni z biura", "dzien w biurze", "dzien z biura",
		"dni pracy zdalnej", "dni zdalnie", "dni zdalnych", "dni pracy z domu",
	}},
	{"onsite", []string{
		"praca zdalna: brak", "praca stacjonarna", "pracy stacjonarnej", "prace stacjonarna",
		"praca w trybie stacjonarnym", "pracy w trybie stacjonarnym",
		"model pracy: stacjonarny", "model pracy: stacjonarna", "model pracy: stacjonarnie",
		"tryb pracy: stacjonarny", "tryb pracy: stacjonarna", "tryb pracy: stacjonarnie",
		"forma pracy: stacjonarna", "100% stacjonarnie", "100% stacjonarna",
		"brak mozliwosci pracy zdalnej", "nie ma mozliwosci pracy zdalnej",
	}},
}

// polishNegations, found just before a remote or hybrid phrase in its clause, turn it
// into its opposite ("nie oferujemy pracy hybrydowej", "bez możliwości pracy zdalnej"),
// and the hiring process's own nouns withdraw it the same way: "Rekrutacja: 100%
// zdalna", "proces rekrutacyjny (100% zdalnie)" and "rozmowa w trybie hybrydowym" describe
// the interview, not the job. A platform, a cloud or an environment "w modelu
// hybrydowym" is a hybrid cloud.
var polishNegations = []string{"nie ", "brak ", "bez ", "rekrutac", "proces rekrut",
	"rozmow", "spotkani", "onboarding", "szkoleni", "lekcj", "zajec", "kurs",
	"platform", "chmur", "srodowisk", "infrastruktur", "architektur"}

// polishRefusals, found just AFTER a remote or hybrid phrase in its sentence, withdraw it
// ("praca zdalna: nie", "możliwość pracy zdalnej nie jest przewidziana"), and a cloud
// after a hybrid model names the architecture ("w modelu hybrydowym oraz multi-cloud").
var polishRefusals = []string{": nie", ": brak", "nie jest mozliw", "niemozliw",
	"nie jest przewidzian", "nie przewidujemy", "cloud", "chmur", "on-prem"}

// polishPerks, in the same sentence as a remote or hybrid phrase, make it a BOUNDED perk
// rather than the arrangement: "20 dni pracy zdalnej w roku", "praca zdalna w okresie
// wakacyjnym".
var polishPerks = []string{"w roku", "wakacj", "covid", "pandemi"}

// polishNegationWindow reaches over the noun Polish puts between a negation and the
// phrase: "brak możliwości pracy zdalnej".
const polishNegationWindow = 24

var polishGuards = phraseGuards{negations: polishNegations, refusals: polishRefusals,
	perks: polishPerks, negationWindow: polishNegationWindow}

// polishWorkModeTriggers gate the phrase walk, spelled as the folded text spells them.
var polishWorkModeTriggers = []string{"zdaln", "hybrydow", "stacjonarn", "w biurze", "z biura"}

// polishModeChoice offers the job in more than one model — "możliwość pracy stacjonarnej,
// zdalnej lub hybrydowej", "praca zdalna lub hybrydowa" — which is a flexible arrangement,
// read as hybrid before the phrase list, whose onsite "pracy stacjonarnej" it contains.
var polishModeChoice = regexp.MustCompile(`\b(?:prac[aey]|pracowac|model\p{L}*|tryb\p{L}*|form\p{L}*)\s+(?:w\s+(?:trybie|modelu)\s+)?(?:stacjonarn|zdaln|hybrydow)\p{L}*(?:,\s*(?:stacjonarn|zdaln|hybrydow)\p{L}*)*\s+(?:lub|i|albo|badz)\s+(?:w\s+pelni\s+)?(?:stacjonarn|zdaln|hybrydow)`)

// polishSplitWeek is a week split between the office and home stated as a share below a
// hundred percent ("80% zdalnie", "60% pracy zdalnej"); a hundred percent is the phrase
// list's.
var polishSplitWeek = regexp.MustCompile(`\b[1-9]\d? ?% (?:zdalnie|zdalna|zdalnej|pracy zdalnej|w biurze|stacjonarnie|z biura)\b`)

// polishWorkMode derives a work mode from Polish arrangement prose, "" when none of
// polishWorkModePhrases is stated.
func polishWorkMode(desc string) string {
	text := strings.ReplaceAll(dropAccents(foldMarkup(desc)), "ł", "l")
	if !containsAny(text, polishWorkModeTriggers) {
		return ""
	}
	for _, wm := range polishWorkModePhrases {
		if wm.mode == "hybrid" && polishFlexible(text) {
			return "hybrid"
		}
		for _, p := range wm.phrases {
			if guardedPhraseMatches(text, p, wm.mode, polishGuards) {
				return wm.mode
			}
		}
	}
	return ""
}

// polishFlexible reports a choice of models or a split week, each under the same guards
// as the phrase list ("nie oferujemy 50% pracy zdalnej" is not one).
func polishFlexible(text string) bool {
	for _, re := range []*regexp.Regexp{polishModeChoice, polishSplitWeek} {
		for _, m := range re.FindAllStringIndex(text, -1) {
			if !polishGuards.withdraws(text, m[0], m[1]) {
				return true
			}
		}
	}
	return false
}
