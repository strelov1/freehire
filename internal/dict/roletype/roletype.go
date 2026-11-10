// Package roletype resolves whether a job title names a people-management role.
// It is a curated dictionary, not a model — the same doctrine as internal/dict/classify,
// internal/dict/roletag and internal/dict/skilltag: it emits the canonical value for what it
// can resolve and nothing for what it cannot, and it never guesses.
//
// The vocabulary (vocab.RoleTypeValues) holds ONE value. "This role manages people"
// is legible from a title; "this role does not" is not, and the absence of a marker
// is therefore an unresolved posting rather than a resolved individual contributor.
// Measured on production, the titles that state IC status outright — the
// staff/principal ladder, "member of technical staff", the literal phrase — are
// about 9,100 of 3.1M postings, 0.3%. Nothing downstream may present the absence of
// a value as evidence of the opposite.
//
// The signal is orthogonal to the facets that already exist: of the 171,726
// production titles carrying a marker, only 15,760 sat in the old category=management
// and only 46,330 carry seniority lead-or-c_level, leaving 111,786 reachable by neither.
//
// It is also a PLACEMENT, not only a filter. The `management` category was retired
// (2026-10): it filed every title whose only resolved word was "manager", which said
// nothing about the work. A manager title now resolves the craft it names through
// internal/dict/classify, and the generic people-managing titles that name no craft —
// General Manager, Assistant Manager, Store/Branch/District/Plant Manager, the Russian
// начальник/заведующий/директор — are reached by this facet alone. Search therefore keeps
// a posting this package marks even when no category files it (search.CategoryUnresolved),
// and the ingest filter does not refuse one (classify.ConfirmedNonTech). A
// search for the retired `category=management` is answered with role_type=people_manager.
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

// craftManagers are the "… manager" forms whose managed noun is a team of people.
// They are an explicit allowlist because the bare word is not a marker: on production
// 150,779 of the 378,410 titles containing "manager" are individual-contributor roles
// where the managed noun is a product, a project, a program or an account. Admitting
// "manager" alone would make this facet wrong for two matches in five, so a title
// earns the value only by naming the craft it manages.
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
	// The generic people-managing titles of the service economy: the store, the
	// restaurant, the branch and the plant, and the chain's multi-unit layer above them.
	// None names a product, a project or an account — the object of every one is a
	// workplace and the people in it — and none names a craft, which is why the
	// category facet cannot carry them: with `management` retired, this facet is the
	// only one that reaches a General Manager, and search keeps a posting it reaches
	// (search.CategoryUnresolved).
	//
	// The assistant and deputy forms are included: an Assistant Manager runs the shift
	// the manager is not on, and is named for exactly that.
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

// headWords are the Russian titles of an office that has reports — the head, the
// chief, the director — matched as whole words the way managerMarkers are. Russian
// inflects and wordmatch has no morphology, so the forms a title actually carries are
// listed: the nominative of both genders, and the genitive a deputy title puts its
// office in ("заместитель начальника", "заместитель директора").
//
// "Менеджер" is not here for the reason the English bare "manager" is not: it is the
// word for a salesperson ("менеджер по продажам") at least as often as for a manager.
var headWords = []string{
	"руководитель", "руководителя",
	"начальник", "начальника",
	"заведующий", "заведующая", "заведующего",
	"директор", "директора",
}

// headBlindPhrases contain a headWords word while naming no people-management role,
// cut before matching exactly as blindPhrases are. The kindergarten's "музыкальный
// руководитель" is its music teacher, a "руководитель кружка" leads a hobby club for
// children (a culture house's "руководитель клубного формирования" the same), a
// college's "руководитель физического воспитания" is its PE teacher, and a
// "руководитель проекта" is the project manager — the object is a project, not people,
// the same reason the English "project manager" earns nothing here.
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

// blindPhrases contain a managerMarkers word while naming no people-management role.
// "Chief" is the marker that needs this: Chief Engineer, Chief Architect and Chief
// Scientist are the top of an individual-contributor ladder, and a Chief of Staff is
// an exec aide whose reports are usually nobody. This mirrors
// classify.gradeBlindPhrases, which solves the same shape of problem for grades.
//
// The phrases are CUT from the title before matching rather than used to reject it,
// so a genuine marker elsewhere in the same title survives — "Director of
// Engineering, Chief Architect Track" is still a manager because of "director".
//
// The cut is word-boundary aware for the same reason the matching is. A bare
// substring cut would also swallow any LONGER word a phrase happens to prefix:
// "chief engineer" prefixes "chief engineering", so "Chief Engineering Officer" —
// who runs the org — lost its marker and resolved to nothing.
var blindPhrases = []string{
	"chief of staff",
	"chief engineer",
	"chief architect",
	"chief scientist",
}

