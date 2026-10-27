package location

import "strings"

// portugueseWorkModePhrases is the Portuguese counterpart of spanishWorkModePhrases, built
// from the Brazilian and Portuguese crawl (gupy, solides, quickin, catho, empregos, vagas,
// inhire, pandape, smartrecruiters, workday, teamtailor, whatjobs-br/pt: 37,608 open
// postings on 2026-10-27, of which 95% stated no arrangement any reader understood).
//
// Brazilian postings state the model in a labelled line ("Modelo de trabalho: híbrido",
// "Modalidade: presencial", "Atuação: 100% remota"), and the Spanish list already reads
// the words both languages share ("100% remoto", "modelo híbrido", "100% presencial",
// "dias de home office"). This list carries the rest. Two Brazilian habits shape it:
//   - "home office" names the REMOTE model when it is the model ("Modelo de trabalho: home
//     office", "local de trabalho: home office"), and a benefit everywhere else ("auxílio
//     home office", "política de home office"), which a remote, a hybrid and — per company
//     — an onsite posting all list. Only the model forms and the counted days count.
//   - the hybrid is written as a count of office days ("2x por semana presencial", "3 dias
//     presenciais e 2 dias home office"), so the days are hybrid, as in Spanish.
//
// The order is remote, hybrid, onsite, as in the Spanish list: a hybrid statement often
// holds the onsite word ("3 dias presenciais").
//
// Absent on purpose:
//   - bare "remoto"/"trabalho remoto": "suporte remoto", "estrutura adequada para trabalho
//     remoto", "auxílio para despesas do trabalho remoto", "possibilitando o trabalho
//     remoto em alguns dias".
//   - bare "teletrabalho" and "sistema híbrido": the internship boilerplate "programa de
//     gestão por teletrabalho ou sistema híbrido de aprendizagem" sits on ~200 empregos
//     postings and says the opposite.
//   - bare "presencial": "entrevista presencial", "treinamento presencial".
//   - "trabalhe de onde quiser" / "de qualquer lugar": the work-from-anywhere perk shape
//     (freehire#2696).
var portugueseWorkModePhrases = []struct {
	mode    string
	phrases []string
}{
	{"remote", []string{
		"100 % remoto", "100 % remota", "100% em home office", "home office integral",
		"totalmente remoto", "totalmente remota", "integralmente remoto", "integralmente remota",
		"completamente remoto", "completamente remota",
		"modelo de trabalho: remoto", "modelo de trabalho remoto", "modelo de trabalho: home office",
		"modelo de trabalho: (home office)", "modelo remoto",
		"modalidade remota", "modalidade: remoto", "modalidade: remota", "modalidade de trabalho: remoto",
		"modalidade de trabalho: remota", "modalidade: home office", "modalidade home office",
		"regime remoto", "regime de trabalho remoto", "regime de trabalho: remoto",
		"regime de teletrabalho", "regime de home office", "formato remoto", "formato: remoto",
		"atuacao remota", "atuacao: remota", "atuacao: remoto", "atuacao: home office",
		"vaga remota", "vaga 100% remota", "posicao remota", "local de trabalho: remoto",
		"local de trabalho: home office",
	}},
	{"hybrid", []string{
		"modelo de trabalho hibrido", "modelo de trabalho: hibrido", "modelo hibrido",
		"regime hibrido", "regime de trabalho hibrido", "regime de trabalho: hibrido",
		"modalidade hibrida", "modalidade: hibrida", "modalidade: hibrido",
		"modalidade de trabalho: hibrida", "modalidade de trabalho: hibrido",
		"trabalho hibrido", "formato: hibrido", "atuacao hibrida", "atuacao: hibrida",
		"dias presenciais", "dias presencial", "dias no escritorio", "dias de escritorio",
		"dias escritorio", "dias em home office", "dias remotos", "dias de trabalho remoto",
		"dias em remoto", "dias de trabalho presencial", "por semana presencial", "por semana presencialmente",
		"por semana no escritorio", "na semana presencial", "vezes na semana presencial",
		"presencial + home office", "presencial e home office", "home office e presencial",
		"presencial e remoto", "com home office as", "possibilidade de home office",
		"possibilidade de trabalho remoto",
	}},
	{"onsite", []string{
		"modelo de trabalho: presencial", "modelo de trabalho presencial", "modelo presencial",
		"modalidade: presencial", "modalidade 100% presencial",
		"modalidade de trabalho: presencial", "regime presencial", "regime de trabalho presencial",
		"regime de trabalho: presencial", "atuacao presencial", "atuacao: presencial",
		"atuacao 100% presencial", "formato presencial", "formato: presencial",
		"trabalho presencial", "vaga presencial", "jornada presencial",
		"local de trabalho: presencial", "presencial em tempo integral",
		"sem home office", "nao ha home office", "nao oferecemos home office",
		"sem possibilidade de home office", "sem teletrabalho",
	}},
}

// portugueseNegations, found just before a remote or hybrid phrase, turn it into its
// opposite ("sem home office", "não é 100% remoto"). "não" is read accent-dropped.
var portugueseNegations = []string{"nao ", "sem ", "nem ", "nunca "}

// portugueseRefusals, found just AFTER a remote or hybrid phrase in the same sentence,
// withdraw it ("home office: não é possível").
var portugueseRefusals = []string{"nao e possivel", "nao disponivel", "nao se aplica",
	"nao aplicavel", "nao permitido", "nao elegivel"}

// portuguesePerks, in the same sentence as a remote or hybrid phrase, make it a BOUNDED
// perk rather than the arrangement ("home office nas férias", "no fim de ano").
var portuguesePerks = []string{"ferias", "natal", "verao", "fim de ano"}

// portugueseGuards bundles the Portuguese qualifiers for guardedPhraseMatches. The
// negation window is the Spanish one: "sem possibilidade de home office" puts a noun
// between the negation and the phrase.
var portugueseGuards = phraseGuards{negations: portugueseNegations, refusals: portugueseRefusals,
	perks: portuguesePerks, negationWindow: spanishNegationWindow}

// portugueseWorkModeTriggers gate the fold, spelled the way both the accented and the
// bare form begin.
var portugueseWorkModeTriggers = []string{"remot", "home office", "híbrid", "hibrid",
	"presencia", "teletrabalho", "escritório", "escritorio"}

// portugueseWorkMode derives a work mode from Portuguese arrangement prose, "" when none
// of portugueseWorkModePhrases is stated. lower is the description already lowercased.
// A label's colon is read with or without the space before it ("Modalidade : Presencial").
func portugueseWorkMode(desc, lower string) string {
	if !containsAny(lower, portugueseWorkModeTriggers) {
		return ""
	}
	text := strings.ReplaceAll(dropAccents(foldMarkup(desc)), " :", ":")
	for _, wm := range portugueseWorkModePhrases {
		for _, p := range wm.phrases {
			if guardedPhraseMatches(text, p, wm.mode, portugueseGuards) {
				return wm.mode
			}
		}
	}
	return ""
}
