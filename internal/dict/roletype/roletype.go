// Package roletype resolves whether a job title names a people-management role.
// It is a curated dictionary, not a model — the same doctrine as internal/dict/classify,
// internal/dict/roletag and internal/dict/skilltag: it emits the canonical value for what it
// can resolve and nothing for what it cannot, and it never guesses.
//
// The vocabulary (vocab.RoleTypeValues) holds ONE value. "This role manages people" is legible
// from a title; "this role does not" is not (titles that state IC status outright are 0.3% of
// postings), so the absence of a marker is an unresolved posting, not a resolved individual
// contributor. Nothing downstream may present the absence of a value as evidence of the opposite.
//
// The signal is orthogonal to the existing facets: most marker-carrying titles are reached by
// neither the old category=management nor seniority lead-or-c_level.
//
// It is also a PLACEMENT. The `management` category was retired because it filed every title
// whose only resolved word was "manager". A manager title now resolves the craft it names
// through internal/dict/classify; the generic people-managing titles that name no craft
// (General Manager, Assistant Manager, Store/Branch/District/Plant Manager, the Russian
// начальник/заведующий/директор) are reached by this facet alone. A search for the retired
// `category=management` is answered with role_type=people_manager.
package roletype

import (
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/dict/wordmatch"
)

// managerMarkers name a person who has reports, whatever the craft. Matched as whole
// words against the lowercased title.
var managerMarkers = []string{
	"head of",
	"director",
	// The graded forms are separate entries because matching is whole-word: "vp"
	// cannot fire inside "svp", so an SVP/EVP/AVP title would otherwise resolve to
	// nothing despite naming the same office.
	"vp",
	"svp",
	"evp",
	"avp",
	"vice president",
	"chief",
	"supervisor",
}

// craftManagers are the "… manager" forms whose managed noun is a team of people. An explicit
// allowlist, because the bare word is not a marker: about 40% of titles containing "manager" are
// individual-contributor roles (a product, project, program or account), so a title earns the
// value only by naming the craft it manages.
var craftManagers = []string{
	"engineering manager",
	"development manager",
	"software development manager",
	"software manager",
	"technology manager",
	"technical manager",
	"it manager",
	"qa manager",
	"quality assurance manager",
	"data manager",
	"design manager",
	"infrastructure manager",
	"security manager",
	"operations manager",
	"team manager",
	// The generic people-managing titles of the service economy: the store, restaurant, branch and
	// plant, and the chain's multi-unit layer above them. None names a product, project or account
	// (the object is a workplace and its people), and none names a craft, which is why the category
	// facet cannot carry them. The assistant and deputy forms are included: an Assistant Manager
	// runs the shift the manager is not on.
	"general manager",
	"assistant manager",
	"deputy manager",
	"store manager",
	"shop manager",
	"restaurant manager",
	"kitchen manager",
	"branch manager",
	"district manager",
	"area manager",
	"regional manager",
	"plant manager",
	"site manager",
	"shift manager",
	"department manager",
	"unit manager",
	"duty manager",
	"night manager",
	"floor manager",
	"center manager",
	"centre manager",
}

// headWords are the Russian titles of an office that has reports (head, chief, director),
// matched as whole words like managerMarkers. wordmatch has no morphology, so the forms a title
// carries are listed: the nominative of both genders and the genitive a deputy title uses
// ("заместитель начальника", "заместитель директора").
//
// "Менеджер" is absent for the same reason the English bare "manager" is: it is a salesperson
// ("менеджер по продажам") at least as often as a manager.
var headWords = []string{
	"руководитель", "руководителя",
	"начальник", "начальника",
	"заведующий", "заведующая", "заведующего",
	"директор", "директора",
}

// headBlindPhrases contain a headWords word while naming no people-management role, cut before
// matching like blindPhrases. A kindergarten's "музыкальный руководитель" is its music teacher,
// a "руководитель кружка" leads a hobby club, "руководитель физического воспитания" is a PE
// teacher, and a "руководитель проекта" is a project manager (the object is a project, not people).
var headBlindPhrases = []string{
	"музыкальный руководитель",
	"руководитель кружка",
	"руководитель клубного формирования",
	"руководитель физического воспитания",
	"руководитель проекта",
	"руководитель проектов",
	"ассистент руководителя",
	"помощник руководителя",
	"помощник директора",
	"ассистент директора",
	"секретарь руководителя",
	"секретарь директора",
	"советник директора",
}

// blindPhrases contain a managerMarkers word while naming no people-management role. "Chief"
// needs this: Chief Engineer, Chief Architect and Chief Scientist top an individual-contributor
// ladder, and a Chief of Staff is an exec aide. Mirrors classify.gradeBlindPhrases.
//
// The phrases are CUT from the title before matching rather than rejecting it, so a genuine
// marker elsewhere survives ("Director of Engineering, Chief Architect Track" is still a manager).
// The cut is word-boundary aware: a bare substring cut would swallow a LONGER word the phrase
// prefixes ("chief engineer" in "Chief Engineering Officer", who runs the org).
var blindPhrases = []string{
	"chief of staff",
	"chief engineer",
	"chief architect",
	"chief scientist",
}

