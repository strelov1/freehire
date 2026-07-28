package classify

import "testing"

// The terms added for the catalogue-pruning campaign. These are not merely labels: the
// prune worker deletes on them permanently, so each carries a negative case naming a
// real technical title that must not match. The negatives are chosen to share a word
// with the term — a term is dangerous exactly where it nearly collides.
func TestPruningTermsMatchTheirCluster(t *testing.T) {
	positive := []string{
		"Registered Behavior Technician (RBT)",
		"Behavior Technician - No Experience Required",
		"Pathway to RBT, In Home and Clinic",
		"Maintenance Technician II",
		"Behavioral Health Associate",
		"Speech Language Pathologist - Schools",
		"Certified Occupational Therapy Assistant",
		"Personal Care Aide - On Call",
		"Care Assistant, Nights",
		"Licensed Clinical Social Worker",
		"Machine Operator - 2nd Shift",
		"Crew Member (Full Time)",
		"Car Rental Driver at the Airport",
	}
	for _, title := range positive {
		if !IsNonTech(title) {
			t.Errorf("IsNonTech(%q) = false, want true", title)
		}
	}
}

func TestPruningTermsDoNotMatchTechnicalTitles(t *testing.T) {
	// Each of these shares a word with one of the added terms. If a term ever widens,
	// this is where it shows.
	negative := []string{
		"Machine Learning Engineer",              // machine operator
		"Senior NLP / Natural Language Engineer", // language pathologist
		"Behavior-Driven Development Engineer",   // behavior technician
		"Software Maintenance Engineer",          // maintenance technician
		"Healthcare Platform Engineer",           // care aide / care assistant
		"Social Media Data Engineer",             // social worker
		"Site Reliability Engineer",              // (control)
		"Backend Engineer, Health Records",       // behavioral health
		"Mobile Developer - Car Sharing",         // car rental
		"Crew Scheduling Systems Engineer",       // crew member
	}
	for _, title := range negative {
		if IsNonTech(title) {
			t.Errorf("IsNonTech(%q) = true, want false — a technical title must never match a pruning term", title)
		}
	}
}

// The second mining wave. Same contract as the first: every term deletes permanently,
// so each family carries a real title it must place and a real technical title that
// shares a word with it and must survive.
func TestSecondWaveTermsMatchTheirCluster(t *testing.T) {
	positive := []string{
		"Electrical Engineer - Substation Design",
		"Senior Mechanical Engineer (HVAC)",
		"Civil Engineer, Roads and Drainage",
		"Structural Engineer - Bridges",
		"Quantity Surveyor - Commercial",
		"Mental Health Technician - Night Shift",
		"Clinical Research Coordinator II",
		"Physician Assistant - Family Medicine",
		"Primary Care Provider (APP)",
		"MRI Technologist PRN",
		"Care Coordinator, Community Health",
		"Sous Chef - Fine Dining",
		"Chef de Partie (Pastry)",
		"Room Attendant - Housekeeping",
		"Automotive Technician / Mechanic",
		"Express Lube Technician",
		"Heavy Equipment Operator - Class A",
		"Production Supervisor, 2nd Shift",
		"Personal Banker - Downtown Branch",
		"Licensed Insurance Agent",
		"Retail Keyholder (Part Time)",
		"Store Mgr - Store 4471",
		"Asst Store Mgr",
		"Adjunct Faculty - Nursing",
		"Lab Technician, Microbiology",
		"Auxiliar Administrativo",
		"Специалист по охране труда",
	}
	for _, title := range positive {
		if !IsNonTech(title) {
			t.Errorf("IsNonTech(%q) = false, want true", title)
		}
	}
}

func TestSecondWaveTermsDoNotMatchTechnicalTitles(t *testing.T) {
	// Each shares a word with a second-wave term. These are the titles the volume
	// ranking would have swept up had the clusters been imported as reported.
	negative := []string{
		"Manufacturing Execution Systems Engineer", // manufacturing engineer
		"Electrical Grid Data Platform Engineer",   // electrical engineer
		"Mechanical Design Automation Developer",   // mechanical engineer
		"Structural Analysis Software Developer",   // structural engineer
		"Backend Engineer - Mental Health Startup", // mental health technician
		"Clinical Research Data Platform Engineer", // clinical research coordinator
		"Site Reliability Engineer, Production",    // production supervisor/operator
		"Production Engineer (Platform)",           // production supervisor/operator
		"Equipment Telemetry Software Engineer",    // equipment operator
		"App Store Release Engineer",               // store manager
		"Data Engineer - Retail Analytics",         // retail keyholder
		"Machine Learning Engineer, Primary Care",  // primary care physician/provider
		"Full Stack Developer - Guest Experience",  // guest service
		"Automotive Embedded Software Engineer",    // automotive technician
		"Laboratory Information Systems Developer", // lab technician
		"Медицинский информационный аналитик",      // охране труда (control, Cyrillic)
	}
	for _, title := range negative {
		if IsNonTech(title) {
			t.Errorf("IsNonTech(%q) = true, want false — a technical title must never match a pruning term", title)
		}
	}
}
