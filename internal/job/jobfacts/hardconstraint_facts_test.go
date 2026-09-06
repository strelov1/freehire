package jobfacts

import "testing"

func TestRequiredCertifications(t *testing.T) {
	// The clause a credential sits in decides whether the posting REQUIRES it. Both
	// credentials are named here; only one is a requirement, and reading the other as
	// one caps a candidate's fit score at 60 for a certification nobody demanded.
	got := RequiredCertifications("Requires an active CISSP; PMP preferred.")
	if len(got) != 1 || got[0] != "cissp" {
		t.Errorf("RequiredCertifications = %v, want [cissp] only", got)
	}
	got = RequiredCertifications(`<h3>Requirements</h3><ul><li>CISSP</li></ul>` +
		`<h3>Nice to have</h3><ul><li>PMP</li></ul>`)
	if len(got) != 1 || got[0] != "cissp" {
		t.Errorf("RequiredCertifications(preferred section) = %v, want [cissp] only", got)
	}
	if len(RequiredCertifications("Backend role, Go and Postgres.")) != 0 {
		t.Error("expected no certifications for a plain description")
	}
	// A comma is a clause break, and blanking a clause must not cost the clauses
	// around it their own credentials.
	got = RequiredCertifications("CISSP, CISA and CISM required, PMP preferred.")
	if len(got) != 3 || got[0] != "cissp" || got[1] != "cisa" || got[2] != "cism" {
		t.Errorf("RequiredCertifications = %v, want [cissp cisa cism]", got)
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