// Derive resolves title to "people_manager" or to "". It lowercases, cuts the blind
// phrases, then looks for a marker or a craft-qualified manager. Matching is
// whole-word (via wordmatch), so "vp" never fires inside "vps" and "director" never
// inside "directorate".
//
// Note what is deliberately absent. "Lead" is not a marker: production's
// seniority=lead holds 116,893 postings and only 3,303 carry any management marker,
// so in this catalogue the word names the IC ladder (Tech Lead, Lead Engineer) far
// more often than a manager. Resolving it either way would be a guess, so a Lead
// title stays unresolved and reachable through the seniority facet instead.
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

// bcsHead is the Croatian, Serbian, Bosnian and Slovenian head of a workplace, matched
// on a folded title (foldBCS): the director (English "director"), and a "voditelj",
// "šef" or Slovenian "vodja" of a branch, shop, department, service, sector, shift or
// group — the English "Branch/Store/Department/Shift/Team Manager" and "Head of". The
// head words are the manager nouns of these languages, and like the English bare
// "manager" none is a marker alone: "voditelj projekta" runs a project, "vodja
// prodaje" a sales desk, so only the workplace nouns after them are listed, in the
// genitive the title puts them in. The store manager and the shift and group leaders
// written as one word (poslovođa, izmenovodja, skupinovodja) need no unit. The
// director counts in the nominative, and in the genitive only after a deputy: "u
// uredu direktora", "v tajništvu generalnega direktorja", "pomoćnik direktora" work
// for the director, as the Russian "помощник директора" does (headBlindPhrases).
var bcsHead = regexp.MustCompile(`\b(?:(?:voditelj(?:ica|a|ice)?|sef(?:ica|a|ice)?|vodj[ae])\s+(?:poslovnice|maloprodajnog\s+mj?esta|prodajnog\s+mj?esta|trgovine|prodavaonice|prodavnice|odjela|odeljenja|sluzbe|sektora|smjene|smene|restorana|poslovalnice|poslovne\s+enote|izmene|oddelka|sektorja|skupine|prodajalne|restavracije)|(?:poslovodj|izmenovodj|skupinovodj)[aeiu]|direktor(?:ica|ka)?|(?:zamj?enik|zamj?enica|namestnik|namestnica)\s+(?:(?:generaln|izvrsn)\w*\s+)?direktor(?:a|ja|ice))\b`)

// bcsAideOfHead is staff who work FOR a workplace head: "Tajnica voditelja odjela",
// "Asistent vodje sektorja". The head word is cut with its aide so bcsHead does not read
// the secretary as the manager. The "pomoćnik/pomočnik" of a head is left alone: in a
// shop it is the assistant manager ("Pomočnik vodje poslovne enote"), whom English
// "Assistant Manager" marks too.
var bcsAideOfHead = regexp.MustCompile(`\b(?:tajni(?:k|ca)|sekretar(?:ka|ica)?|asistent(?:ka|ica)?|administrator(?:ka|ica)?|referent(?:ka|ica)?)\s+(?:voditelj|vodj|sef|direktor)\w*`)

// bcsGenderSlash is the feminine form written after a slash ("Voditelj/ica",
// "Šef/ica"), cut so the masculine word before it reads on.
var bcsGenderSlash = regexp.MustCompile(`(\pL)\s*/\s*(?:ica|ice|ka|ke|a|e)\b`)

// bcsASCII drops the diacritics the BCS and Slovenian alphabets use, đ written "dj".
var bcsASCII = strings.NewReplacer("č", "c", "ć", "c", "š", "s", "ž", "z", "đ", "dj")

// foldBCS writes a lowercased title as the BCS and Slovenian heads are matched: Serbian
// Cyrillic in Latin, the slash feminine cut, the diacritics dropped. Russian and other
// Cyrillic titles pass through the transliteration harmlessly — no bcsHead word is
// spelled in them, and their own heads were read above.
func foldBCS(s string) string {
	s = normalize.LatinizeSerbian(s)
	s = bcsGenderSlash.ReplaceAllString(s, "$1")
	return bcsASCII.Replace(s)
}

// cutWord replaces every whole-word occurrence of phrase in s with a space. It is
// the mask's counterpart to wordmatch.Contains: cutting on a bare substring would
// consume any longer word the phrase prefixes, which is how the "chief engineer"
// mask silently blinded "Chief Engineering Officer".
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
