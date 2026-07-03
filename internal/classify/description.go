package classify

import (
	"strings"

	"github.com/strelov1/freehire/internal/wordmatch"
)

// descriptionSeniorityPhrases maps a seniority grade to intent-anchored phrases
// that signal it in a job description, checked in precedence order (highest grade
// first, mirroring seniorityOrder). Unlike the title aliases — short, role-focused,
// safe as bare whole words — these are tuned for PRECISION in long prose: a bare
// "senior"/"lead"/"head of"/"staff" matches incidental text ("senior management",
// "lead the team", "report to the head of product", "our staff"), so every phrase
// is anchored to an unambiguous grade statement. Matching uses wordmatch, so a
// phrase only fires on word boundaries ("as a lead" never matches "as a leading
// provider", "technical lead" never matches "technical leadership"). The detector
// emits nothing on a weak signal and never infers a grade from a years figure.
var descriptionSeniorityPhrases = []struct {
	grade   string
	phrases []string
}{
	{"c_level", []string{
		"looking for a head of", "seeking a head of", "hiring a head of",
		"as head of", "as a head of", "vp of", "vice president of",
		"chief technology officer", "chief product officer", "chief executive officer",
		"c-level position", "c-level role",
	}},
	{"principal", []string{
		"principal engineer", "principal developer", "principal architect",
		"principal scientist", "principal consultant", "principal position", "principal role",
	}},
	{"staff", []string{
		"staff engineer", "staff developer", "staff software engineer",
		"staff position", "staff role",
	}},
	{"lead", []string{
		"lead role", "lead position", "looking for a lead", "hiring a lead",
		"seeking a lead", "tech lead", "technical lead", "team lead position", "team lead role",
	}},
	{"senior", []string{
		"senior-level", "senior position", "senior role",
		"looking for a senior", "hiring a senior", "seeking a senior",
	}},
	{"middle", []string{
		"mid-level", "intermediate-level",
	}},
	{"junior", []string{
		"entry-level", "entry level", "junior position", "junior role",
		"junior-level", "graduate position", "new grad",
	}},
	{"intern", []string{
		"internship", "intern position", "intern role", "trainee position", "trainee role",
	}},
}

// descriptionNonTechPhrases maps a confidently non-technical category to
// role-statement phrases that signal it in a job description, checked in order
// (first match wins). Like the seniority phrases — and unlike the bare title
// aliases — these are tuned for PRECISION in long prose: a bare "sales"/"support"
// matches incidental text ("work with our sales team", "our support engineers"),
// so every phrase is a full role noun. Anchors that could shadow a technical role
// are deliberately absent: there is no loose "hiring a sales" (it would fire on
// "sales engineer"), and the management phrases name only administrative roles —
// engineering/product/project/data manager forms are never listed, because those
// are technical categories, not `management`. Matching uses wordmatch on word
// boundaries. Values are the enrich.NonTechCategories members.
var descriptionNonTechPhrases = []struct {
	category string
	phrases  []string
}{
	{"sales", []string{
		"sales representative", "sales development representative",
		"account executive", "business development representative",
		"inside sales representative",
	}},
	{"marketing", []string{
		"marketing manager", "marketing specialist", "content marketing manager",
		"digital marketing manager", "growth marketing manager",
		"social media manager", "brand manager", "seo specialist",
	}},
	{"support", []string{
		"customer support representative", "customer support specialist",
		"customer success manager", "customer success specialist",
		"support representative", "help desk technician", "help desk specialist",
	}},
	{"management", []string{
		"office manager", "operations manager", "general manager", "hr manager",
	}},
}

// NonTechFromDescription derives a confidently non-technical category
// (enrich.NonTechCategories) from a job description's prose, returning "" when no
// anchored non-technical role statement is present. It resolves ONLY non-technical
// categories — a technical role yields "" — and never guesses: it is the
// lowest-priority category source (after the structured signal and the title
// dictionary), so it only fills a category the title left empty, feeding the
// AI-enrichment cost gate without risking a technical job's enrichment.
func NonTechFromDescription(desc string) string {
	lower := strings.ToLower(desc)
	for _, c := range descriptionNonTechPhrases {
		for _, p := range c.phrases {
			if wordmatch.Contains(lower, p, wordmatch.UnicodeBoundary) {
				return c.category
			}
		}
	}
	return ""
}

// SeniorityFromDescription derives a seniority grade from a job description's prose,
// returning "" when no anchored grade statement is present. It is the lower-priority
// seniority source (after the title dictionary), so it only fills a value the title
// left empty. Values are from enrich.SeniorityValues.
func SeniorityFromDescription(desc string) string {
	lower := strings.ToLower(desc)
	for _, g := range descriptionSeniorityPhrases {
		for _, p := range g.phrases {
			if wordmatch.Contains(lower, p, wordmatch.UnicodeBoundary) {
				return g.grade
			}
		}
	}
	return ""
}
