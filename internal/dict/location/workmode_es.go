package location

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// spanishWorkModePhrases is the Spanish counterpart of dutchWorkModePhrases, built from
// the Spanish-language crawl (infojobs, occ, pandapelatam, teamtailor, factorial,
// getonbrd, torre, hiringroom, personio, eures: ~25k open postings on 2026-10-27, of which
// the ~6k that carry no structured work mode are what this list fills).
//
// Spanish states the arrangement more often than German or Dutch do ("Modalidad:
// híbrida", "100% remoto", "modalidad presencial"), but home working is still listed as a
// BENEFIT too — "teletrabajo" in a perks list, "ayuda al teletrabajo", "2 días de
// teletrabajo", "home office" — and each of those offers home working beside an office,
// which is hybrid. Remote needs an explicit totality ("100% remoto", "totalmente
// remoto", "100% teletrabajo"). A remote phrase can CONTAIN a hybrid one ("100%
// teletrabajo" holds "teletrabajo"), and a hybrid statement often contains the onsite
// word ("3 días presenciales"), so the order is remote, hybrid, onsite, as in German.
//
// Every phrase is matched against the folded text (see foldMarkup) with its accents
// dropped ("híbrida" → "hibrida"), so each is written without accents, and must start and
// end on a word boundary.
//
// Absent on purpose:
//   - bare "remoto"/"remota": "soporte remoto", "dispensación remota", "colaborar de forma
//     remota" — a technical service or a way of working with a team, not the arrangement.
//   - bare "híbrido": "entornos tecnológicos híbridos", "aplicaciones nativas/híbridas".
//   - bare "presencial": "clases presenciales o en línea" (a tutoring platform's
//     boilerplate, ~5k rows), "entrevista presencial", "formación presencial". Only the
//     work-model forms count.
//   - "trabajo desde casa": the LinkedIn line "Hay opciones de teletrabajo/trabajo desde
//     casa disponibles" is an option, read through "teletrabajo" as hybrid; alone it is
//     as often a remote call-centre post as a perk.
//   - "desde cualquier lugar": the "work from anywhere" perk shape (freehire#2696).
//   - bare "home office": German and English prose reach this list too, and "Home Office
//     ist leider nicht möglich" or "equip your home office" are not a Spanish offer. Only
//     the Spanish forms around it count ("2 días de home office", "apoyo de home office").
//   - "teletrabajo completo en verano/agosto": a seasonal perk, guarded by spanishPerks.
var spanishWorkModePhrases = []struct {
	mode    string
	phrases []string
}{
	{"remote", []string{
		"100% remoto", "100% remota", "100 % remoto", "100 % remota", "100% en remoto",
		"100% teletrabajo", "100 % teletrabajo", "teletrabajo 100%", "teletrabajo al 100%",
		"teletrabajo completo", "teletrabajo total", "full remoto", "full remote",
		"totalmente remoto", "totalmente remota", "completamente remoto", "completamente remota",
		"totalmente en remoto", "completamente en remoto",
		"modalidad remota", "modalidad remoto", "modalidad: remoto", "modalidad: remota",
		"modalidad de trabajo: remoto", "modalidad de trabajo: remota",
		"modalidad de trabajo: en remoto", "modalidad de trabajo remoto",
		"modalidad: teletrabajo", "modalidad teletrabajo", "modalidad de teletrabajo",
		"posicion es remota", "posicion remota", "puesto remoto", "rol remoto",
		"100% home office", "100 % home office", "full home office", "home office 100%",
		"100% desde casa", "100 % desde casa",
	}},
	{"hybrid", []string{
		"modalidad hibrida", "modalidad hibrido", "modalidad: hibrida", "modalidad: hibrido",
		"modalidad de trabajo: hibrida", "modalidad de trabajo: hibrido",
		"modalidad de trabajo hibrida", "modalidad de trabajo hibrido", "modalidad mixta",
		"tipo de jornada: hibrida", "jornada: hibrida",
		"presencial/hibrido", "presencial / hibrido", "presencial o hibrido",
		"modelo hibrido", "modelo de trabajo hibrido", "esquema hibrido", "formato hibrido",
		"trabajo hibrido", "regimen hibrido", "jornada hibrida", "de forma hibrida",
		"entorno de trabajo hibrido", "posicion es hibrida", "puesto es hibrido",
		"posicion hibrida", "puesto hibrido", "rol hibrido",
		"dias presenciales", "dia presencial", "dias de presencialidad",
		"dias a la semana presencial", "dias por semana presencial",
		"dias en oficina", "dias en la oficina", "dia en oficina", "dia en la oficina",
		"dias de teletrabajo", "dia de teletrabajo", "dias en remoto", "dia en remoto",
		"dias remoto", "dia remoto", "dias de home office", "dias home office",
		"dia de home office", "dia home office", "dias en home office", "dias desde casa",
		"presencial y remoto", "remoto y presencial", "presencial y teletrabajo",
		"teletrabajo y presencial", "oficina y teletrabajo", "oficina y remoto",
		"teletrabajo parcial", "teletrabajo flexible", "opciones de teletrabajo",
		"opcion de teletrabajo", "posibilidad de teletrabajo", "politica de teletrabajo",
		"ayuda al teletrabajo", "apoyo de home office", "opcion de home office",
		"teletrabajo",
	}},
	{"onsite", []string{
		"100% presencial", "100 % presencial", "totalmente presencial",
		"modalidad presencial", "modalidad: presencial", "modalidad de trabajo: presencial",
		"modalidad de trabajo presencial", "empleo presencial", "trabajo presencial",
		"puesto presencial", "posicion presencial",
		"sin teletrabajo", "sin opcion de teletrabajo", "sin posibilidad de teletrabajo",
		"no hay teletrabajo", "no se ofrece teletrabajo", "sin home office",
	}},
}

