package jobfacts

import (
	"slices"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/vocab"
)

func TestEmploymentType(t *testing.T) {
	cases := []struct {
		name, title, desc, want string
	}{
		{"unstated -> empty", "Software Engineer", "Build great things.", ""},
		{"internship", "Software Engineering Intern", "A summer internship program.", "internship"},
		{"intern word not internal", "Engineer", "Work on internal international internet tools.", ""},
		{"part time", "Barista", "This is a part-time role, 20h/week.", "part_time"},
		{"contract", "Consultant", "6-month contract, fixed-term engagement.", "contract"},
		{"contractor", "Dev", "We hire a contractor for this.", "contract"},
		{"freelance", "Designer", "Freelance, remote.", "contract"},
		{"temporary -> contract", "Picker", "Temporary seasonal position.", "contract"},
		{"1099 -> contract", "Dev", "This is a 1099 position.", "contract"},
		{"c2c -> contract", "Dev", "Open to C2C candidates.", "contract"},
		{"corp-to-corp -> contract", "Dev", "W2 or corp-to-corp accepted.", "contract"},
		{"corp to corp spaced -> contract", "Dev", "Corp to corp engagement.", "contract"},
		// b2b/c2c are business-model prose far more often than an employment type, so the
		// bare token must not override an explicit full-time or mislabel an ordinary posting.
		{"b2b saas business model does not beat full-time", "Backend Engineer", "We build B2B SaaS for enterprises. Full-time, permanent.", "full_time"},
		{"b2b product prose is not contract", "Engineer", "Join us building a b2b product for enterprises.", ""},
		{"c2c marketplace is not contract", "Engineer", "We run a C2C marketplace for hobbyists.", ""},
		{"full time", "Engineer", "Full-time, permanent position.", "full_time"},
		{"internship beats full-time", "Intern", "A full-time internship for students.", "internship"},
		{"fellowship", "Postdoctoral Fellowship, Applied Data Science", "Join our research team as a fellow.", "fellowship"},
		{"fellowship beats contract", "PhD Fellowship", "A 3-year fixed-term fellowship position.", "fellowship"},
		{"fellowship manager is staff, not fellowship", "Fellowship Manager, Anywhere", "Runs our fellowship program full-time.", "full_time"},
		{"fellowship program coordinator is staff", "Fellowship Program Coordinator", "Coordinates the fellowship program.", ""},
		{"fellowship mention far from staff word still counts", "AI Policy Fellowship", strings.Repeat("Research the impact of AI governance policy. ", 20) + "Reports to the Program Manager.", "fellowship"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EmploymentType(c.title, c.desc); got != c.want {
				t.Errorf("EmploymentType(%q,%q) = %q, want %q", c.title, c.desc, got, c.want)
			}
		})
	}
}

func TestEducationLevel(t *testing.T) {
	cases := []struct{ name, desc, want string }{
		{"unstated", "Strong coding skills required.", ""},
		{"bachelor", "Bachelor's degree in CS or equivalent.", "bachelor"},
		{"bsc abbrev", "BSc in Computer Science required.", "bachelor"},
		{"master", "A Master's degree is preferred.", "master"},
		{"mba", "An MBA is a plus.", "master"},
		{"phd", "PhD in Machine Learning required.", "phd"},
		{"phd dotted", "Ph.D. or equivalent research experience.", "phd"},
		{"phd beats bachelor", "Bachelor's required, PhD preferred.", "phd"},
		{"bachelor degree no apostrophe", "A bachelor degree in CS is required.", "bachelor"},
		{"explicit none", "No degree required for this role.", "none"},
		{"degree word alone not enough", "This is a degree of difficulty.", ""},
		{"MS Office is not a master's", "Proficiency in MS Office and MS SQL Server.", ""},
		{"scrum master is not a degree", "Experienced scrum master leading the team.", ""},
		{"bare BS is not bachelor", "This role involves a lot of bs paperwork.", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EducationLevel(c.desc); got != c.want {
				t.Errorf("EducationLevel(%q) = %q, want %q", c.desc, got, c.want)
			}
		})
	}
}

