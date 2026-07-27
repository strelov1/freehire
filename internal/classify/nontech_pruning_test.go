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
