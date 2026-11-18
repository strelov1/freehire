package skilltag

import (
	"slices"
	"testing"
)

// Cases reflect the skill combinations observed in the industrial vacancy audit,
// with controls for the software and business meanings of the same acronyms.
func TestParseIndustrialMaintenance(t *testing.T) {
	tests := []struct {
		name, text, category string
		want, absent         []string
	}{
		{"factory maintenance", "Plan preventive and predictive maintenance using SAP PM and CMMS. Track OEE, MTBF and MTTR; follow LOTO procedures and react to faults.", "skilled_trades",
			[]string{"preventive-maintenance", "predictive-maintenance", "sap-pm", "cmms", "overall-equipment-effectiveness", "mtbf", "mttr", "lockout-tagout"}, []string{"react"}},
		{"petroleum plant", "Maintenance planning for rotating equipment; shutdown management and turnaround management. Use IBM Maximo and Failure Mode and Effects Analysis. API, ASME, or equivalent industry certifications.", "skilled_trades",
			[]string{"maintenance-planning", "rotating-equipment", "shutdown-management", "turnaround-management", "maximo", "fmea"}, []string{"api"}},
		{"reliability methods", "Reliability engineering with RCM, TPM and APM. Combine condition monitoring with predictive maintenance.", "industrial_engineering",
			[]string{"reliability-engineering", "reliability-centered-maintenance", "total-productive-maintenance", "asset-performance-management", "condition-monitoring", "predictive-maintenance"}, nil},
		{"lab and construction", "Testing laboratory: ISO/IEC 17025 and equipment calibration; coordinate MEP design coordination and HVAC systems. React promptly to requests.", "",
			[]string{"iso-17025", "equipment-calibration", "mep", "hvac"}, []string{"react"}},
		{"mechanical diagnostics", "Diagnose hydraulic systems and pneumatic systems; implement reliability-centred maintenance.", "",
			[]string{"hydraulics", "pneumatics", "reliability-centered-maintenance"}, nil},
		{"russian maintenance", "Планирование ТОиР, графики планово-предупредительного ремонта и мониторинг технического состояния вращающегося оборудования. Анализ первопричин и анализ видов и последствий отказов.", "skilled_trades",
			[]string{"maintenance-planning", "preventive-maintenance", "condition-monitoring", "rotating-equipment", "root-cause-analysis", "fmea"}, nil},
		{"russian abbreviated maintenance", "ППР оборудования", "manufacturing", []string{"preventive-maintenance"}, nil},
		{"ambiguous abbreviations without category", "TPM RCM APM ППР", "", nil,
			[]string{"total-productive-maintenance", "reliability-centered-maintenance", "asset-performance-management", "preventive-maintenance"}},
		{"software acronyms", "TPM leads the platform team; configure APM with Python and a TPM security module.", "software_engineering", []string{"python"},
			[]string{"total-productive-maintenance", "asset-performance-management"}},
		{"healthcare rcm", "RCM revenue cycle management in a hospital.", "healthcare", nil, []string{"reliability-centered-maintenance"}},
		{"bare ambiguous words", "PM, ТО, calibration of a statistical model, and a server shutdown.", "", nil,
			[]string{"sap-pm", "equipment-calibration", "shutdown-management"}},
		{"petroleum and software in one posting", "API/ASME certification; build a REST API with Python.", "industrial_engineering",
			[]string{"api", "rest", "python"}, nil},
		{"petroleum institute", "American Petroleum Institute (API) standards and FMEA.", "", []string{"fmea"}, []string{"api"}},
		{"petroleum markup", "Meet <b>API</b>/<b>ASME</b> industry requirements.", "", nil, []string{"api"}},
		{"software API standards", "Follow API standards with Python.", "backend", []string{"api", "python"}, nil},
		{"parliamentary MEP", "Assistant to an MEP in the European Parliament.", "administration", nil, []string{"mep"}},
		{"construction MEP", "Oversee MEP and HVAC installations.", "engineering_design", []string{"mep", "hvac"}, nil},
		{"ordinary software reliability", "SRE team applies reliability engineering to Kubernetes services.", "sre", []string{"sre", "kubernetes"}, []string{"reliability-engineering"}},
		{"unknown software reliability", "SRE team applies reliability engineering to Kubernetes services.", "", []string{"sre", "kubernetes"}, []string{"reliability-engineering"}},
		{"site reliability stays software", "Site reliability engineering using Python; track MTTR.", "sre", []string{"python", "mttr"}, []string{"reliability-engineering"}},
		{"mixed reliability disciplines", "Site reliability engineering and equipment reliability engineering.", "industrial_engineering", []string{"reliability-engineering"}, nil},
		{"software API unchanged", "API developer using Python and PostgreSQL.", "backend", []string{"api", "python", "postgresql"}, nil},
		{"certification is not a skill", "CMRP, Certified Maintenance & Reliability Professional", "skilled_trades", nil, []string{"cmrp"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.text, WithAcronymCategory(tt.category))
			for _, want := range tt.want {
				if !slices.Contains(got, want) {
					t.Errorf("Parse() = %v, missing %q", got, want)
				}
			}
			for _, absent := range tt.absent {
				if slices.Contains(got, absent) {
					t.Errorf("Parse() = %v, must not contain %q", got, absent)
				}
			}
		})
	}
}

func TestIndustrialSkillResolution(t *testing.T) {
	tests := map[string]string{
		"oee":                             "overall-equipment-effectiveness",
		"Oee":                             "overall-equipment-effectiveness",
		"TPM":                             "total-productive-maintenance",
		"tpm":                             "total-productive-maintenance",
		"RCM":                             "reliability-centered-maintenance",
		"rcm":                             "reliability-centered-maintenance",
		"APM":                             "asset-performance-management",
		"apm":                             "asset-performance-management",
		"ППР":                             "preventive-maintenance",
		"ппр":                             "preventive-maintenance",
		"SAP Plant Maintenance":           "sap-pm",
		"Reliability-centred maintenance": "reliability-centered-maintenance",
		"Computerised Maintenance Management System": "cmms",
		"Failure Mode and Effects Analysis":          "fmea",
		"ISO/IEC 17025":                              "iso-17025",
		"планово-предупредительный ремонт":           "preventive-maintenance",
		"Анализ первопричин":                         "root-cause-analysis",
		"OEE":                                        "overall-equipment-effectiveness",
	}
	for alias, want := range tests {
		if got, ok := Resolve(alias); !ok || got != want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", alias, got, ok, want)
		}
	}
	for _, acronym := range []string{"TPM", "RCM", "APM", "ППР"} {
		if got := Canonicalize([]string{acronym}, WithResumeAcronyms()); len(got) != 0 {
			t.Errorf("ambiguous resume Canonicalize(%q) = %v, want none", acronym, got)
		}
	}
}
