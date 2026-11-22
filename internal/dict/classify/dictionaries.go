package classify

import (
	"regexp"
	"slices"
)

// aliasEntry pairs a lowercase title alias with its enrich-vocabulary canonical.
// The ordered table is the single source of truth: its order encodes precedence
// (the most specific / highest-rank alias first, so a title carrying several terms
// resolves the stronger one) and the canonical is read straight off the entry, so
// the alias set and its mapping cannot drift out of parity. Aliases are lowercase;
// multi-word and hyphenated forms are explicit.
type aliasEntry struct {
	alias     string
	canonical string
}

// gradeBlindPhrases are role names that CONTAIN a seniority word without stating a grade: "Member of Technical Staff" is the generic IC title at Oracle/xAI/Pure Storage, not the staff grade, and "Lead Generation" is a marketing function. They are cut before the seniority match so the remaining words state the real grade ("Senior Member of Technical Staff" is senior; a bare MTS has none). Without the mask "staff" outranks "senior" and SMTS resolved to staff.
//
// Only phrases that shadow a seniorityTable alias belong here: MTS/SMTS/DMTS contain no grade word. The longer form is listed first so it is cut whole.
var gradeBlindPhrases = []string{
	"member of the technical staff",
	"member of technical staff",
	"lead generation",
	// A model-training stage ("Member of Technical Staff, Mid-training"): the hyphen is a word boundary, so the bare "mid" alias would read it as middle grade.
	"mid-training",
	// A region, not a grade: "Middle East" would otherwise grade the posting middle.
	"middle east",
	// The hyphenated spellings of the two phrases above: a hyphen is a word boundary, so the spaced forms cannot mask them ("... (Middle-East & Africa)").
	"middle-east",
	"lead-generation",
	// Corporate titles a rung below Vice President — an AVP leads a desk at a bank or an
	// account at a services firm — not executives. They name no grade of their own.
	"assistant vice president",
	"associate vice president",
	// "Second new graduate" — someone a year or three out of university — is written as a
	// welcome ("第二新卒歓迎") on postings of every grade, so it states none. Cut, it also stops
	// the 新卒 inside it from reading as a new-graduate hire.
	"第二新卒",
}

