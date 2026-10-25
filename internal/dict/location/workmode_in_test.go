package location

import "testing"

// The Indian work-arrangement register. Every description line is a real one from the
// Indian part of the catalogue (sampled 2026-10-25) unless the case says otherwise.
func TestWorkModeFromIndianDescription(t *testing.T) {
	tests := []struct {
		name, desc, want string
	}{
		{"WFO in the location line", "Location - Bangalore (WFO) Experience – 6-9 years", "onsite"},
		{"work mode work from office", "<p>Work Mode: Work From Office (Monday- Friday)</p>", "onsite"},
		{"work type WFO", "Work Type: WFO Interview Rounds: 2", "onsite"},
		{"five days WFO", "Location: HSR Layout, Bengaluru WFO - 5 days", "onsite"},
		{"5 days work from office", "This role requires 5 days work from office.", "onsite"},
		{"hybrid days office", "Hybrid (3 days office)", "hybrid"},
		{"days WFO", "Hybrid - 3 days WFO, Pune", "hybrid"},
		{"days a week from office", "You will work 2 days a week from office.", "hybrid"},
		{"days work from office", "Hybrid 3 days work from office", "hybrid"},
		{"days WFH", "3 days WFO and 2 days WFH", "hybrid"},
		{"office and home", "Flexible working hours and possibility to combine work from office and home (hybrid ways of working).", "hybrid"},
		{"WFO days a month", "Work mode – Hybrid (10days WFO in a month)", "hybrid"},
		{"WFH role", "This is a permanent WFH role.", "remote"},
		{"WFH remote", "Job Location: WFH / Remote Experience: 8-10 years", "remote"},
		{"location WFH", "Location: WFH (ready to travel to the installation site)", "remote"},
		{"work mode WFH", "Work mode: Work from home", "remote"},

		// A benefit or an HR duty is not the arrangement.
		{"occasional WFH", "Office based culture with occasional WFH that provides flexibility", ""},
		{"rotational WFH shift", "Monday–Friday in-office and a rotational WFH shift on Saturday", ""},
		{"WFH days perk", "Flexible policy: up to 12 WFH days every 6 months.", ""},
		{"managing WFH staff", "Monitor and manage remote/WFH employees through tools.", ""},
		// A five-day WEEK is not an office statement.
		{"five days working", "5 days working, Monday to Friday", ""},
		// Days a MONTH in the office is not a week's pattern (synthetic).
		{"days a month", "5 days in office per month", "hybrid"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := WorkModeFromDescription(tc.desc); got != tc.want {
				t.Errorf("WorkModeFromDescription(%q) = %q, want %q", tc.desc, got, tc.want)
			}
		})
	}
}

func TestParseIndianWorkModeMarkers(t *testing.T) {
	for loc, want := range map[string]string{
		"Bangalore (WFO)":            "onsite",
		"Chennai - Work From Office": "onsite",
		"Hybrid (3 days WFO), Pune":  "hybrid",
		"WFH":                        "remote",
	} {
		if got := Parse(loc).WorkMode; got != want {
			t.Errorf("Parse(%q).WorkMode = %q, want %q", loc, got, want)
		}
	}
}
