package skilltag

import (
	"slices"
	"testing"
)

// TestParse_GermanMarket covers the 2026-10-25 batch mined from the German ATS crawl
// (coveto, rexx, dvinci, xing, concludis, onlyfy, kenjo). That crawl is not enriched, so
// the batch came from the requirement clauses themselves ("Kenntnisse in …", "Erfahrung
// mit …") rather than from LLM output. Every positive is a real clause, lightly trimmed.
func TestParse_GermanMarket(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		// SAP: the bare word stays gated (tree sap), but German binds it to a requirement.
		{"sap-kenntnisse", "Gute MS-Office- und SAP-Kenntnisse", []string{"sap"}, nil},
		{"kenntnisse in sap", "Idealerweise Kenntnisse in SAP", []string{"sap"}, nil},
		{"sap-systemen", "Erfahrung im Umgang mit SAP-Systemen", []string{"sap"}, nil},
		{"sap modules", "Fundierte Kenntnisse in SAP FI/CO", []string{"sap-fico"}, nil},
		{"sap r/3", "Anwenderkenntnisse in SAP R/3", []string{"sap"}, nil},
		{"sap fi/co does not vouch", "Buchhalter mit SAP FI/CO; Montage (assembly) der Belegordner.",
			[]string{"sap-fico"}, []string{"assembly"}},
		{"bare sap still gated", "Clear vegetation and wash tree sap off the guard rails.", nil, []string{"sap"}},
		// A German SAP clause names a tool an office clerk uses; it must not vouch.
		{"sap does not vouch", "Sachbearbeiter Einkauf mit SAP-Kenntnissen; Montage (assembly) im Lager.",
			[]string{"sap"}, []string{"assembly"}},

		// Industrial automation: SPS is German for PLC.
		{"sps-programmierung", "Erfahrung in der SPS-Programmierung (Siemens S7, TIA Portal)", []string{"plc"}, nil},
		{"tia portal alone", "Kenntnisse im TIA Portal", []string{"plc"}, nil},
		{"simatic", "Inbetriebnahme von SIMATIC-Steuerungen", []string{"plc"}, nil},
		{"wincc", "Visualisierung mit WinCC", []string{"scada"}, nil},
		{"sinumerik", "Programmierung von Sinumerik-Steuerungen", []string{"cnc"}, nil},
		{"heidenhain steuerung", "Erfahrung mit Heidenhain-Steuerungen", []string{"cnc"}, nil},
		{"heidenhain the employer", "Die Dr. Johannes Heidenhain GmbH sucht eine Buchhaltung.", nil, []string{"cnc"}},
		{"fieldbuses", "Kenntnisse in PROFIBUS, EtherCAT und CANopen", []string{"profibus", "ethercat", "canopen"}, nil},
		{"knx", "Programmierung von KNX-Anlagen", []string{"knx"}, nil},
		{"eplan", "Erstellung von Stromlaufplänen in EPLAN P8", []string{"eplan"}, nil},
		{"eplan inside a word", "Erstellung der Speisepläne und Dienstpläne", nil, []string{"eplan"}},

		// Construction and architecture software.
		{"itwo", "Kalkulation mit RIB iTWO", []string{"itwo"}, nil},
		{"itwo does not vouch", "iTWO-Kenntnisse von Vorteil. Bitte richten Sie Ihre Bewerbung an Frau Julia Radtke.",
			[]string{"itwo"}, []string{"julia"}},
		{"archicad allplan", "Sicherer Umgang mit ArchiCAD oder Allplan", []string{"archicad", "allplan"}, nil},

		// Accounting software: a tool, and the clerk's, so it does not vouch.
		{"datev", "Sicherer Umgang mit DATEV Lohn und Gehalt", []string{"datev"}, nil},
		{"lodas", "Erfahrung mit LODAS", []string{"datev"}, nil},
		{"datev does not vouch", "Steuerfachangestellte mit DATEV; Belege für die Cloud-Ablage scannen.",
			[]string{"datev"}, []string{"cloud"}},

		// ERP/CRM in their German plural forms; the English singular stays gated.
		{"erp-systemen", "Erfahrung mit ERP-Systemen", []string{"erp"}, nil},
		{"crm-systeme", "Pflege der CRM-Systeme", []string{"crm"}, nil},
		{"english crm stays gated", "Account executive keeping our CRM up to date.", nil, []string{"crm"}},

		// Left out on purpose by the dictionary's own rule: office suites, welding and
		// German regulations are not technology skills.
		{"ms office", "Gute MS-Office-Kenntnisse (Excel, Word, PowerPoint)", nil, []string{"excel", "microsoft-office"}},
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
