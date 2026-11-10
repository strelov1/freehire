package roletype

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

func TestDerive(t *testing.T) {
	cases := []struct {
		name, title, want string
	}{
		// Unambiguous markers. Each names a person who has reports, in any craft —
		// which is why the facet is orthogonal to `category`.
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

		// The generic people-managing titles of the service economy. With the
		// management category retired this facet is the only one that reaches them,
		// and search keeps a posting it reaches.
		{"general manager", "General Manager", "people_manager"},
		{"assistant general manager", "Assistant General Manager", "people_manager"},
		{"assistant manager", "Assistant Manager - Part Time", "people_manager"},
		{"store manager", "Store Manager", "people_manager"},
		{"restaurant manager", "Restaurant Manager", "people_manager"},
		{"branch manager", "Branch Manager", "people_manager"},
		{"district manager", "District Manager", "people_manager"},
		{"area manager", "Area Manager", "people_manager"},
		{"plant manager", "Plant Manager", "people_manager"},
		{"site manager", "Site Manager", "people_manager"},
		{"shift manager", "Shift Manager (Auntie Anne's)", "people_manager"},
		{"department manager", "Department Manager", "people_manager"},
		{"team manager", "Contact Center Team Manager", "people_manager"},

		// A manager title naming no person-shaped object stays unresolved — the bare
		// word included, for the reason the IC block above gives.
		{"case manager", "Case Manager", ""},
		{"portfolio manager", "Portfolio Manager", ""},
		{"category manager", "Category Manager", ""},
		{"senior manager", "Senior Manager, Strategy", ""},

		// Russian heads of an office: the nominative of both genders, and the genitive a
		// deputy title puts its office in.
		{"начальник", "Начальник отдела кадров", "people_manager"},
		{"начальник смены", "Начальник смены", "people_manager"},
		{"заведующий", "Заведующий складом", "people_manager"},
		{"заведующая", "Заведующая детским садом", "people_manager"},
		{"директор", "Директор школы", "people_manager"},
		{"директор магазина", "Директор магазина", "people_manager"},
		{"заместитель директора", "Заместитель директора по АХЧ", "people_manager"},
		{"руководитель", "Руководитель отдела продаж", "people_manager"},
		{"заместитель руководителя", "Заместитель руководителя службы", "people_manager"},

		// …and the Russian titles that carry the word without the reports.
		{"music teacher", "Музыкальный руководитель", ""},
		{"hobby club leader", "Руководитель кружка", ""},
		{"PE teacher", "Руководитель физического воспитания", ""},
		{"project manager ru", "Руководитель проекта", ""},
		{"assistant to the head", "Помощник руководителя", ""},
		{"director's adviser", "Советник директора по воспитанию", ""},
		{"bare менеджер", "Менеджер", ""},
		{"sales менеджер", "Менеджер по продажам", ""},

		// "Lead" is deliberately unresolved: production's `seniority=lead` holds
		// 116,893 postings and only 3,303 carry any management marker, so the word
		// names the IC ladder far more often than a manager. Guessing either way
		// would break the dict-only rule.
		// Croatian, Serbian, Bosnian and Slovenian heads (#4412): a manager noun
		// ("voditelj", "šef", "vodja") counts only before a workplace — branch, shop,
		// department, service, sector, shift, group — as "Branch/Department/Shift
		// Manager" do; the director in the nominative, or after a deputy.
		{"bcs branch", "Voditelj poslovnice (m/ž)", "people_manager"},
		{"bcs branch slash", "VODITELJ/ICA POSLOVNICE", "people_manager"},
		{"bcs deputy store", "Zamjenik voditelja trgovine (m/ž)", "people_manager"},
		{"bcs outlet", "VODITELJ / VODITELJICA MALOPRODAJNOG MJESTA", "people_manager"},
		{"bcs department", "VODITELJ/VODITELJICA ODJELA DELIKATESA", "people_manager"},
		{"bcs store manager", "Poslovođa (m/ž)", "people_manager"},
		{"bcs deputy store manager", "Zamenik poslovođe", "people_manager"},
		{"bcs shift", "Voditelj smjene (m/ž)", "people_manager"},
		{"bcs shift chief", "Šef smene (na pumpi)", "people_manager"},
		{"bcs head of department", "Šef/ica odeljenja za razvoj platnih kartica", "people_manager"},
		{"bcs director", "DIREKTOR/DIREKTORICA PODRUŽNICE DUGA RESA", "people_manager"},
		{"bcs deputy director", "Zamjenik direktora (m/ž)", "people_manager"},
		{"sr cyrillic director", "Директор маркета", "people_manager"},
		{"sr cyrillic branch", "Шеф пословнице", "people_manager"},
		{"sl branch", "Vodja poslovalnice, Cash & Carry Ljubljana", "people_manager"},
		{"sl business unit deputy", "Namestnik vodje poslovne enote Kamnik", "people_manager"},
		{"sl shift", "Vodja izmene SPAR Sežana", "people_manager"},
		{"sl shift one word", "Izmenovodja (m/ž)", "people_manager"},
		{"sl group", "Skupinovodja v sektorju prometne signalizacije (m/ž)", "people_manager"},
		{"sl department", "VODJA ODDELKA (ODDELEK AVTOELEKTRIKE)", "people_manager"},
		{"sl general director", "Generalni direktor", "people_manager"},

		// …and the same nouns without a workplace, or working for the director.
		{"bcs project lead", "Voditelj projekta (m/ž)", ""},
		{"sl sales lead", "Vodja prodaje", ""},
		{"bcs bare voditelj", "Voditelj/ica", ""},
		{"bcs director's assistant", "Pomoćnik direktora", ""},
		{"sl director's office", "Svetovalec za področje podpore in koordinacije projektov v Uradu direktorja", ""},
		{"sl director's secretariat", "STROKOVNI SODELAVEC VII/1 V TAJNIŠTVU GENERALNEGA DIREKTORJA", ""},
		{"directorate is not director", "VIŠJI SVETOVALEC, DIREKTORAT ZA OKOLJE", ""},

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
