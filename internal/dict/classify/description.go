package classify

import (
	"strings"

	"github.com/strelov1/freehire/internal/dict/wordmatch"
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
// "sales engineer"), and the manager phrases name only back-office roles —
// engineering/product/project/data manager forms are never listed, because those
// are technical categories. Each manager phrase resolves the function it names, the
// way the title dictionary files it ("Office Manager" and "Operations Manager" are
// `operations`, "HR Manager" `hr`); "general manager" names no function and is not
// here, now that there is no management category to file it under. Matching uses
// wordmatch on word boundaries. Values are the vocab.NonTechCategories members.
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
		"support representative",
		// No help desk phrase: the help desk is `it_support`, a technical category,
		// and this detector resolves only non-technical ones.
	}},
	{"operations", []string{"office manager", "operations manager"}},
	{"hr", []string{"hr manager"}},
}

// NonTechFromDescription derives a confidently non-technical category
// (vocab.NonTechCategories) from a job description's prose, returning "" when no
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
// left empty. Values are from vocab.SeniorityValues. An entry grade a mentoring verb
// governs is passed over (notMentored).
func SeniorityFromDescription(desc string) string {
	lower := strings.ToLower(desc)
	for _, g := range descriptionSeniorityPhrases {
		boundary := wordmatch.UnicodeBoundary
		if g.grade == "junior" || g.grade == "intern" {
			boundary = notMentored
		}
		for _, p := range g.phrases {
			if wordmatch.Contains(lower, p, boundary) {
				return g.grade
			}
		}
	}
	return ""
}

// mentorVerbs are the words that make an entry grade the people a role looks after
// rather than the role itself: "serves as a guide and mentor for junior-level software
// development engineers" is a prod "Backend Software Engineer 3" posting, served as
// junior off that line. Every senior posting that mentors says so, so the guard reads
// only the two entry grades — a higher grade's phrases are not what a role mentors.
var mentorVerbs = []string{
	// No "mentorship": the noun is a benefit the role receives ("we offer mentorship in
	// this entry-level position"), not a role it plays.
	"mentor", "mentors", "mentoring",
	"coach", "coaches", "coaching",
	"guide", "guides", "guiding",
	"supervise", "supervises", "supervising",
	"teach", "teaches", "teaching",
}

// mentorWindow is how far before a grade phrase a mentoring verb still governs it.
const mentorWindow = 40

// clauseBreaks end the stretch a mentoring verb governs: sentence punctuation, a line
// break, and the block-level tags a list item or paragraph opens and closes with. An
// inline tag ("mentor <strong>junior-level</strong> engineers") is not one.
var clauseBreaks = []string{".", ";", "!", "?", "\n", "<li", "</li", "<p", "</p", "<br", "<div", "</div"}

// notMentored is wordmatch.UnicodeBoundary that also refuses a match a mentoring verb
// governs: one within mentorWindow bytes before it, with no clauseBreaks between them.
func notMentored(s string, start, end int) bool {
	if !wordmatch.UnicodeBoundary(s, start, end) {
		return false
	}
	lo := max(0, start-mentorWindow)
	for lo > 0 && s[lo-1] != ' ' { // never start the window inside a word
		lo--
	}
	before := s[lo:start]
	for _, b := range clauseBreaks {
		if i := strings.LastIndex(before, b); i >= 0 {
			before = before[i+len(b):]
		}
	}
	for _, v := range mentorVerbs {
		if wordmatch.Contains(before, v, wordmatch.UnicodeBoundary) {
			return false
		}
	}
	return true
}
