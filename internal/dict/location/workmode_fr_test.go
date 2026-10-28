package location

import "testing"

// TestFrenchWorkModeFromDescription pins the French arrangement phrases. French lists home
// working as a benefit beside an office, which is hybrid; remote needs a stated totality.
// Every case is a sentence from a live French posting (wttj, flatchr, teamtailor,
// smartrecruiters, recruitee, lever, join, eures), measured 2026-10-28.
func TestFrenchWorkModeFromDescription(t *testing.T) {
	cases := []struct {
		desc, want string
	}{
		// Remote: a stated totality.
		{"Nous vous offrons : 100 % télétravail, 35h par semaine.", "remote"},
		{"Poste en 100% télétravail. Participer à l'évolution d'une nouvelle startup.", "remote"},
		{"Vous serez en télétravail total avec autonomie.", "remote"},
		{"Ce poste est entièrement en télétravail et rattaché au Head of Engineering.", "remote"},
		{"Le poste est basé en télétravail à 100 %, avec un contexte de production actif.", "remote"},
		{"Modalités : - Lieu : full-remote - Date : à partir du 4 novembre", "remote"},
		// Hybrid: home working beside an office.
		{"Un poste basé à Morges avec télétravail partiel possible.", "hybrid"},
		{"Nous proposons 2 jours de télétravail hebdomadaire.", "hybrid"},
		{"Un accord télétravail pour télétravailler jusqu’à 2 jours par semaine.", "hybrid"},
		{"Contrat : CDI Télétravail : Hybride Salaire : de 40 k€ à 45 k€", "hybrid"},
		{"Avantages : carte Swile, Gymlib, subvention télétravail, espaces de coworking.", "hybrid"},
		{"Le poste est hybride (3 jours sur site par semaine).", "hybrid"},
		{"Une organisation du travail modulable : télétravail jusqu'à 2 jours par semaine.", "hybrid"},
		// Onsite: a stated refusal or a stated totality on site.
		{"Poste en CDI 35h. Pas de télétravail. Salaire 30/35k€.", "onsite"},
		{"Lieu : Lavaur Contrat : CDI Télétravail : non Salaire : de 22 k€ à 25 k€", "onsite"},
		{"Poste basé à Hennebont (56) - 100% en présentiel - à pourvoir dès que possible.", "onsite"},
		{"Lieu du poste : En présentiel", "onsite"},
		{"Le poste n'est pas télétravaillable.", "onsite"},
		// A negation or a refusal withdraws a remote or hybrid phrase.
		{"Il n'y a pas de possibilité de télétravail sur ce poste.", "onsite"},
		{"Télétravail possible : non.", ""},
		// A bounded period is not the arrangement.
		{"Possibilité de semaine en full-remote durant les fêtes de fin d'année.", ""},
		// The training, the interview, the product and the business model are not the job.
		{"3 jours en entreprise et 1 jour en formation à distance.", ""},
		{"Une formation flexible, 100 % à distance, sans frais.", ""},
		{"Un entretien en présentiel ou en visioconférence est prévu.", ""},
		{"Nous concevons les véhicules électriques et hybrides de demain.", ""},
		{"Notre modèle hybride — entre courtage B2B2C et distribution directe.", ""},
		{"Prime télétravail : 100 € / mois.", "hybrid"},
	}
	for _, c := range cases {
		if got := WorkModeFromDescription(c.desc); got != c.want {
			t.Errorf("WorkModeFromDescription(%q) = %q, want %q", c.desc, got, c.want)
		}
	}
}
