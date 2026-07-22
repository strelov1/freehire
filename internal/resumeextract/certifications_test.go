package resumeextract

import "testing"

func TestSanitizeCertifications(t *testing.T) {
	s := Structured{Certifications: []string{"  AWS Certified Solutions Architect  ", "", "   ", "PMP"}}
	s.Sanitize()
	if len(s.Certifications) != 2 || s.Certifications[0] != "AWS Certified Solutions Architect" || s.Certifications[1] != "PMP" {
		t.Errorf("Certifications = %#v, want trimmed [AWS..., PMP] with blanks dropped", s.Certifications)
	}
}

func TestSanitizeCertificationsCapsCount(t *testing.T) {
	many := make([]string, 100)
	for i := range many {
		many[i] = "cert"
	}
	s := Structured{Certifications: many}
	s.Sanitize()
	if len(s.Certifications) > maxCertifications {
		t.Errorf("Certifications count = %d, want <= %d", len(s.Certifications), maxCertifications)
	}
}
