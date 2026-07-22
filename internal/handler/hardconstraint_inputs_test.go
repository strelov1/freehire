package handler

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/hardconstraint"
	"github.com/strelov1/freehire/internal/resumeextract"
	"github.com/strelov1/freehire/internal/userprofile"
)

func TestBuildHardConstraintInputs(t *testing.T) {
	job := db.Job{
		Description:        "Senior role. Requires 5+ years and an active CISSP. Bachelor's degree or equivalent experience.",
		EducationLevel:     "bachelor",
		ExperienceYearsMin: pgtype.Int4{Int32: 5, Valid: true},
		WorkMode:           "onsite",
		Countries:          []string{"US"},
		Enrichment:         []byte(`{"visa_sponsorship":false}`),
	}
	cv := resumeextract.Structured{
		TotalYears:     3,
		Education:      []resumeextract.Education{{Degree: "BSc in CS"}},
		Certifications: []string{"AWS Certified Solutions Architect"},
	}
	loc := userprofile.LocationPreferences{WorkModes: []string{"remote"}, Base: userprofile.BaseLocation{Country: "br"}}

	jr, ev := buildHardConstraintInputs(job, cv, loc)

	if jr.ExperienceYearsMin == nil || *jr.ExperienceYearsMin != 5 {
		t.Errorf("ExperienceYearsMin = %v, want 5", jr.ExperienceYearsMin)
	}
	if !jr.DegreeOptional {
		t.Error("DegreeOptional = false, want true (posting says 'or equivalent experience')")
	}
	if len(jr.RequiredCertifications) != 1 || jr.RequiredCertifications[0] != "cissp" {
		t.Errorf("RequiredCertifications = %v, want [cissp]", jr.RequiredCertifications)
	}
	if jr.VisaSponsorship == nil || *jr.VisaSponsorship {
		t.Errorf("VisaSponsorship = %v, want false", jr.VisaSponsorship)
	}
	if !ev.PrefersRemote || ev.CountryCode != "br" {
		t.Errorf("CV remote=%v country=%q, want true/br", ev.PrefersRemote, ev.CountryCode)
	}

	// End to end: experience unmet (3<5) and work-auth (no sponsorship, BR outside US)
	// and location (onsite vs remote-pref) block; education is degree-optional so skipped.
	bs := hardconstraint.Evaluate(jr, ev)
	if _, ok := findBlocker(bs, hardconstraint.CategoryEducation); ok {
		t.Error("education should be skipped (degree-optional)")
	}
	if b, ok := findBlocker(bs, hardconstraint.CategoryExperience); !ok || b.Met {
		t.Error("experience should be an unmet blocker")
	}
}

func findBlocker(bs []hardconstraint.Blocker, cat hardconstraint.BlockerCategory) (hardconstraint.Blocker, bool) {
	for _, b := range bs {
		if b.Category == cat {
			return b, true
		}
	}
	return hardconstraint.Blocker{}, false
}
