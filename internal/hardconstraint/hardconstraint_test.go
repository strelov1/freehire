package hardconstraint

import "testing"

func intp(v int) *int    { return &v }
func boolp(v bool) *bool { return &v }

// find returns the first blocker of a category, or ok=false if none was emitted
// (i.e. the category was skipped).
func find(bs []Blocker, cat BlockerCategory) (Blocker, bool) {
	for _, b := range bs {
		if b.Category == cat {
			return b, true
		}
	}
	return Blocker{}, false
}

func TestExperienceCategory(t *testing.T) {
	t.Run("unmet is a blocker naming both numbers", func(t *testing.T) {
		bs := Evaluate(JobRequirements{ExperienceYearsMin: intp(5)}, CVEvidence{TotalYears: 3})
		b, ok := find(bs, CategoryExperience)
		if !ok || b.Met {
			t.Fatalf("want unmet experience blocker, got %+v ok=%v", b, ok)
		}
		if b.ScoreCap != 65 {
			t.Errorf("experience score cap = %d, want 65", b.ScoreCap)
		}
	})
	t.Run("met is satisfied", func(t *testing.T) {
		bs := Evaluate(JobRequirements{ExperienceYearsMin: intp(5)}, CVEvidence{TotalYears: 6})
		if b, ok := find(bs, CategoryExperience); !ok || !b.Met {
			t.Fatalf("want met experience, got %+v ok=%v", b, ok)
		}
	})
	t.Run("no CV years is skipped", func(t *testing.T) {
		bs := Evaluate(JobRequirements{ExperienceYearsMin: intp(5)}, CVEvidence{TotalYears: 0})
		if _, ok := find(bs, CategoryExperience); ok {
			t.Error("experience should be skipped when CV years unknown")
		}
	})
	t.Run("no job requirement is skipped", func(t *testing.T) {
		bs := Evaluate(JobRequirements{}, CVEvidence{TotalYears: 3})
		if _, ok := find(bs, CategoryExperience); ok {
			t.Error("experience should be skipped when the job carries no minimum")
		}
	})
}

func TestEducationCategory(t *testing.T) {
	t.Run("higher degree satisfies lower requirement", func(t *testing.T) {
		bs := Evaluate(JobRequirements{EducationLevel: "bachelor"}, CVEvidence{Degrees: []string{"Master of Science"}})
		if b, ok := find(bs, CategoryEducation); !ok || !b.Met {
			t.Fatalf("want met education, got %+v ok=%v", b, ok)
		}
	})
	t.Run("a full free-text degree phrase still resolves", func(t *testing.T) {
		// A real résumé rarely says just "bachelor" — it says the full phrase.
		bs := Evaluate(JobRequirements{EducationLevel: "bachelor"}, CVEvidence{Degrees: []string{"Bachelor of Science in Computer Science"}})
		if b, ok := find(bs, CategoryEducation); !ok || !b.Met {
			t.Fatalf("want met education from full phrase, got %+v ok=%v", b, ok)
		}
	})
	t.Run("lower degree is a blocker", func(t *testing.T) {
		bs := Evaluate(JobRequirements{EducationLevel: "master"}, CVEvidence{Degrees: []string{"BSc"}})
		if b, ok := find(bs, CategoryEducation); !ok || b.Met {
			t.Fatalf("want unmet education, got %+v ok=%v", b, ok)
		}
	})
	t.Run("no parseable degree is skipped", func(t *testing.T) {
		bs := Evaluate(JobRequirements{EducationLevel: "bachelor"}, CVEvidence{Degrees: []string{"bootcamp"}})
		if _, ok := find(bs, CategoryEducation); ok {
			t.Error("education should be skipped with no parseable CV degree")
		}
	})
	t.Run("none requirement is skipped", func(t *testing.T) {
		bs := Evaluate(JobRequirements{EducationLevel: "none"}, CVEvidence{Degrees: []string{"BSc"}})
		if _, ok := find(bs, CategoryEducation); ok {
			t.Error("education should be skipped when requirement is none")
		}
	})
	t.Run("degree-optional posting skips the education blocker", func(t *testing.T) {
		// Master required, CV has only a bachelor — normally a blocker — but the
		// posting accepts equivalent experience, so no education blocker fires.
		bs := Evaluate(JobRequirements{EducationLevel: "master", DegreeOptional: true}, CVEvidence{Degrees: []string{"BSc"}})
		if _, ok := find(bs, CategoryEducation); ok {
			t.Error("education must be skipped when the posting is degree-optional")
		}
	})
}