// earlyCareersDeskPatterns are the grade-blind phrases of the early-careers vocabulary, stated as a SHAPE: staff who run graduate programmes and placements (university offices, graduate recruiting, placement officers) carry "graduate"/"placement" beside the desk they name ("Graduate Recruiting Manager", "Work Placement Officer"). Masked, they state no grade.
//
// The first shape is the word FOLLOWED by the desk (optionally through "program(me)"): "Graduate Program Manager". The second is a desk in charge OF "graduate programs": "Director of Graduate Programs". A hire's own title never has that shape ("Graduate Project Manager" puts its craft between the two), and a real grade elsewhere survives the cut ("Senior Graduate Recruiter" is senior).
var earlyCareersDeskPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(?:graduate|(?:(?:industrial|industry|work|summer|student|sandwich) )?placement(?: year| student)?)(?: program(?:me)?s?)? (?:co-?ordinator|manager|director|officer|advis[eo]r|administrator|recruiter|recruiting|recruitment|admissions|school|studies|office|medical education|student cent(?:er|re))\b`),
	regexp.MustCompile(`\b(?:director|manager|co-?ordinator|officer|head|dean|advis[eo]r|administrator)(?: of| for|,)? graduate program(?:me)?s\b`),
}

// categoryNone is the sentinel canonical for a "blind" alias: a phrase that CONTAINS a categoryTable alias while naming no category of its own. "Software Design Engineer" is software engineering ("design" qualifies what is engineered) and the vocabulary has no value for a software generalist. Emitting nothing is the honest answer; is_tech comes from the tech-title detector.
//
// A table entry rather than a pre-match mask on purpose: cutting the span would expose whatever alias sits further down ("Software Design Engineer - Sales Tools" would resolve to `sales`), and cutting is boundary-blind where every matcher here is boundary-aware. As an entry it wins the first-match walk and matchCategory translates it to "".
//
// Only phrases with no better category belong here; otherwise route the title ("cloud design engineer" → devops, "design engineer in test" → qa). Keep phrases narrow: a broad "systems design engineer" would blank the category of every "HVAC/Mechanical Systems Design Engineer".
const categoryNone = "-"

// seniorityTable lists seniority aliases in precedence order (most specific /
// highest rank first), each paired with its vocab.SeniorityValues canonical.
var seniorityTable = []aliasEntry{
	{"head of", "c_level"},
	{"chief", "c_level"},
	{"cto", "c_level"},
	{"cpo", "c_level"},
	{"ceo", "c_level"},
	{"vp", "c_level"},
	{"vice president", "c_level"},
	{"директор", "c_level"},
	{"руководитель", "c_level"},
	{"principal", "principal"},
	{"staff", "staff"},
	{"lead", "lead"},
	{"ведущий", "lead"},
	{"тимлид", "lead"},
	{"teamlead", "lead"},
	{"team lead", "lead"},
	{"senior", "senior"},
	// "sr" already covers the dotted "Sr." form: '.' is a non-word boundary, so a
	// separate "sr." alias could never match anything "sr" does not.
	{"sr", "senior"},
	// Latin America's "semi senior" (Ssr) is the rung between junior and senior. The
	// spelled-out forms contain "senior", so matchSeniority folds them to "ssr" first;
	// listed after "sr", a range like "Ssr/Sr" keeps its upper bound.
	{"ssr", "middle"},
	{"старший", "senior"},
	{"синьор", "senior"},
	{"сеньор", "senior"},
	{"middle", "middle"},
	{"mid", "middle"},
	{"mid-level", "middle"},
	{"mid level", "middle"},
	{"средний", "middle"},
	{"мидл", "middle"},
	{"junior", "junior"},
	// Like "sr", the bare "jr" covers "Jr." — a dotted alias would be dead.
	{"jr", "junior"},
	{"младший", "junior"},
	{"джуниор", "junior"},
	{"джун", "junior"},
	{"intern", "intern"},
	{"internship", "intern"},
	// A trainee HIRED as one is an employee on an entry grade, not an intern: India's
	// Graduate Engineer Trainee and the Management Trainee are full-time campus hires, as
	// a UK graduate trainee is. Listed before the bare word, which keeps its reading.
	{"graduate engineer trainee", "junior"},
	{"graduate trainee", "junior"},
	{"management trainee", "junior"},
	{"engineer trainee", "junior"},
	{"trainee engineer", "junior"},
	{"trainee", "intern"},
	{"стажёр", "intern"},
	{"стажер", "intern"},
	{"стажировка", "intern"},
	// UK early-careers vocabulary. A placement there is a student's paid year or summer inside the employer, which is what an internship is. Every alias is a PHRASE: a bare "placement" names the recruiter's job ("Placement Coordinator") and the marketing one ("Product Placement"), and a bare "undergraduate" names most of a university's admissions and faculty titles ("The Undergraduate School - Adjunct Faculty").
	{"industrial placement", "intern"},
	{"industry placement", "intern"},
	{"placement year", "intern"},
	{"year placement", "intern"},
	{"year in industry", "intern"},
	{"placement student", "intern"},
	{"student placement", "intern"},
	{"summer placement", "intern"},
	{"undergraduate placement", "intern"},
	{"work placement", "intern"},
	// "12 Month Placement" / "12-month placement": the duration is what makes it a
	// student's year out rather than a recruiter placing a candidate.
	{"month placement", "intern"},
	{"vacation scheme", "intern"},
	{"summer analyst", "intern"},
	// "Graduate" is the entry-level hire (a graduate scheme, "New Graduate RN"), as the description matcher reads "graduate position". It is LAST, below every intern alias, as the weakest evidence in the table: "Graduate Summer Internship" is an internship, and any explicit grade in the title outranks it. Staff desks that merely contain the word are masked by earlyCareersDeskPatterns.
	{"graduate", "junior"},
}

// categoryTable lists category aliases in precedence order — multi-word and more
// specific terms first, so "data analyst" wins over a bare "data" and "fullstack"
// is not shadowed by "backend"/"frontend" — each paired with its
// vocab.CategoryValues canonical.
//
// The Japanese block (japanese.go) leads it: a Japanese title routinely carries Latin
// fragments ("AWS", "SE", "PM") that the aliases below would otherwise claim first.
// The Chinese block (chinese.go) closes it, behind everything a title could already
// resolve to.
var categoryTable = slices.Concat(japaneseCategoryTable, coreCategoryTable, japaneseTrailCategoryTable, chineseCategoryTable, koreanCategoryTable)

var coreCategoryTable = slices.Concat(
	coreCategoryTable1,
	coreCategoryTable2,
	coreCategoryTable3,
	coreCategoryTable4,
	coreCategoryTable5,
	coreCategoryTable6,
	coreCategoryTable7,
	coreCategoryTable8,
	coreCategoryTable9,
	coreCategoryTable10,
	coreCategoryTable11,
	coreCategoryTable12,
	coreCategoryTable13,
	coreCategoryTable14,
	coreCategoryTable15,
	coreCategoryTable16,
	coreCategoryTable17,
)
