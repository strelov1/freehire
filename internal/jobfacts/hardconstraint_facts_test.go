package jobfacts

import "testing"

func TestRequiredCertifications(t *testing.T) {
	got := RequiredCertifications("Requires an active CISSP; PMP preferred.")
	found := map[string]bool{}
	for _, s := range got {
		found[s] = true
	}
	if !found["cissp"] || !found["pmp"] {
		t.Errorf("RequiredCertifications = %v, want cissp and pmp", got)
	}
	if len(RequiredCertifications("Backend role, Go and Postgres.")) != 0 {
		t.Error("expected no certifications for a plain description")
	}
}

func TestDegreeOptional(t *testing.T) {
	optional := []string{
		"Bachelor's degree or equivalent experience",
		"BS in CS or equivalent work experience",
		"Degree or equivalent",
	}
	for _, d := range optional {
		if !DegreeOptional(d) {
			t.Errorf("DegreeOptional(%q) = false, want true", d)
		}
	}
	hard := []string{
		"Bachelor's degree required",
		"Must have a Master's degree in Computer Science",
	}
	for _, d := range hard {
		if DegreeOptional(d) {
			t.Errorf("DegreeOptional(%q) = true, want false", d)
		}
	}
}