func TestExperienceYearsMin(t *testing.T) {
	ptr := func(n int) *int { return &n }
	cases := []struct {
		name, desc string
		want       *int
	}{
		{"unstated", "Great communication skills.", nil},
		{"plain", "5 years of experience required.", ptr(5)},
		{"plus", "7+ years building distributed systems.", ptr(7)},
		{"range low end", "3-5 years of relevant experience.", ptr(3)},
		{"to range", "2 to 4 years experience.", ptr(2)},
		{"yrs abbrev", "10 yrs experience.", ptr(10)},
		{"min across mentions", "5 years of Go and 2 years of Kubernetes.", ptr(2)},
		{"age ignored", "Must be 18 years of age. 4+ years experience.", ptr(4)},
		{"hyperbole capped out", "100 years of fun awaits you.", nil},

		// An entry-level posting states the requirement in prose, not as a figure.
		// Reading only digits left that population indistinguishable from a posting
		// that says nothing at all, so the filter could not reach it.
		{"no experience required", "No prior experience required — we will train you.", ptr(0)},
		{"no experience necessary", "No experience necessary. Full training provided.", ptr(0)},
		{"no previous experience needed", "No previous experience needed for this role.", ptr(0)},
		{"copula between", "No prior experience is required for this position.", ptr(0)},
		{"zero wins the floor", "No prior experience required. 3 years with Go is a plus.", ptr(0)},
		{"hyperbole with explicit no-experience", "100 years of fun, no experience needed.", ptr(0)},

		// The phrase is about a TOOL, not the job: "no prior experience with X is
		// required" means X is optional. Requiring the statement to close without an
		// object in between is what keeps this out.
		{"tool-scoped is not entry level", "No prior experience with Kubernetes is required, but 5 years of backend is.", ptr(5)},
		{"domain-scoped is not entry level", "No previous experience in fintech required; 4+ years engineering expected.", ptr(4)},

		// The object can also TRAIL the requirement word, which reads the same way:
		// the tool is optional, the role is not. Guarding only the leading order
		// caught one word order and waved the other through.
		{"trailing tool scope", "No prior experience is required with our proprietary CRM. The role needs 5+ years of enterprise sales.", ptr(5)},
		{"trailing domain scope", "No experience required in Kubernetes, but 4 years of backend is expected.", ptr(4)},

		// A trailing "for <the role>" is the ordinary entry-level phrasing, not a
		// scoping object, so it must survive the guard above.
		{"trailing role reference still entry level", "No experience necessary for this position.", ptr(0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExperienceYearsMin(c.desc)
			switch {
			case got == nil && c.want == nil:
			case got == nil || c.want == nil:
				t.Errorf("ExperienceYearsMin(%q) = %v, want %v", c.desc, got, c.want)
			case *got != *c.want:
				t.Errorf("ExperienceYearsMin(%q) = %d, want %d", c.desc, *got, *c.want)
			}
		})
	}
}

// TestValuesAreInVocabulary guards that every value the matchers can return is a
// member of the enrichment contract's controlled vocabulary, so jobfacts and the
// served enum never drift apart.
func TestValuesAreInVocabulary(t *testing.T) {
	for _, v := range []string{"internship", "part_time", "contract", "full_time", "fellowship"} {
		if !slices.Contains(vocab.EmploymentTypeValues, v) {
			t.Errorf("employment_type %q not in vocab.EmploymentTypeValues", v)
		}
	}
	for _, v := range []string{"none", "bachelor", "master", "phd"} {
		if !slices.Contains(vocab.EducationLevelValues, v) {
			t.Errorf("education_level %q not in vocab.EducationLevelValues", v)
		}
	}
	for _, v := range []string{"none", "a1", "a2", "b1", "b2", "c1", "native"} {
		if !slices.Contains(vocab.EnglishLevelValues, v) {
			t.Errorf("english_level %q not in vocab.EnglishLevelValues", v)
		}
	}
}

