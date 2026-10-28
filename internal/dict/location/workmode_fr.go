package location

import "strings"

// frenchWorkModePhrases is the French counterpart of spanishWorkModePhrases, built from the
// French-language crawl (wttj, flatchr, werecruit, smartrecruiters, teamtailor, workday,
// eures, recruitee, lever, join, workable, greenhouse, personio: 24,609 open postings on
// 2026-10-28, of which 69% carried no structured work mode and 0.4% stated one any reader
// understood).
//
// French lists home working as a BENEFIT, as German does — "télétravail partiel possible",
// "jusqu'à 2 jours de télétravail par semaine", "accord télétravail", "subvention
// télétravail", a "charte de télétravail" — and each of those offers home working beside an
// office, which is hybrid. Remote needs an explicit totality ("100 % télétravail",
// "télétravail total", "full remote"). A remote phrase can CONTAIN a hybrid one, and a
// hybrid statement often holds the onsite word ("3 jours en présentiel"), so the order is
// remote, hybrid, onsite.
//
// Every phrase is matched against the folded text with its accents dropped ("télétravail"
// → "teletravail") and its typographic apostrophe straightened, so each is written that way.
//
// Absent on purpose:
//   - bare "télétravail": "outils de télétravail" sells software, and the arrangement is
//     stated by the forms around it.
//   - bare "à distance", "100 % à distance" and "distanciel": "formation à distance" (an
//     apprenticeship's school days, ~900 postings), "une formation flexible, 100 % à
//     distance", "vente à distance", "assistance à distance".
//   - "télétravail : 100": "prime télétravail : 100 € / mois", "télétravail : 100 %
//     présentiel pendant les 3 premiers mois".
//   - bare "présentiel" and "en présentiel": "entretien en présentiel", "formations en
//     présentiel" — the interview and the training, not the job.
//   - bare "hybride", "mode hybride" and "modèle hybride": "véhicules hybrides", "cloud
//     hybride", "formations en mode hybride", "notre modèle hybride — entre courtage B2B2C
//     et distribution directe". Only the work-model forms count.
//   - "poste hybride": "un poste hybride, varié et gratifiant" mixes tasks, not places.
//   - "poste en télétravail": "ce n'est pas un poste en télétravail" puts the negation
//     outside any window.
var frenchWorkModePhrases = []struct {
	mode    string
	phrases []string
}{
	{"remote", []string{
		"100% teletravail", "100 % teletravail", "100% en teletravail", "100 % en teletravail",
		"teletravail a 100",
		"teletravail total", "teletravail complet", "teletravail integral",
		"entierement en teletravail", "totalement en teletravail", "full teletravail",
		"full-remote", "100% remote", "100 % remote", "entierement a distance",
		"totalement a distance",
	}},
	{"hybrid", []string{
		"teletravail partiel", "jours de teletravail", "jour de teletravail",
		"journee de teletravail", "journees de teletravail",
		"teletravail possible", "possibilite de teletravail", "possibilite de teletravailler",
		"teletravail jusqu'a", "teletravail flexible", "teletravail ponctuel",
		"teletravail occasionnel", "teletravail regulier",
		"accord teletravail", "accord de teletravail", "charte teletravail",
		"charte de teletravail", "politique de teletravail", "politique teletravail",
		"subvention teletravail", "indemnite teletravail", "prime teletravail",
		"teletravail : hybride", "teletravail: hybride", "travail hybride",
		"mode de travail hybride", "organisation hybride", "rythme hybride",
		"fonctionnement hybride", "remote friendly", "remote-friendly",
		"jours sur site", "jours de presence", "jours au bureau", "jours en presentiel",
	}},
	{"onsite", []string{
		"100% presentiel", "100 % presentiel", "100% en presentiel", "100 % en presentiel",
		"100% sur site", "100 % sur site", "100% sur place", "100 % sur place",
		"pas de teletravail", "aucun teletravail", "sans teletravail",
		"pas de possibilite de teletravail", "aucune possibilite de teletravail",
		"teletravail : non", "teletravail: non", "teletravail non autorise",
		"teletravail non possible", "teletravail non prevu", "teletravail impossible",
		"pas teletravaillable", "non teletravaillable",
		"poste en presentiel", "lieu du poste : en presentiel",
		"sur site uniquement", "uniquement sur site",
	}},
}

// frenchNegations, found just before a remote or hybrid phrase in its clause, turn it into
// its opposite ("pas de possibilité de télétravail", "sans télétravail régulier").
var frenchNegations = []string{"pas ", "sans ", "aucun ", "aucune ", "ni ", "non "}

// frenchRefusals, found just AFTER a remote or hybrid phrase in its sentence, withdraw it
// ("télétravail possible : non", "100 % remote-friendly" is a culture, not the
// arrangement).
var frenchRefusals = []string{": non", ":non", "non prevu", "non possible", "non autorise",
	"pas possible", "impossible", "-friendly", " friendly"}

// frenchPerks, in the same sentence as a remote or hybrid phrase, make it a BOUNDED
// arrangement rather than the job's ("télétravail pendant les vacances scolaires",
// "en raison du covid-19", "une semaine en full-remote durant les fêtes").
var frenchPerks = []string{"vacances", "covid", "greve", "intemperies", "fetes"}

// frenchNegationWindow reaches over the noun French puts between a negation and the
// phrase: "pas de possibilité de télétravail".
const frenchNegationWindow = 24

var frenchGuards = phraseGuards{negations: frenchNegations, refusals: frenchRefusals,
	perks: frenchPerks, negationWindow: frenchNegationWindow}

// frenchWorkModeTriggers gate the fold, spelled the way both the accented and the bare
// form begin.
var frenchWorkModeTriggers = []string{"teletravail", "télétravail", "remote", "hybride",
	"presentiel", "présentiel", "sur site", "sur place", "distance", "teletravaill",
	"télétravaill"}

// frenchWorkMode derives a work mode from French arrangement prose, "" when none of
// frenchWorkModePhrases is stated. lower is the description already lowercased.
func frenchWorkMode(desc, lower string) string {
	if !containsAny(lower, frenchWorkModeTriggers) {
		return ""
	}
	text := strings.ReplaceAll(dropAccents(foldMarkup(desc)), "’", "'")
	for _, wm := range frenchWorkModePhrases {
		for _, p := range wm.phrases {
			if guardedPhraseMatches(text, p, wm.mode, frenchGuards) {
				return wm.mode
			}
		}
	}
	return ""
}
