package sources

import (
	"testing"
)

// TestProfessionPlace pins the two things the address line states and the ld+json does not:
// which of the platform's three work-mode badges the posting carries, and a place the
// location dictionary can read.
func TestProfessionPlace(t *testing.T) {
	cases := []struct {
		name, line, wantMode, wantPlace string
	}{
		{"sited address", "1087 Budapest, Kerepesi út 21.", "onsite", "Budapest, Kerepesi út ."},
		{"hybrid with a place", "Hibrid • 9024 Győr, Práter utca 9.", "hybrid", "Győr, Práter utca ."},
		{"hybrid with no place", "Hibrid", "hybrid", ""},
		{"remote", "Távmunka / Remote", "remote", ""},
		// "Nationwide coverage" is where the work is, not how it is done, and it names no
		// place either.
		{"nationwide", "Országos lefedettség", "", ""},
		{"bare city", "Budaörs", "onsite", "Budaörs"},
		{"nothing stated", "", "onsite", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mode, place := professionPlace(tc.line)
			if mode != tc.wantMode || place != tc.wantPlace {
				t.Errorf("professionPlace(%q) = %q/%q, want %q/%q",
					tc.line, mode, place, tc.wantMode, tc.wantPlace)
			}
		})
	}
}

// TestProfessionEmploymentType pins the picklist's two halves apart: it states a contract
// form and a schedule in one comma-joined field, and only the schedule is an employment type.
func TestProfessionEmploymentType(t *testing.T) {
	cases := map[string]string{
		"":                                     "",
		"Alkalmazotti jogviszony":              "",
		"Vállalkozói, Alkalmazotti jogviszony": "",
		"Teljes munkaidő, Alkalmazotti jogviszony": "full_time",
		"Részmunkaidő, Alkalmazotti jogviszony":    "part_time",
		"Diákmunka": "internship",
		// Both schedules ticked names neither.
		"Teljes munkaidő, Részmunkaidő": "",
	}
	for field, want := range cases {
		if got := professionEmploymentType(field); got != want {
			t.Errorf("professionEmploymentType(%q) = %q, want %q", field, got, want)
		}
	}
}

func TestProfessionExperienceYears(t *testing.T) {
	cases := map[string]*int{
		"Nem kell tapasztalat":      intPtr(0),
		"Pályakezdő/friss diplomás": intPtr(0),
		"1-3 év tapasztalat":        intPtr(1),
		"3-5 év tapasztalat":        intPtr(3),
		"5-10 év tapasztalat":       intPtr(5),
		">10 év tapasztalat":        intPtr(10),
		"":                          nil,
		"Some new band":             nil,
	}
	for level, want := range cases {
		got := professionExperienceYears(level)
		switch {
		case want == nil && got != nil:
			t.Errorf("professionExperienceYears(%q) = %d, want nil", level, *got)
		case want != nil && (got == nil || *got != *want):
			t.Errorf("professionExperienceYears(%q) = %v, want %d", level, got, *want)
		}
	}
}

// TestProfessionEducationAndLanguage pins the platform's educationRequirements picklist,
// which states two different things in one comma-joined field: zero or more LANGUAGE
// levels and exactly one SCHOOL level. The whole closed vocabulary — 28 tokens — was read
// off all 709 live postings, so a value outside it is a change on the platform's side and
// must yield nothing rather than a guess.
func TestProfessionEducationAndLanguage(t *testing.T) {
	cases := []struct {
		name, field, wantEdu, wantEng string
	}{
		// The two commonest values, together 46% of the slice.
		{"intermediate english, secondary school", "Angol középfok, Középiskola", "none", "b2"},
		{"intermediate english, college", "Angol középfok, Főiskola", "bachelor", "b2"},
		// Hungary's exam levels are NOT "basic/medium/high": alapfok/középfok/felsőfok are
		// the state-recognised levels and map to B1/B2/C1 (Gov. Decree 137/2008). The trio
		// has no name for C2, which is why felsőfok is so often mistranslated as one.
		{"basic english is B1, not A-something", "Angol alapfok, Középiskola", "none", "b1"},
		{"advanced english is C1, not C2", "Angol felsőfok, Egyetem", "master", "c1"},
		{"native english", "Angol anyanyelvi szint, Egyetem", "master", "native"},
		// The platform's own way of saying no language is required.
		{"no language needed", "Nem kell nyelvtudás, Középiskola", "none", "none"},
		// A posting may require several languages; only English answers this facet, and a
		// posting naming other languages says nothing about English.
		{"english alongside german", "Angol középfok, Német középfok, Középiskola", "none", "b2"},
		{"german only states no english level", "Német felsőfok, Egyetem", "master", ""},
		{"hungarian native alongside english", "Angol felsőfok, Magyar anyanyelvi szint, Egyetem", "master", "c1"},
		// School levels below a degree are the platform stating that no degree is needed.
		{"primary school", "Nem kell nyelvtudás, Általános iskola", "none", "none"},
		{"vocational school", "Angol alapfok, Szakiskola / szakmunkás képző", "none", "b1"},
		// Felsőoktatási szakképzés is a two-year tertiary programme, below a bachelor's and
		// above secondary — freehire's four-value vocabulary has no member for it, so it
		// yields nothing rather than rounding to a neighbour.
		{"higher vocational maps to neither", "Angol középfok, Felsőoktatási szakképzés", "", "b2"},
		{"empty field", "", "", ""},
		{"a value the platform has not published before", "Angol mesterfok, Doktori", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			edu, eng := professionEducationAndLanguage(tc.field)
			if edu != tc.wantEdu || eng != tc.wantEng {
				t.Errorf("professionEducationAndLanguage(%q) = %q/%q, want %q/%q",
					tc.field, edu, eng, tc.wantEdu, tc.wantEng)
			}
		})
	}
}