// Derive resolves title to "people_manager" or to "". It lowercases, cuts the blind phrases,
// then looks for a marker or a craft-qualified manager. Matching is whole-word (wordmatch), so
// "vp" never fires inside "vps" and "director" never inside "directorate".
//
// "Lead" is deliberately not a marker: in this catalogue it names the IC ladder (Tech Lead,
// Lead Engineer) far more often than a manager, so a Lead title stays unresolved and is
// reachable through the seniority facet.
func Derive(title string) string {
	s := strings.ToLower(title)
	for _, phrase := range blindPhrases {
		s = cutWord(s, phrase)
	}
	for _, phrase := range headBlindPhrases {
		s = cutWord(s, phrase)
	}
	for _, list := range [][]string{managerMarkers, craftManagers, headWords} {
		for _, term := range list {
			if wordmatch.Contains(s, term, wordmatch.UnicodeBoundary) {
				return "people_manager"
			}
		}
	}
	if bcsHead.MatchString(bcsAideOfHead.ReplaceAllString(foldBCS(s), " ")) {
		return "people_manager"
	}
	return ""
}

// bcsHead is the Croatian, Serbian, Bosnian and Slovenian head of a workplace, matched on a
// folded title (foldBCS): the director, and a "voditelj", "šef" or Slovenian "vodja" of a branch,
// shop, department, service, sector, shift or group. Like the English bare "manager", the head
// words are no marker alone ("voditelj projekta" runs a project, "vodja prodaje" a sales desk),
// so only the workplace nouns after them are listed, in the genitive. Store manager and
// shift/group leaders written as one word (poslovođa, izmenovodja, skupinovodja) need no unit.
// The director counts in the nominative, and in the genitive only after a deputy ("pomoćnik
// direktora"), as the Russian "помощник директора" does (headBlindPhrases).
var bcsHead = regexp.MustCompile(`\b(?:(?:voditelj(?:ica|a|ice)?|sef(?:ica|a|ice)?|vodj[ae])\s+(?:poslovnice|maloprodajnog\s+mj?esta|prodajnog\s+mj?esta|trgovine|prodavaonice|prodavnice|odjela|odeljenja|sluzbe|sektora|smjene|smene|restorana|poslovalnice|poslovne\s+enote|izmene|oddelka|sektorja|skupine|prodajalne|restavracije)|(?:poslovodj|izmenovodj|skupinovodj)[aeiu]|direktor(?:ica|ka)?|(?:zamj?enik|zamj?enica|namestnik|namestnica)\s+(?:(?:generaln|izvrsn)\w*\s+)?direktor(?:a|ja|ice))\b`)

// bcsAideOfHead is staff who work FOR a workplace head ("Tajnica voditelja odjela"). It is cut
// with its head word so bcsHead does not read the secretary as the manager. "Pomoćnik/pomočnik"
// of a head is left alone: in a shop it is the assistant manager, as English "Assistant Manager".
var bcsAideOfHead = regexp.MustCompile(`\b(?:tajni(?:k|ca)|sekretar(?:ka|ica)?|asistent(?:ka|ica)?|administrator(?:ka|ica)?|referent(?:ka|ica)?)\s+(?:voditelj|vodj|sef|direktor)\w*`)

// bcsGenderSlash is the feminine form written after a slash ("Voditelj/ica",
// "Šef/ica"), cut so the masculine word before it reads on.
var bcsGenderSlash = regexp.MustCompile(`(\pL)\s*/\s*(?:ica|ice|ka|ke|a|e)\b`)

// bcsASCII drops the diacritics the BCS and Slovenian alphabets use, đ written "dj".
var bcsASCII = strings.NewReplacer("č", "c", "ć", "c", "š", "s", "ž", "z", "đ", "dj")

// foldBCS writes a lowercased title as the BCS and Slovenian heads are matched: Serbian Cyrillic
// in Latin, the slash feminine cut, diacritics dropped. Other Cyrillic titles pass through
// harmlessly: no bcsHead word is spelled in them.
func foldBCS(s string) string {
	s = normalize.LatinizeSerbian(s)
	s = bcsGenderSlash.ReplaceAllString(s, "$1")
	return bcsASCII.Replace(s)
}

// cutWord replaces every whole-word occurrence of phrase in s with a space. A bare substring cut
// would consume a longer word the phrase prefixes ("chief engineer" in "Chief Engineering Officer").
func cutWord(s, phrase string) string {
	var b strings.Builder
	for from := 0; ; {
		i := strings.Index(s[from:], phrase)
		if i < 0 {
			b.WriteString(s[from:])
			return b.String()
		}
		i += from
		end := i + len(phrase)
		if !wordmatch.UnicodeBoundary(s, i, end) {
			// Not a standalone occurrence — keep it and resume just past its start, so
			// an occurrence beginning inside this one is still considered.
			b.WriteString(s[from : i+1])
			from = i + 1
			continue
		}
		b.WriteString(s[from:i])
		b.WriteString(" ")
		from = end
	}
}
