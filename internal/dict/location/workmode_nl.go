package location

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// dutchWorkModePhrases is the Dutch counterpart of germanWorkModePhrases, built from the
// Dutch crawl (eures, recruitee, whatjobs-nl: ~42k open postings on 2026-10-25).
//
// Dutch postings share the German register: home working is mostly a BENEFIT — "hybride
// werken", "2 dagen thuiswerken", "thuiswerkvergoeding", "mogelijkheid tot thuiswerken" —
// and each of those offers home working beside an office, which is hybrid. Remote needs
// an explicit totality ("volledig remote", "100% thuiswerken", "volledig vanuit huis").
// As in German, a remote phrase can CONTAIN a hybrid one ("100% thuiswerken" holds
// "thuiswerken"), so remote is checked first. Onsite is last and fills only an explicit
// refusal of home working.
//
// Every phrase is matched against the folded text (see foldMarkup) and must end on a
// word boundary.
//
// Absent on purpose:
//   - bare "hybride": "hybride cloud", "hybride omgevingen" are
//     IT, "hybride zzp" is a contract form. Only the work-model forms count.
//   - "op afstand": mostly remote SUPPORT ("op afstand ondersteunen", "storingen op
//     afstand verhelpen"), a technical service like the German "Fernwartung".
//   - "op kantoor", "op locatie": each says where the work is, alongside a home-working
//     offer as often as not, never the arrangement on its own.
//   - "vanuit het buitenland werken": a bounded work-from-abroad perk, the Dutch shape
//     of the "work from anywhere" false positive (freehire#2696).
//   - "volledig thuis" alone: "je volledig thuis voelen" is feeling at home.
//   - "hybride functie"/"hybride rol": as often a role split between office and field
//     ("een hybride functie en bent daarom mobiel") as one split with home.
var dutchWorkModePhrases = []struct {
	mode    string
	phrases []string
}{
	{"remote", []string{
		"volledig remote", "geheel remote", "fulltime remote", "100% remote werken",
		"volledig thuiswerken", "volledig thuis werken", "volledig thuis te werken",
		"volledig vanuit huis", "geheel vanuit huis", "100% vanuit huis",
		"100% thuiswerken", "100 % thuiswerken", "100% thuis werken", "100% thuiswerk",
		"volledig plaatsonafhankelijk", "volledig op afstand werken",
		"remote functie", "remote vacature", "remote baan",
	}},
	{"hybrid", []string{
		"hybride werken", "hybride te werken", "hybride werkt", "hybride gewerkt",
		"hybride werkomgeving", "hybride werkmodel", "hybride werkvorm", "hybride werkweek",
		"hybride werkbeleid", "hybride werkmogelijkheden", "hybride werkmogelijkheid",
		"hybride werkplek", "hybride werkopties",
		"hybride manier van werken", "hybride werk", "hybride werkomstandigheden",
		"hybrid werken",
		"deels thuis", "deels vanuit huis", "deels remote", "gedeeltelijk remote",
		"gedeeltelijk thuis", "gedeeltelijk vanuit huis", "grotendeels vanuit huis",
		"grotendeels remote", "grotendeels thuis", "voornamelijk remote",
		"thuiswerken", "thuis werken", "thuis te werken", "thuiswerkt",
		"vanuit huis werken", "vanuit huis te werken", "vanuit huis werkt",
		"thuiswerkmogelijkheden", "thuiswerkmogelijkheid", "thuiswerkdag", "thuiswerkdagen",
		"thuiswerkvergoeding", "thuiswerkbudget", "thuiswerkregeling", "thuiswerkbeleid",
		"thuiswerkfaciliteiten", "thuiswerkplekvergoeding", "thuiswerkvoorzieningen",
		"thuiswerkset", "thuiswerkpakket",
		"plaats- en tijdonafhankelijk", "tijd- en plaatsonafhankelijk",
		"remote werken", "remote te werken",
	}},
	{"onsite", []string{
		"thuiswerken is niet mogelijk", "thuiswerken is helaas niet mogelijk",
		"thuiswerken is in deze functie niet mogelijk", "niet mogelijk om thuis te werken",
		"niet mogelijk om vanuit huis te werken", "geen thuiswerk", "geen thuiswerken",
		"geen mogelijkheid tot thuiswerken", "geen mogelijkheid om thuis te werken",
		// Not "niet remote": every live hit was a support desk's "incidenten die niet
		// remote verholpen kunnen worden", the same remote-support sense as "op afstand".
	}},
}

