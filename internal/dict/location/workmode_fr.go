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
//   - "télétravail : 100" and "télétravail à 100" without the percent sign: "prime
//     télétravail : 100 € / mois", "télétravail : 100 % présentiel pendant les 3 premiers
//     mois".
//   - "remote-friendly" and "fonctionnement hybride": a culture and a system's design as
//     often as the job's arrangement.
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
		"teletravail a 100%", "teletravail a 100 %",
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
		"jours sur site", "jours de presence", "jours au bureau", "jours en presentiel",
	}},
	{"onsite", []string{
		"100% presentiel", "100 % presentiel", "100% en presentiel", "100 % en presentiel",
		"100% sur site", "100 % sur site", "100% sur place", "100 % sur place",
		"pas de teletravail", "aucun teletravail", "sans teletravail",
		"pas de possibilite de teletravail", "aucune possibilite de teletravail",
		"teletravail : non", "teletravail: non", "teletravail non autorise",
		"teletravail possible : non", "teletravail possible: non",
		"teletravail non possible", "teletravail non prevu", "teletravail impossible",
		"pas teletravaillable", "non teletravaillable",
		"poste en presentiel", "lieu du poste : en presentiel",
		"sur site uniquement", "uniquement sur site",
	}},
}

// frenchOnsiteTotality states the job is wholly on site, and outranks the office days a
// hybrid phrase counts: "évoluerez dans un cadre hybride, mais le poste est 100% sur
// site". It is read after the remote list and before the hybrid one, under the same
// guards as a remote phrase: "pas 100 % sur site" states no totality.
var frenchOnsiteTotality = []string{"100% sur site", "100 % sur site", "sur site uniquement",
	"uniquement sur site", "100% en presentiel", "100 % en presentiel", "100% presentiel",
	"100 % presentiel", "100% sur place", "100 % sur place"}

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

// frenchWorkModeTriggers gate the phrase walk, spelled without accents: they are read on
// the folded text.
var frenchWorkModeTriggers = []string{"teletravail", "remote", "hybride", "presentiel",
	"sur site", "sur place", "distance"}

// frenchWorkMode derives a work mode from French arrangement prose, "" when none of
// frenchWorkModePhrases is stated. lower is the description already lowercased.
//
// The triggers are checked on the decoded text: a description may spell its accents as
// HTML entities ("t&eacute;l&eacute;travail").
func frenchWorkMode(desc string) string {
	text := strings.ReplaceAll(dropAccents(foldMarkup(desc)), "’", "'")
	if !containsAny(text, frenchWorkModeTriggers) {
		return ""
	}
	for _, wm := range frenchWorkModePhrases {
		if wm.mode == "hybrid" {
			for _, p := range frenchOnsiteTotality {
				if guardedPhraseMatches(text, p, "totality", frenchGuards) && !frenchOnsiteBounded(text, p) {
					return "onsite"
				}
			}
		}
		for _, p := range wm.phrases {
			if guardedPhraseMatches(text, p, wm.mode, frenchGuards) && (wm.mode != "onsite" || !frenchOnsiteBounded(text, p)) {
				return wm.mode
			}
		}
	}
	return ""
}

// frenchOnsiteBounds mark an onsite statement that holds only for a period: "pas de
// télétravail pendant la période de formation", "100 % présentiel les 3 premiers mois".
var frenchOnsiteBounds = []string{"periode d'essai", "periode de formation", "periode d'integration",
	"premiers mois", "premieres semaines", "pendant la formation", "durant la formation"}

// frenchOnsiteBounded reports whether every occurrence of the onsite phrase p sits in a
// sentence that bounds it to a period, so the posting states no lasting arrangement.
func frenchOnsiteBounded(text, p string) bool {
	found := false
	for from := 0; ; {
		i := strings.Index(text[from:], p)
		if i < 0 {
			return found
		}
		start, end := from+i, from+i+len(p)
		from = end
		if !containsAny(sentenceAround(text, start, end), frenchOnsiteBounds) {
			return false
		}
		found = true
	}
}
