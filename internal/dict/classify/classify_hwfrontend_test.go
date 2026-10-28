package classify

import "testing"

// In chip and radio design the "front end" is a stage of the hardware, not a web UI.
// A qualifier beside the word says which one it is; a title with neither keeps the
// web reading. Every title below is an open posting measured on prod (2026-10-28).
func TestParse_HardwareFrontEnd(t *testing.T) {
	cases := []struct{ title, want string }{
		{"Senior RF Front-End Design Engineer", "hardware"},
		{"Senior RF Front-End Design Engineer - Satellite User Terminals", "hardware"},
		{"ASIC Frontend Design Engineer", "hardware"},
		{"RTL Design Lead/IC Front-End Architect/Datapath Lead", "hardware"},
		{"ASIC front-end verification engineer", "hardware"},
		{"Senior ASIC (Front-End) Design Engineer", "hardware"},
		{"Sr. Staff Engineer, CPU (Front-End) Verification", "hardware"},
		{"Senior RTL Design Engineer - CPU Frontend", "hardware"},
		{"Graduate Talent (ISCP SoC Front End Team)", "hardware"},
		{"Staff Engineer, GPU Front-End Infrastructure/ Methodology", "hardware"},
		{"VLSI Frontend CAD Applications Engineer, Senior", "hardware"},
		{"Senior Engineer CAD Pre-Silicon Front End", "hardware"},
		{"Neuromorphic Front-End Silicon Lead", "hardware"},
		{"Process Engineer (Semiconductor Front-end / MNC)", "hardware"},
		{"Principal Research Engineer - Front-End Semiconductor Equipment Engineering", "hardware"},
		{"Lead FrontEnd Methodology Engineer", "hardware"},
		{"Front End Design Verification Engineer", "hardware"},
		{"Front-end Design/Verification Engineer【1 year contractor】", "hardware"},
		{"Front End RTL Designer / SoC integration", "hardware"},
		{"Research Fellow/Senior Research Fellow (Front-end design (RTL) Scientist for Heterogeneous Systems)", "hardware"},
		{"AI Developer Intern — Interconnect Hardware Frontend, 2027", "hardware"},

		// The web reading stays: "IC" is an individual contributor, "RTL" is
		// right-to-left text, "digital" is a digital product, and a bare
		// "front-end design engineer" is as often a UI designer as a chip designer.
		{"Sr Software Engineer Frontend IC", "frontend"},
		{"Front-end Engineer 4 (Manager, IC)", "frontend"},
		{"Frontend Developer (React/TypeScript) — RTL UI in Riyadh", "frontend"},
		{"React / Next.js - Digital Frontend Developer", "frontend"},
		{"Senior Frontend Design Engineer — React/TypeScript UI Architect", "frontend"},
		{"Front-End Design Engineer - UX/UI for Web Apps", "frontend"},
		{"Senior Front End Developer", "frontend"},
	}
	for _, tc := range cases {
		if got := Parse(tc.title).Category; got != tc.want {
			t.Errorf("Parse(%q).Category = %q, want %q", tc.title, got, tc.want)
		}
	}
}
