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
