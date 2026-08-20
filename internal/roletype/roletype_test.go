package roletype

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/vocab"
)

func TestDerive(t *testing.T) {
	cases := []struct {
		name, title, want string
	}{
		// Unambiguous markers. Each names a person who has reports, in any craft —
		// which is why the facet is orthogonal to `category`: on production 91% of
		// these titles sit in a category other than `management`.
		{"head of", "Head of Platform Engineering", "people_manager"},
		{"director", "Director of Data Engineering", "people_manager"},
		{"director bare", "Engineering Director", "people_manager"},
		{"vp", "VP, Infrastructure", "people_manager"},
		{"vice president", "Vice President of Product Engineering", "people_manager"},
		{"chief", "Chief Technology Officer", "people_manager"},
		{"supervisor", "Production Supervisor", "people_manager"},

		// A manager qualified by a craft is a people manager; see the IC block below
		// for why the bare word is not.
		{"engineering manager", "Engineering Manager, Payments", "people_manager"},
		{"data manager", "Data Manager", "people_manager"},
		{"qa manager", "Senior QA Manager", "people_manager"},
		{"software manager", "Software Development Manager", "people_manager"},

		// Nothing stated about management. The empty string means "no marker", NOT
		// "individual contributor" — the catalogue cannot show the latter.
		{"plain engineer", "Backend Engineer", ""},
		{"senior engineer", "Senior Software Engineer", ""},
		{"empty", "", ""},
		{"whitespace", "   ", ""},

		// The word "manager" with a non-person object. 150,779 of production's
		// 378,410 "manager" titles are these — 40% — so admitting the bare word would
		// make the facet wrong for two matches in five.
		{"product manager", "Product Manager", ""},
		{"senior product manager", "Senior Product Manager", ""},
		{"project manager", "Project Manager", ""},
		{"program manager", "Program Manager", ""},
		{"account manager", "Account Manager", ""},
		{"marketing manager", "Marketing Manager", ""},
		{"community manager", "Community Manager", ""},
		{"bare manager", "Manager", ""},

		// "Chief" is the one marker with a senior-IC sense: Chief Engineer, Chief
		// Architect and Chief Scientist are the top of an individual-contributor
		// ladder, not people who have reports. Chief of Staff is an exec aide whose
		// reports are usually nobody. These are what the blind-phrase mask is for.
		{"chief engineer is IC", "Chief Engineer", ""},
		{"chief architect is IC", "Chief Architect", ""},
		{"chief scientist is IC", "Chief Scientist", ""},
		{"chief of staff", "Chief of Staff", ""},

		// …but a Chief of <function> runs that function. Masking it was a slip: the
		// test one block down already reads "Head of Product" as a manager, and the
		// same office spelled "Chief" cannot mean the opposite.
		{"chief of product", "Chief of Product", "people_manager"},
		{"chief of engineering", "Chief of Engineering", "people_manager"},

		// The graded VP forms are the same office. "vp" is a whole word, so it does not
		// fire inside "svp" — each spelling has to be its own entry.
		{"svp", "SVP, Product Engineering", "people_manager"},
		{"evp", "EVP of Technology", "people_manager"},
		{"avp", "AVP, Engineering", "people_manager"},

		// A blind phrase must not swallow a LONGER word that merely starts with it:
		// "chief engineer" is a prefix of "chief engineering", and a Chief Engineering
		// Officer runs the org. Cutting on a bare substring destroyed the marker.
		{"chief engineering officer", "Chief Engineering Officer", "people_manager"},
		{"chief architecture officer", "Chief Architecture Officer", "people_manager"},
		{"chief engineering manager", "Chief Engineering Manager", "people_manager"},

		// The mask is CUT before matching rather than used to reject the title, so a
		// real marker elsewhere in the same title still resolves.
		{"director beside a masked phrase", "Director of Engineering, Chief Architect Track", "people_manager"},
		{"director of product management", "Director of Product Management", "people_manager"},
		{"head of product", "Head of Product", "people_manager"},

		// "Lead" is deliberately unresolved: production's `seniority=lead` holds
		// 116,893 postings and only 3,303 carry any management marker, so the word
		// names the IC ladder far more often than a manager. Guessing either way
		// would break the dict-only rule.
		{"tech lead", "Tech Lead", ""},
		{"lead engineer", "Lead Software Engineer", ""},
		{"team lead", "Team Lead", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Derive(c.title); got != c.want {
				t.Errorf("Derive(%q) = %q, want %q", c.title, got, c.want)
			}
		})
	}
}

// TestValuesAreInVocabulary guards that every value Derive can return is a member of
// the shared vocabulary, so the facet config and this dictionary cannot drift.
func TestValuesAreInVocabulary(t *testing.T) {
	titles := []string{
		"Head of Platform Engineering", "Director of Data Engineering", "VP, Infrastructure",
		"Chief Technology Officer", "Production Supervisor", "Engineering Manager",
		"Backend Engineer", "Product Manager", "",
	}
	for _, title := range titles {
		got := Derive(title)
		if got == "" {
			continue
		}
		if !slices.Contains(vocab.RoleTypeValues, got) {
			t.Errorf("Derive(%q) = %q, which is not in vocab.RoleTypeValues", title, got)
		}
	}
}