// spanishNegations, found just before a remote or hybrid phrase, turn it into its
// opposite ("sin teletrabajo", "no es 100% remoto").
//
// "evaluar" is a hedge, not a negation, but it withdraws the same way: "Modalidad:
// híbrida, con posibilidad de evaluar 100% remoto" is hybrid.
//
// Portuguese prose reaches this list before its own (workmode_pt.go) and shares many of
// its phrases ("100% remoto", "modelo híbrido"), so the Portuguese "não" and "sem" are
// read here too: "esta vaga não é 100% remota" is not a remote offer.
var spanishNegations = []string{"no ", "sin ", "ni ", "nunca ", "evaluar ", "nao ", "sem "}

// spanishNegationWindow is wider than the German one: a Spanish refusal puts a verb or a
// noun between the negation and the phrase ("no se ofrece teletrabajo", "sin posibilidad
// de teletrabajo").
const spanishNegationWindow = 20

// spanishRefusals, found just AFTER a remote or hybrid phrase in the same sentence,
// withdraw it ("teletrabajo/remoto no corresponde", "teletrabajo: no disponible").
var spanishRefusals = []string{"no es posible", "no disponible", "no aplica",
	"no corresponde", "no se contempla", "no esta disponible"}

// spanishPerks, in the same sentence as a remote or hybrid phrase, make it a BOUNDED
// perk rather than the arrangement: "teletrabajo completo en verano, Navidad y Semana
// Santa", "jornada intensiva y teletrabajo durante agosto". A month alone is not one —
// "Incorporación en agosto, modalidad 100% remoto" starts in August — so a month counts
// only after "durante".
var spanishPerks = []string{"verano", "navidad", "semana santa", "durante agosto",
	"durante julio", "durante el mes de agosto", "julio y agosto"}

// spanishGuards bundles the Spanish qualifiers for guardedPhraseMatches.
var spanishGuards = phraseGuards{negations: spanishNegations, refusals: spanishRefusals,
	perks: spanishPerks, negationWindow: spanishNegationWindow}

// spanishWorkModeTriggers gate the fold: each is one word, so neither a tag nor an entity
// can split it, and each is spelled the way both the accented and the bare form begin.
var spanishWorkModeTriggers = []string{"remot", "teletrabajo", "presencial", "home",
	"híbrid", "hibrid", "oficina", "desde casa", "mixta"}

// spanishWorkMode derives a work mode from Spanish arrangement prose, "" when none of
// spanishWorkModePhrases is stated. lower is the description already lowercased.
func spanishWorkMode(desc, lower string) string {
	if !containsAny(lower, spanishWorkModeTriggers) {
		return ""
	}
	text := dropAccents(foldMarkup(desc))
	for _, wm := range spanishWorkModePhrases {
		for _, p := range wm.phrases {
			if guardedPhraseMatches(text, p, wm.mode, spanishGuards) {
				return wm.mode
			}
		}
	}
	return ""
}

// dropAccents removes the combining marks from text, so "híbrida" reads "hibrida" and
// "días" reads "dias".
func dropAccents(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range norm.NFD.String(text) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
