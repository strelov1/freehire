package skilltag

import (
	"slices"
	"testing"
)

// TestParse_DutchMarket covers the 2026-10-25 batch mined from the open Dutch postings
// (eures, recruitee, whatjobs-nl): the LLM skills the dictionary missed, counted in the
// descriptions. Every positive is a real clause, lightly trimmed.
func TestParse_DutchMarket(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		// Low-code platforms and the data stack.
		{"mendix", "Senior Mendix Developer met ervaring in Mendix 10", []string{"mendix"}, nil},
		{"outsystems thinkwise", "Kennis van OutSystems of Thinkwise is een pré", []string{"outsystems", "thinkwise"}, nil},
		{"pega", "Je bouwt cases op het Pega Platform als Pega developer", []string{"pega"}, nil},
		{"bare pega is a verb", "El equipo pega carteles en la tienda.", nil, []string{"pega"}},
		{"data stack", "Ervaring met Matillion, Data Vault 2.0 en Tabular Editor",
			[]string{"matillion", "data-vault", "tabular-editor"}, nil},
		{"onelake is fabric", "Je bouwt lakehouses in OneLake", []string{"microsoft-fabric"}, nil},
		{"business central", "Business Central consultant met kennis van AL", []string{"microsoft-dynamics"}, nil},
		{"integration", "Integraties met Apache Camel en Auth0", []string{"apache-camel", "auth0"}, nil},
		{"tekton", "CI/CD met Tekton pipelines op OpenShift", []string{"tekton"}, nil},
		{"bare tekton is a tool brand", "Gereedschapsset van Tekton voor de monteur.", nil, []string{"tekton"}},
		{"firestore", "Data opslaan in Firestore", []string{"firestore"}, nil},
		// Architecture and the service desk.
		{"architecture", "Je modelleert in ArchiMate en kent TOGAF", []string{"archimate", "togaf"}, nil},
		{"topdesk", "Meldingen afhandelen in TOPdesk", []string{"topdesk"}, nil},

		// A clerk's suites and the regimes tag, and never vouch for a gated word.
		{"afas", "Functioneel beheerder AFAS Profit", []string{"afas"}, nil},
		{"clerk suites", "Salarisadministratie in Nmbrs, Youforce en Exact Online",
			[]string{"nmbrs", "youforce", "exact-online"}, nil},
		{"afas does not vouch", "Salarisadministrateur met AFAS; je archiveert de dossiers in de cloud.",
			[]string{"afas"}, []string{"cloud"}},
		{"regimes", "Je borgt NIS2 en NEN 7510 binnen het ziekenhuis", []string{"nis2", "nen-7510"}, nil},
		{"hastus", "Dienstroosters maken in HASTUS", []string{"hastus"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in)
			for _, w := range c.want {
				if !slices.Contains(got, w) {
					t.Errorf("Parse(%q) = %v, missing %q", c.in, got, w)
				}
			}
			for _, a := range c.absent {
				if slices.Contains(got, a) {
					t.Errorf("Parse(%q) = %v, must NOT contain %q", c.in, got, a)
				}
			}
		})
	}
}