func TestEnglishLevel(t *testing.T) {
	cases := []struct{ name, desc, want string }{
		{"unstated", "Strong coding skills required.", ""},
		{"english mentioned, no level", "You will write docs in English.", ""},
		// CEFR codes, only in English context.
		{"cefr b2", "English level B2 required.", "b2"},
		{"cefr c1 code before english", "C1 English is a must.", "c1"},
		{"cefr russian context", "Английский язык на уровне B2.", "b2"},
		{"b2b is not b2", "We build B2B SaaS. English is used daily.", ""},
		{"a1 out of context ignored", "Experience with the Audi A1 platform.", ""},
		// Phrase levels (EN).
		{"native", "Native English speaker required.", "native"},
		{"fluent -> c1", "Fluent English is required.", "c1"},
		{"advanced -> c1", "Advanced English, written and spoken.", "c1"},
		{"upper-intermediate -> b2", "Upper-intermediate English or higher.", "b2"},
		{"intermediate -> b1", "Intermediate English is enough.", "b1"},
		{"pre-intermediate -> a2", "Pre-intermediate English acceptable.", "a2"},
		{"elementary -> a1", "Elementary English is fine.", "a1"},
		{"basic -> a2", "Basic English required.", "a2"},
		{"advanced degree is not english", "An advanced degree in CS is required.", ""},
		{"native app is not native english", "Build native iOS apps. English docs provided.", ""},
		// Phrase levels (RU).
		{"english conversational -> b1", "Conversational English is enough.", "b1"},
		{"russian native speaker -> native", "Требуется носитель английского языка.", "native"},
		{"russian fluent -> c1", "Свободный английский обязателен.", "c1"},
		{"russian conversational -> b1", "Нужен разговорный английский.", "b1"},
		{"russian above-average -> b2", "Английский выше среднего.", "b2"},
		{"russian average -> b1", "Английский на среднем уровне.", "b1"},
		{"russian basic -> a2", "Базовый английский достаточно.", "a2"},
		// Phrase levels (PL) — Polish boards (NoFluffJobs, JustJoinIT) state English
		// requirements in Polish; "angielski" is Polish for "English".
		{"polish cefr b2", "Angielski na poziomie min. B2+ -> międzynarodowa współpraca", "b2"},
		{"polish fluent -> c1", "Biegły angielski jest wymagany.", "c1"},
		{"polish advanced -> c1", "Zaawansowana znajomość języka angielskiego.", "c1"},
		{"polish upper-intermediate -> b2", "Wyższy średniozaawansowany angielski.", "b2"},
		{"polish intermediate -> b1", "Średniozaawansowany angielski wystarczy.", "b1"},
		{"polish conversational -> b1", "Komunikatywny angielski.", "b1"},
		{"polish beginner -> a1", "Początkujący angielski jest ok.", "a1"},
		{"polish basic -> a2", "Podstawowa znajomość angielskiego.", "a2"},
		{"polish no english -> none", "Bez angielskiego.", "none"},
		// Minimum-of-several and explicit none.
		{"range takes the floor", "English from intermediate to advanced.", "b1"},
		{"explicit no english -> none", "No English required for this role.", "none"},
		{"positive beats no-english phrase", "B2 English; no advanced English needed.", "b2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EnglishLevel(c.desc); got != c.want {
				t.Errorf("EnglishLevel(%q) = %q, want %q", c.desc, got, c.want)
			}
		})
	}
}

// EnglishLevel's near/spanNear matching is O(#keyword-matches × #phrase-matches) in the
// worst case, so a description with no upper bound could turn one job's derivation into
// a CPU-bound stall on the per-job ingest path (a scraped/SEO-padded posting repeating
// "english"/CEFR-like tokens without ever pairing them closely). A real signal placed
// past englishScanMaxRunes must be invisible to EnglishLevel — proof the input is
// actually bounded, not just that the matching is fast on realistic descriptions.
func TestEnglishLevelIgnoresContentPastTheLengthCap(t *testing.T) {
	padding := strings.Repeat("x ", englishScanMaxRunes) // far past the cap on its own
	beyondCap := padding + "Advanced English is required."
	if got := EnglishLevel(beyondCap); got != "" {
		t.Errorf("EnglishLevel returned %q for a signal placed past the length cap, want \"\" (the cap must apply before matching)", got)
	}

	// The same signal, comfortably inside the cap, is still found — the cap must not
	// suppress an ordinary, in-range description.
	withinCap := "Advanced English is required." + strings.Repeat("x", 100)
	if got := EnglishLevel(withinCap); got != "c1" {
		t.Errorf("EnglishLevel(%q) = %q, want %q", withinCap, got, "c1")
	}
}
