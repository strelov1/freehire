package classify

import "testing"

// TestIsNonTech_ExpandedClusters locks the widened non-tech coverage across the
// clusters the prod unknown bucket is dominated by, plus the tech-collision traps
// the expansion must never flip.
func TestIsNonTech_ExpandedClusters(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  bool
	}{
		// Healthcare
		{"cna", "Certified Nursing Assistant (CNA)", true},
		{"pharmacy technician", "Pharmacy Technician - Night", true},
		{"phlebotomy", "Phlebotomy Technician", true},
		{"respiratory therapist", "Registered Respiratory Therapist", true},
		{"medical scribe", "Medical Scribe", true},
		// Food service
		{"cook", "Cook", true},
		{"food service", "Food Service Worker", true},
		// Retail
		{"retail associate", "Retail Sales Associate", true},
		{"sales clerk", "Sales Clerk", true},
		// Warehouse / logistics
		{"warehouse associate", "Warehouse Associate", true},
		{"order picker", "Order Picker / Packer", true},
		{"material handler", "Material Handler II", true},
		{"cdl driver", "CDL Driver - Local", true},
		// Trades
		{"ironworker", "Reinforcing Ironworker", true},
		{"laborer", "General Laborer", true},
		{"pipefitter", "Pipefitter", true},
		{"hvac technician", "HVAC Technician", true},
		// Office / finance
		{"paralegal", "Paralegal - EL/PL", true},
		{"bookkeeper", "Full Charge Bookkeeper", true},
		{"accountant", "Staff Accountant", true},
		{"payroll clerk", "Payroll Clerk", true},
		{"teller", "Bank Teller", true},
		// Education
		{"substitute teacher", "Substitute Teacher", true}, // also via "teacher"
		{"teaching assistant", "Teaching Assistant", true},
		{"childcare", "Childcare Provider", true},

		// Trap negatives — tech or tech-adjacent that MUST stay unflagged.
		{"it technician", "IT Technician", false},
		{"field service technician", "Field Service Technician", false},
		{"data warehouse engineer", "Data Warehouse Engineer", false},
		{"security engineer", "Security Engineer", false},
		{"systems coordinator", "Systems Coordinator", false},
		{"data analyst", "Data Analyst", false},
		{"software engineer", "Software Engineer II", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNonTech(tt.title); got != tt.want {
				t.Errorf("IsNonTech(%q) = %v, want %v", tt.title, got, tt.want)
			}
		})
	}
}
