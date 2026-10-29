package location

import "strings"

// bcsWorkModePhrases is the Croatian, Serbian and Bosnian counterpart of
// spanishWorkModePhrases, built from the open postings placed in hr/rs/ba/me
// (talentlyft, eures, zohorecruit, smartrecruiters, teamtailor: ~1,600 BCS-language
// postings on 2026-10-29, 0.1% of which a description reader resolved; 6.9% after).
//
// BCS lists home working as a BENEFIT, as German does: "mogućnost rada od kuće",
// "mogućnost povremenog rada od kuće", "mogućnost rada na daljinu", "hibridni rad i
// fleksibilno radno vrijeme" in a "Što nudimo" list. Each offers home working beside an
// office, which is hybrid. Remote needs the place of work to BE the home ("Mjesto rada:
// rad od kuće", "Lokacija: remote") or an explicit totality ("100% remote", "potpuno
// udaljeni rad", "svi radimo od kuće"). A remote phrase can contain a hybrid one, so the
// order is remote, hybrid, onsite.
//
// Every phrase is matched against the folded text (foldMarkup, then dropAccents: "kuće"
// reads "kuce", "načina" reads "nacina"; "đ" stays), so each is written that way, and
// must start and end on a word boundary.
//
// Absent on purpose:
//   - bare "rad na daljinu" and bare "rad od kuće": almost always the "mogućnost" perk,
//     and "rad na terenu i na daljinu" is field work done partly by phone. Only the forms
//     listed below count.
//   - bare "remote": "remote dežurstvo" (remote on-call duty), "(remote i lokalno)"
//     support. The English list reads the English phrases before this one.
//   - "rad se odvija od kuće": a support desk's weekend on-call duty ("rad se odvija od
//     kuće (remote dežurstvo) vikendom"), beside "rad se odvija iz ureda" on weekdays.
//   - "daljinski" (remotely controlled): "daljinski upravljati STS dizalicom" is a crane.
//   - "u uredu" alone: "kavu i slatkiše u uredu", "geodetski poslovi na terenu i u uredu".
var bcsWorkModePhrases = []struct {
	mode    string
	phrases []string
}{
	{"remote", []string{
		"mjesto rada: rad od kuce", "mesto rada: rad od kuce", "mjesto rada: rad na daljinu",
		"mesto rada: rad na daljinu", "mjesto rada: remote", "mesto rada: remote",
		"lokacija: remote", "lokacija: rad od kuce", "lokacija: rad na daljinu",
		"100% remote", "100 % remote", "100% rad od kuce", "100% od kuce", "100% na daljinu",
		"100% rad na daljinu", "potpuno udaljeni rad", "potpuno remote", "potpuno na daljinu",
		"u potpunosti na daljinu", "u potpunosti od kuce", "iskljucivo od kuce",
		"iskljucivo rad od kuce", "iskljucivo na daljinu", "svi radimo od kuce",
		"remote first", "posao se obavlja od kuce", "potpunog remote rada",
	}},
	{"hybrid", []string{
		"hibridni rad", "hibridnog rada", "hibridni model rada", "hibridnog modela rada",
		"hibridni nacin rada", "hibridnog nacina rada", "hibridni oblik rada",
		"hibridnom modelu", "hibridno", "mjesto rada: hibrid", "mesto rada: hibrid",
		"mogucnost rada od kuce", "mogucnost rada na daljinu", "mogucnost povremenog rada od kuce",
		"mogucnost rada iz kuce", "povremeni rad od kuce", "povremenog rada od kuce",
		"rad od kuce i u uredu", "rad od kuce i iz ureda", "rada od kuce i rada iz ureda",
		"kombinacija rada od kuce", "kombiniramo rad od kuce", "kombinujemo rad od kuce",
		"djelomican remote rad", "delimican remote rad", "djelomicni rad od kuce",
		"remote rad", "remote rada", "moguc je i rad na daljinu", "moguc rad od kuce",
		"dana rada od kuce", "dana od kuce", "dan od kuce", "dana u uredu", "dana iz ureda",
		"dana u kancelariji", "dana iz kancelarije",
		"rad od kuce / rad iz kancelarije", "rad od kuce / rad iz ureda",
		"rad od kuce ili iz ureda", "rad od kuce ili iz kancelarije",
	}},
	{"onsite", []string{
		"rad iz ureda", "rad iz kancelarije", "rad se odvija iz ureda", "rad u uredu u", "rad u kancelariji u",
		"iskljucivo iz ureda", "iskljucivo iz kancelarije", "nije rad od kuce",
		"nije moguc rad od kuce", "nema rada od kuce", "bez rada od kuce",
		"bez mogucnosti rada od kuce",
	}},
}

// bcsNegations, found just before a remote or hybrid phrase, turn it into its opposite
// ("nije moguć rad od kuće", "bez mogućnosti rada od kuće"). "nema" and "bez" lead a noun
// phrase, so the window is the Spanish one.
var bcsNegations = []string{"nije ", "nisu ", "ne ", "nema ", "bez ", "ni "}

// bcsRefusals, found just AFTER a remote or hybrid phrase in the same sentence, withdraw
// it ("rad od kuće nije moguć").
var bcsRefusals = []string{"nije moguc", "nije moguca", "nije dozvoljen", "nije dopusten",
	"ne postoji", "nije opcija"}

// bcsGuards bundles the BCS qualifiers for guardedPhraseMatches.
var bcsGuards = phraseGuards{negations: bcsNegations, refusals: bcsRefusals,
	negationWindow: spanishNegationWindow}

// bcsWorkModeTriggers gate the fold: each is spelled the way both the accented and the
// bare form begin.
var bcsWorkModeTriggers = []string{"od ku", "daljin", "remote", "hibrid", "ured",
	"kancelarij"}

// bcsWorkMode derives a work mode from Croatian, Serbian or Bosnian arrangement prose,
// "" when none of bcsWorkModePhrases is stated. lower is the description already
// lowercased.
func bcsWorkMode(desc, lower string) string {
	if !containsAny(lower, bcsWorkModeTriggers) {
		return ""
	}
	text := strings.ReplaceAll(dropAccents(foldMarkup(desc)), " :", ":")
	for _, wm := range bcsWorkModePhrases {
		for _, p := range wm.phrases {
			if guardedPhraseMatches(text, p, wm.mode, bcsGuards) {
				return wm.mode
			}
		}
	}
	return ""
}