func TestCertificationCategory(t *testing.T) {
	t.Run("held via alias is met", func(t *testing.T) {
		bs := Evaluate(
			JobRequirements{RequiredCertifications: []string{"cissp"}},
			CVEvidence{Certifications: []string{"Certified Information Systems Security Professional"}},
		)
		if b, ok := find(bs, CategoryCertification); !ok || !b.Met {
			t.Fatalf("want met certification, got %+v ok=%v", b, ok)
		}
	})
	t.Run("required cert absent from a résumé that lists others is a hard blocker", func(t *testing.T) {
		bs := Evaluate(
			JobRequirements{RequiredCertifications: []string{"pmp"}},
			CVEvidence{Certifications: []string{"AWS Certified Solutions Architect"}},
		)
		b, ok := find(bs, CategoryCertification)
		if !ok || b.Met || b.Severity != SeverityHard || b.ScoreCap != 60 {
			t.Fatalf("want unmet hard certification cap 60, got %+v ok=%v", b, ok)
		}
	})
	t.Run("no recognized cert evidence is skipped, never a false blocker", func(t *testing.T) {
		bs := Evaluate(JobRequirements{RequiredCertifications: []string{"pmp"}}, CVEvidence{})
		if _, ok := find(bs, CategoryCertification); ok {
			t.Error("certification must be skipped when the résumé carries no recognized certification")
		}
	})
}

func TestLanguageIsInfoOnly(t *testing.T) {
	t.Run("english present is met info", func(t *testing.T) {
		bs := Evaluate(JobRequirements{EnglishLevel: "b2"}, CVEvidence{Languages: []string{"English", "German"}})
		if b, ok := find(bs, CategoryLanguage); !ok || !b.Met {
			t.Fatalf("want met language info, got %+v ok=%v", b, ok)
		}
	})
	t.Run("english absent is skipped, never a blocker", func(t *testing.T) {
		bs := Evaluate(JobRequirements{EnglishLevel: "b2"}, CVEvidence{Languages: []string{"German"}})
		if _, ok := find(bs, CategoryLanguage); ok {
			t.Error("language must be skipped (not blocked) when the CV omits English")
		}
	})
}

func TestWorkAuthCategory(t *testing.T) {
	t.Run("no sponsorship and outside country is a hard blocker", func(t *testing.T) {
		bs := Evaluate(
			JobRequirements{VisaSponsorship: boolp(false), Countries: []string{"US"}},
			CVEvidence{CountryCode: "BR"},
		)
		b, ok := find(bs, CategoryWorkAuth)
		if !ok || b.Met || b.ScoreCap != 50 {
			t.Fatalf("want unmet work-auth cap 50, got %+v ok=%v", b, ok)
		}
	})
	t.Run("sponsorship offered is skipped", func(t *testing.T) {
		bs := Evaluate(JobRequirements{VisaSponsorship: boolp(true), Countries: []string{"US"}}, CVEvidence{CountryCode: "BR"})
		if _, ok := find(bs, CategoryWorkAuth); ok {
			t.Error("work-auth should be skipped when the job sponsors")
		}
	})
	t.Run("same country is skipped", func(t *testing.T) {
		bs := Evaluate(JobRequirements{VisaSponsorship: boolp(false), Countries: []string{"US"}}, CVEvidence{CountryCode: "us"})
		if _, ok := find(bs, CategoryWorkAuth); ok {
			t.Error("work-auth should be skipped when the caller is in-country")
		}
	})
	t.Run("unknown caller country is skipped", func(t *testing.T) {
		bs := Evaluate(JobRequirements{VisaSponsorship: boolp(false), Countries: []string{"US"}}, CVEvidence{})
		if _, ok := find(bs, CategoryWorkAuth); ok {
			t.Error("work-auth should be skipped when the caller country is unknown")
		}
	})
}

func TestLocationWorkModeCategory(t *testing.T) {
	t.Run("onsite versus remote preference is a soft blocker", func(t *testing.T) {
		bs := Evaluate(JobRequirements{WorkMode: "onsite"}, CVEvidence{PrefersRemote: true})
		b, ok := find(bs, CategoryLocationWorkMode)
		if !ok || b.Met || b.Severity != SeveritySoft {
			t.Fatalf("want unmet soft location blocker, got %+v ok=%v", b, ok)
		}
	})
	t.Run("remote job is skipped", func(t *testing.T) {
		bs := Evaluate(JobRequirements{WorkMode: "remote"}, CVEvidence{PrefersRemote: true, CountryCode: "BR"})
		if _, ok := find(bs, CategoryLocationWorkMode); ok {
			t.Error("a remote job should raise no location blocker")
		}
	})
}

func TestOverallCap(t *testing.T) {
	t.Run("hardest unmet blocker sets the ceiling", func(t *testing.T) {
		// Unmet certification (cap 60, résumé lists a different cert) and an unmet
		// location conflict (cap 75) → the harder ceiling wins.
		bs := Evaluate(
			JobRequirements{RequiredCertifications: []string{"pmp"}, WorkMode: "onsite"},
			CVEvidence{PrefersRemote: true, Certifications: []string{"AWS Certified Solutions Architect"}},
		)
		if got := OverallCap(bs); got != 60 {
			t.Errorf("OverallCap = %d, want 60 (certification beats location)", got)
		}
	})
	t.Run("no unmet blocker means no cap", func(t *testing.T) {
		bs := Evaluate(JobRequirements{ExperienceYearsMin: intp(3)}, CVEvidence{TotalYears: 5})
		if got := OverallCap(bs); got != 100 {
			t.Errorf("OverallCap = %d, want 100 when nothing is unmet", got)
		}
	})
}
