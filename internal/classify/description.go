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
