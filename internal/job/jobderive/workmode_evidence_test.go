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
