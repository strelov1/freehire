package classify

// Indian titles state a grade in shapes the main table does not read: a numbered rung
// ("SDE-2", "Software Engineer II"), an "Associate" before the role, the graded
// Member-of-Technical-Staff acronyms, and the campus-hire words ("Fresher"). Measured
// 2026-10-25 over 63,305 Indian postings, 31.6% of titles resolved a grade; most of the
// rest state none at all, as everywhere.
//
// indianSeniorityTable is a FALLBACK, consulted only when the main table resolved no
// grade — so "Senior Software Engineer II" stays senior — and before the German one.
// Every value is a member of vocab.SeniorityValues. No grade is read from a years figure
// ("5+ years"): that is the same inference the description matcher refuses.

// indianSeniorityTable lists the aliases in precedence order.
var indianSeniorityTable = buildIndianSeniorityTable()

// rungGrades maps a ladder rung to its grade. The SDE ladder (and its SDET twin) is read
// to the third rung, which is senior on every ladder that uses the name. Elsewhere only
// the first two rungs are read: "Software Engineer III" is a senior grade at some
// employers and a mid one at others, so it states nothing this dictionary may repeat.
var rungGrades = []struct {
	rungs []string
	grade string
}{
	{[]string{"iii", "3"}, "senior"},
	{[]string{"ii", "2"}, "middle"},
	{[]string{"i", "1"}, "junior"},
}

// sdeLadders are the role names whose numbered rungs include the third one.
var sdeLadders = []string{
	"software development engineer in test", "software development engineer",
	"sdet", "sde",
}

// genericLadders are the role nouns whose first two rungs are read — in Roman numerals
// only: "Java Developer 2-5 Years" is an experience range, not a rung.
var genericLadders = []string{"engineer", "developer", "analyst", "scientist", "consultant"}

// associateRoles are the roles "Associate" grades as the entry rung: an Associate
// Software Engineer or Associate Consultant is the campus hire. "Associate" alone, or
// before a rank ("Associate Director") or a shop-floor role ("Sales Associate" — the
// word comes after), states no grade.
var associateRoles = []string{
	"software engineer", "software developer", "engineer", "developer", "consultant",
	"product manager", "data scientist", "data analyst", "data engineer", "analyst",
	"qa engineer", "test engineer", "programmer", "designer",
}

func buildIndianSeniorityTable() []aliasEntry {
	var t []aliasEntry
	for _, rg := range rungGrades {
		for _, ladder := range sdeLadders {
			for _, r := range rg.rungs {
				t = append(t,
					aliasEntry{ladder + " " + r, rg.grade},
					aliasEntry{ladder + "-" + r, rg.grade},
					aliasEntry{ladder + " - " + r, rg.grade})
				if ladder == "sde" || ladder == "sdet" {
					t = append(t, aliasEntry{ladder + r, rg.grade}) // "SDE1"
				}
			}
		}
	}
	for _, rg := range rungGrades[1:] {
		roman := rg.rungs[0]
		for _, noun := range genericLadders {
			t = append(t,
				aliasEntry{noun + " " + roman, rg.grade},
				aliasEntry{noun + " - " + roman, rg.grade})
		}
	}
	t = append(t,
		// The graded Member-of-Technical-Staff acronyms. Bare "MTS" is the generic IC
		// title, as the spelled-out phrase is (gradeBlindPhrases).
		aliasEntry{"pmts", "principal"},
		aliasEntry{"lmts", "lead"},
		aliasEntry{"smts", "senior"},
		aliasEntry{"team leader", "lead"},
	)
	for _, role := range associateRoles {
		t = append(t, aliasEntry{"associate " + role, "junior"})
	}
	t = append(t,
		// A fresher is a new graduate hired with no experience.
		aliasEntry{"fresher", "junior"},
		aliasEntry{"freshers", "junior"},
		// An apprentice is trained under contract rather than employed, as a trainee is.
		aliasEntry{"apprentice", "intern"},
		aliasEntry{"apprenticeship", "intern"},
	)
	return t
}
