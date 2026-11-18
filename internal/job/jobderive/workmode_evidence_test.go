package jobderive

import (
	"os"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/htmltext"
)

func TestDerivePreservesRemoteOnCorporateOrConditionalHybridEvidence(t *testing.T) {
	for name, id := range map[string]string{
		"EPAM remote role with monthly client travel":    "176460331",
		"EPAM corporate benefits only":                   "1611723000",
		"Habr office or conditional hybrid":              "1641840178",
		"SITA finance corporate flex benefits":           "1644452533",
		"SITA incident response corporate flex benefits": "1644452984",
		"Viterbit possibility of hybrid telework":        "1650956252",
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/workmode/" + id + ".html")
			if err != nil {
				t.Fatal(err)
			}
			got := Derive(Input{Title: "Developer", WorkMode: "remote", Description: string(raw)})
			if got.WorkMode != "remote" {
				t.Errorf("WorkMode = %q, want preserved remote", got.WorkMode)
			}
		})
	}
}

func TestDeriveMandatoryOfficeWeekWithoutCorporateHybridBenefits(t *testing.T) {
	raw, err := os.ReadFile("testdata/workmode/1602002587.html")
	if err != nil {
		t.Fatal(err)
	}
	description := strings.Join(strings.Fields(htmltext.ToText(string(raw))), " ")
	description = strings.ReplaceAll(description, "Hybrid by design mode and opportunity to work remotely within Poland", "")
	if strings.Contains(description, "Hybrid by design") {
		t.Fatal("corporate hybrid benefit remains in fixture")
	}
	for _, signal := range []string{"remote", ""} {
		got := Derive(Input{Title: "Lead .NET Engineer", WorkMode: signal, Description: description})
		if got.WorkMode != "hybrid" {
			t.Errorf("WorkMode = %q with signal %q, want hybrid from mandatory office attendance", got.WorkMode, signal)
		}
	}
}

func TestDerive_HybridArrangementContradictsStructuredRemote(t *testing.T) {
	for _, description := range []string{
		"Ubicación: Barcelona (<strong>Modelo Híbrido</strong>)",
		"Full time or Part time with hybrid working",
		"This is not a fully remote role; this is a hybrid position",
		"This role is not fully remote, but is a hybrid role.",
		"Fully remote work is not offered. This is a hybrid role.",
		"Hybrid session environment (in-person and telehealth). Flexible work from home options available.",
		"This hybrid opportunity combines in-person relationship building with work from your home office.",
		"This role offers work from home regularly but must be able to commute to Emory University on a flexible weekly schedule.",
		"Location: Dallas, TX / Charlotte, NC/ Iselin, NJ, / Chandler, AZ / Ohio, Delaware (Hybrid)",
		"Title: Senior Software Developer. Charlotte/ Phoenix (Hybrid)",
		"Job Location: Mumbai (Hybrid)",
		"Location: Houston, TX (Hybrid)",
		"Location: California (Hybrid)",
		"This is hybrid work with two days in the office.",
		"You must work in the office three days per week.",
		"This remote role requires a hybrid schedule with two mandatory office days per week.",
		"This position is remote, with a mandatory hybrid schedule.",
		"Hybrid role with optional dental insurance.",
		"Prior experience required\nLocation: Mumbai (Hybrid)",
	} {
		for _, signal := range []string{"remote", ""} {
			got := Derive(Input{Title: "Developer", WorkMode: signal, Description: description})
			if got.WorkMode != "hybrid" {
				t.Errorf("WorkMode = %q for %q with signal %q, want hybrid", got.WorkMode, description, signal)
			}
		}
	}
}

func TestDerive_RemotePreservedForIncidentalHybridMentions(t *testing.T) {
	for _, description := range []string{
		"This role is not fully remote, but is a hybrid role for 30 days, then fully remote.",
		"Experience with hybrid cloud infrastructure.",
		"Based in Berlin, maintain infrastructure (Hybrid).",
		"Location: Berlin, maintain infrastructure (hybrid).",
		"Location: Berlin / prior experience in infrastructure (hybrid).",
		"We do not offer hybrid working; this position is fully remote.",
		"This is a remote position. Hybrid working is optional.",
		"Hybrid working is optional.",
		"Optional hybrid work arrangement is available.",
		"Location: Mumbai (Hybrid) is optional.",
		"You are not required to work in the office three days per week.",
		"You must work in the office three days per week if you choose the optional hybrid arrangement.",
		"Hybrid work with two days in the office is optional.",
		"A hybrid role is optional.",
		"We never offer hybrid working.",
		"Prior experience managing office-based but hybrid working teams is required.",
		"Hybrid working is not possible; this position is remote.",
		"Hybrid working is not an option; this position is remote.",
		"Hybrid working is entirely optional.",
		"Hybrid working is completely optional.",
		"Experience working in a hybrid working environment is preferred. This role is fully remote.",
		"We support teams with a hybrid model. This role is fully remote.",
		"Modalidad 100% remota; ayuda al teletrabajo.",
	} {
		got := Derive(Input{Title: "Developer", WorkMode: "remote", Description: description})
		if got.WorkMode != "remote" {
			t.Errorf("WorkMode = %q for %q, want remote", got.WorkMode, description)
		}
	}
}