// dutchNegations, found just before a remote or hybrid phrase, turn it into its opposite
// ("geen thuiswerken", "niet volledig remote"). As in German, the onsite family carries
// its own negations as whole phrases.
var dutchNegations = []string{"geen ", "niet ", "zonder ", "nooit "}

// dutchRefusals, found just AFTER a remote or hybrid phrase in the same sentence,
// withdraw it ("thuiswerken is in deze functie helaas niet mogelijk").
var dutchRefusals = []string{"niet mogelijk", "niet toegestaan", "niet van toepassing",
	"niet bespreekbaar", "helaas niet", "uitgesloten"}

// dutchPerks, in the same sentence as a remote or hybrid phrase, make it a BOUNDED perk
// rather than the arrangement: "twee weken per jaar remote werken vanuit het
// buitenland", "volledig remote werken vanuit het buitenland is niet mogelijk". This is
// the Dutch shape of the travel-perk guard on "work from anywhere" (freehire#2696).
var dutchPerks = []string{"buitenland", "per jaar"}

// dutchWorkModeTriggers gate the fold, as germanWorkModeTriggers do: each is one word,
// so neither a tag nor an entity can split it.
var dutchWorkModeTriggers = []string{"thuis", "huis", "hybride", "hybrid", "remote",
	"onafhankelijk"}

// dutchWorkMode derives a work mode from Dutch arrangement prose, "" when none of
// dutchWorkModePhrases is stated. lower is the description already lowercased.
func dutchWorkMode(desc, lower string) string {
	if !containsAny(lower, dutchWorkModeTriggers) {
		return ""
	}
	text := foldMarkup(desc)
	for _, wm := range dutchWorkModePhrases {
		for _, p := range wm.phrases {
			if dutchPhraseMatches(text, p, wm.mode) {
				return wm.mode
			}
		}
	}
	return ""
}

// dutchPhraseMatches reports whether p occurs in text as a whole phrase, read for the
// family mode it belongs to. A remote or hybrid occurrence that a dutchNegations word
// precedes, or a dutchRefusals phrase follows in its sentence, is passed over and the scan
// continues — the shape germanPhraseMatches has, with the Dutch words and the same
// windows. The onsite family is the refusals themselves, so neither check applies to it.
func dutchPhraseMatches(text, p, mode string) bool {
	offset := 0
	for {
		i := strings.Index(text[offset:], p)
		if i < 0 {
			return false
		}
		start, end := offset+i, offset+i+len(p)
		offset = end
		if r, _ := utf8.DecodeRuneInString(text[end:]); end < len(text) && unicode.IsLetter(r) {
			continue
		}
		if start > 0 {
			if r, _ := utf8.DecodeLastRuneInString(text[:start]); unicode.IsLetter(r) {
				continue
			}
		}
		if mode == "onsite" {
			return true
		}
		if containsAny(text[max(0, start-germanNegationWindow):start], dutchNegations) {
			continue
		}
		tail := text[end:min(len(text), end+germanRefusalWindow)]
		if j := strings.IndexAny(tail, ".;!?"); j >= 0 {
			tail = tail[:j]
		}
		if containsAny(tail, dutchRefusals) {
			continue
		}
		if containsAny(sentenceAround(text, start, end), dutchPerks) {
			continue
		}
		return true
	}
}
