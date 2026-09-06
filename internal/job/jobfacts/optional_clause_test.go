package jobfacts

import "testing"

// A qualification a posting calls preferred is not a requirement, and the matchers
// below are read by internal/candidate/hardconstraint as a score ceiling — so
// promoting one caps a candidate's fit for something nobody asked for.
func TestOptionalQualificationsAreNotFacts(t *testing.T) {
	education := []struct{ name, desc, want string }{
		{"preferred master is not a requirement", "A Master's degree is preferred.", ""},
		{"a plus is not a requirement", "An MBA is a plus.", ""},
		{"the required degree survives its preferred neighbour", "Bachelor's required, PhD preferred.", "bachelor"},
		{"a preferred section of paragraphs", "<h3>Elvárások</h3><p>BSc diploma.</p><h3>Előnyt jelent</h3><p>PhD.</p>", "bachelor"},
		{"russian desirable", "Требуется степень бакалавра. Магистратура желательна.", ""},
	}
	for _, c := range education {
		t.Run("education/"+c.name, func(t *testing.T) {
			if got := EducationLevel(c.desc); got != c.want {
				t.Errorf("EducationLevel(%q) = %q, want %q", c.desc, got, c.want)
			}
		})
	}

	english := []struct{ name, desc, want string }{
		{"preferred level", "C1 English preferred.", ""},
		{"hungarian preferred section", "<h3>Előnyt jelent</h3><p>C1 English.</p>", ""},
		{"the required level survives its preferred neighbour", "B2 English required, C1 preferred.", "b2"},
		{"polish nice to have", "Angielski na poziomie C1 mile widziany.", ""},
	}
	for _, c := range english {
		t.Run("english/"+c.name, func(t *testing.T) {
			if got := EnglishLevel(c.desc); got != c.want {
				t.Errorf("EnglishLevel(%q) = %q, want %q", c.desc, got, c.want)
			}
		})
	}

	t.Run("experience/preferred years alone yield nothing", func(t *testing.T) {
		if got := ExperienceYearsMin("<h3>Nice to have</h3><ul><li>5 years of Go</li></ul>"); got != nil {
			t.Errorf("ExperienceYearsMin = %v, want nil", *got)
		}
	})
	t.Run("experience/the required figure survives", func(t *testing.T) {
		got := ExperienceYearsMin("3 years required; 5 years preferred.")
		if got == nil || *got != 3 {
			t.Errorf("ExperienceYearsMin = %v, want 3", got)
		}
	})
}

// Masking must blank WORDS, never restructure the text around them. The matchers read
// punctuation as structure — EnglishLevel binds a level to an English keyword only
// when no "." or newline separates them — so a masker that removed a clause and
// rejoined the halves would invent a boundary the posting never had and silently drop
// the level from the commonest phrasing there is.
func TestMaskingDoesNotInventSentenceBoundaries(t *testing.T) {
	cases := []struct{ desc, want string }{
		{"English, B2 level required.", "b2"},
		{"Требуется английский, уровень B2.", "b2"},
		{"Advanced English, written and spoken.", "c1"},
		{"Angielski na poziomie min. B2, praca zdalna.", "b2"},
		{"English: fluent, spoken and written.", "c1"},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			if got := EnglishLevel(c.desc); got != c.want {
				t.Errorf("EnglishLevel(%q) = %q, want %q", c.desc, got, c.want)
			}
		})
	}
}

// A non-breaking space ends a sentence as an ordinary one does — descriptions pasted
// out of a word processor are full of them — and reading the byte after the period
// instead of the rune would have missed it, blanking the required clause along with the
// preferred one before it.
func TestSentenceBreakOnANonBreakingSpace(t *testing.T) {
	got := RequiredCertifications("PMP preferred. CISSP required.")
	if len(got) != 1 || got[0] != "cissp" {
		t.Errorf("RequiredCertifications = %v, want [cissp] only", got)
	}
}

// The mask works in clauses, so a marker blanks the whole clause it sits in — including
// a requirement coordinated into that same clause by "but" or "while". This is the
// direction the design chooses when it cannot tell: understating a requirement leaves a
// candidate's fit uncapped, which a reader can detect, while overstating one caps the
// score for something nobody asked for, which is the defect this fixes. Pinned rather
// than left to be discovered, because the way OUT is a vocabulary of conjunctions in
// four languages and that is a bigger bet than the one this makes.
func TestACoordinatedRequirementIsLostWithItsClause(t *testing.T) {
	if got := RequiredCertifications("CISSP preferred but CISA required."); len(got) != 0 {
		t.Errorf("RequiredCertifications = %v, want none — the clause is blanked whole", got)
	}
	// Punctuated, which is how a posting states this far more often, it survives.
	got := RequiredCertifications("CISSP preferred; CISA required.")
	if len(got) != 1 || got[0] != "cisa" {
		t.Errorf("RequiredCertifications = %v, want [cisa]", got)
	}
}

// The whole point of the fix is a posting whose preferred section sits beside a real
// requirement — which is also the only shape that gets re-rendered. If the re-render
// re-escapes the requirement's text, the fix silently costs the fact it was meant to
// protect. Prod rows found this; nothing here did.
func TestARequirementSurvivesTheRenderItsPreferredSectionForces(t *testing.T) {
	const preferred = `<h3>Nice to have</h3><ul><li>Kubernetes, a PMP</li></ul>`
	if got := EducationLevel(`<h3>Requirements</h3><p>Bachelor's degree required.</p>` + preferred); got != "bachelor" {
		t.Errorf("EducationLevel = %q, want %q", got, "bachelor")
	}
	got := RequiredCertifications(`<h3>Requirements</h3><p>A commercial driver's license.</p>` + preferred)
	if len(got) != 1 || got[0] != "cdl" {
		t.Errorf("RequiredCertifications = %v, want [cdl]", got)
	}
}
