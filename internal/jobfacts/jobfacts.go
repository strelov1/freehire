// Package jobfacts derives a job's employment type, education level, and minimum
// required experience deterministically from its title and description text. Like
// internal/classify and internal/location it is a curated matcher, not a model:
// it resolves explicit signals and emits nothing ("" / nil) for what it cannot
// resolve — it never guesses. Canonical enum values are members of the controlled
// vocabularies the enrichment contract defines (enrich.EmploymentTypeValues /
// enrich.EducationLevelValues).
package jobfacts

import (
	"regexp"
	"strconv"
	"strings"
)

// Employment-type matchers, checked in precedence order: a "full-time internship"
// is an internship, a part-time contract is part-time, etc. "temporary" / "fixed
// term" map to contract (the closest vocabulary member). Bare \bintern\b is safe —
// the boundary keeps it out of "internal"/"international". The contract matcher also
// covers the unambiguous US-market shorthands for an independent contractor: 1099
// (the tax form) and "corp-to-corp". "consultant" is deliberately excluded — it is as
// often a full-time title as a contract arrangement.
//
// b2b/c2c also denote an independent-contractor arrangement in some markets, but the
// bare tokens collide with ubiquitous business-model prose ("B2B SaaS", "C2C
// marketplace") — so, like the bare "ms"/"bs" degree abbreviations below, they favour
// precision: reContractShorthand matches them only when an employment-context word
// sits right after ("C2C candidates", "B2B contract", "C2C only"), never standalone.
var (
	reInternship        = regexp.MustCompile(`\b(internship|intern|co-?op|working student|praktikum|werkstudent)\b`)
	rePartTime          = regexp.MustCompile(`\bpart[\s-]?time\b`)
	reContract          = regexp.MustCompile(`\b(contractor|contract|freelancer|freelance|fixed[\s-]?term|temporary|1099|corp[\s-]?to[\s-]?corp)\b`)
	reContractShorthand = regexp.MustCompile(`\b(b2b|c2c)\b[\s:/.-]*(only|contract|contractor|candidates?|welcome|accepted|basis|engagement|employment|position|arrangement|w2)\b`)
	reFullTime          = regexp.MustCompile(`\b(full[\s-]?time|permanent)\b`)
)

// EmploymentType resolves the work arrangement from the title and description,
// returning one of enrich.EmploymentTypeValues or "" when nothing is stated. It
// never assumes full-time for an unstated posting.
func EmploymentType(title, description string) string {
	s := strings.ToLower(title + "\n" + description)
	switch {
	case reInternship.MatchString(s):
		return "internship"
	case rePartTime.MatchString(s):
		return "part_time"
	case reContract.MatchString(s) || reContractShorthand.MatchString(s):
		return "contract"
	case reFullTime.MatchString(s):
		return "full_time"
	}
	return ""
}

// Education-level matchers, highest degree first so "Master's or PhD" resolves to
// the ceiling actually named. "none" is emitted only on an explicit negation, and
// only when no positive degree is named (see EducationLevel).
// These favour precision over recall (it is a faceted field — a wrong value is worse
// than a missing one): only unambiguous degree forms match. Bare single-letter
// abbreviations are deliberately excluded — "ms"/"m.s" collide with "MS Office"/
// "MS SQL" and "bs"/"b.s" with everyday text — and bare "master" is excluded because
// "scrum master" is not a degree. The "'s" possessive, an explicit "<level> degree",
// or the -Sc/MBA/PhD tokens are required instead.
var (
	rePhD      = regexp.MustCompile(`\b(ph\.?\s?d|phd|doctorate|doctoral)\b`)
	reMaster   = regexp.MustCompile(`\b(master'?s|master degree|m\.?sc|mba|graduate degree)\b`)
	reBachelor = regexp.MustCompile(`\b(bachelor'?s|bachelor degree|b\.?sc|undergraduate degree)\b`)
	reNoDegree = regexp.MustCompile(`\b(no (?:degree|diploma)|degree not required|without a degree|no degree required)\b`)
)

// EducationLevel resolves the required education from the description, returning
// one of enrich.EducationLevelValues or "" when nothing is stated. A named degree
// wins over a "no degree" phrase (a posting that says "Bachelor's or equivalent;
// no degree required for exceptional candidates" still has a degree signal).
func EducationLevel(description string) string {
	s := strings.ToLower(description)
	switch {
	case rePhD.MatchString(s):
		return "phd"
	case reMaster.MatchString(s):
		return "master"
	case reBachelor.MatchString(s):
		return "bachelor"
	case reNoDegree.MatchString(s):
		return "none"
	}
	return ""
}

// experienceCap bounds a parsed years value; anything larger is hyperbole or a
// mis-parse (a stray age/date), not a real experience requirement.
const experienceCap = 50

// ageNoise strips "years of age" / "years old" so an age requirement is not read
// as an experience requirement.
var ageNoise = regexp.MustCompile(`\d{1,2}\s*years?\s*(?:of age|old)`)

// reRangeYears captures the low end of an "N-M years" range; rePlainYears captures
// "N years" / "N+ years" / "N yrs". Both require the number to sit next to a
// year word, so unrelated digits are ignored.
var (
	reRangeYears = regexp.MustCompile(`\b(\d{1,2})\s*(?:-|–|to)\s*\d{1,2}\s*(?:years?|yrs?)`)
	rePlainYears = regexp.MustCompile(`\b(\d{1,2})\s*\+?\s*(?:years?|yrs?)`)
)

// ExperienceYearsMin extracts the minimum required years of experience from the
// description, or nil when none is stated. It takes the smallest year figure
// mentioned next to a year word (the conservative floor) and ignores age phrases
// and out-of-range numbers.
func ExperienceYearsMin(description string) *int {
	s := ageNoise.ReplaceAllString(strings.ToLower(description), " ")
	best := -1
	consider := func(re *regexp.Regexp) {
		for _, m := range re.FindAllStringSubmatch(s, -1) {
			n, err := strconv.Atoi(m[1])
			if err != nil || n < 0 || n > experienceCap {
				continue
			}
			if best == -1 || n < best {
				best = n
			}
		}
	}
	consider(reRangeYears)
	consider(rePlainYears)
	if best == -1 {
		return nil
	}
	return &best
}

// English-level detection. Precision-first like the matchers above: it resolves an
// explicit CEFR code or a well-known level phrase (EN + RU, since the Telegram
// sources are Russian-heavy) and emits "" when nothing is stated. Every signal must
// sit near an English keyword, so a bare "B2"/"advanced"/"native" is not misread out
// of context ("B2B SaaS", "advanced degree", "native macOS app"). Values are members
// of enrich.EnglishLevelValues.
var (
	// reEnglishKw gates the whole parse and anchors every phrase: english_level is
	// about English, so a description that never names it yields nothing.
	reEnglishKw = regexp.MustCompile(`english|английск`)
	// A CEFR code counts only adjacent (either order) to an English keyword.
	reCEFRForward = regexp.MustCompile(`(?:english|английск\w*)[^.\n]{0,20}\b([abc][12])\b`)
	reCEFRBack    = regexp.MustCompile(`\b([abc][12])\b[^.\n]{0,20}(?:english|английск\w*)`)

	// Level phrases (checked for English proximity via near). The intermediate family
	// carries its prefix so "upper-intermediate"→b2 and "pre-intermediate"→a2 resolve
	// without a lookbehind (RE2 has none); the Russian "средн" family mirrors it.
	reNative     = regexp.MustCompile(`\bnative\b|родн\w*|носител\w*`)
	reFluentAdv  = regexp.MustCompile(`fluen\w*|\badvanced\b|свободн\w*|продвинут\w*`)
	reInterFam   = regexp.MustCompile(`\b(upper[\s-]?|pre[\s-]?)?intermediate\b`)
	reRuMidFam   = regexp.MustCompile(`(выше\s+)?средн\w*`)
	reConvers    = regexp.MustCompile(`\bconversational\b|разговорн\w*`)
	reElementary = regexp.MustCompile(`\belementary\b|\bbeginner\b|начальн\w*`)
	reBasic      = regexp.MustCompile(`\bbasic\b|базов\w*`)
	reNoEnglish  = regexp.MustCompile(`no english|english (?:is )?not required|without english|без английск\w*`)
)

// englishRank orders the vocabulary lowest→highest so the minimum named level is
// returned — the conservative floor, matching "minimum English level required".
var englishRank = map[string]int{"a1": 1, "a2": 2, "b1": 3, "b2": 4, "c1": 5, "c2": 6, "native": 7}

// englishWindow is the byte gap allowed between an English keyword and a level word
// for the two to count as one signal. Sized for Russian (2 bytes/rune), so ~15 runes.
const englishWindow = 30

// EnglishLevel resolves the required English level from the description, returning
// one of enrich.EnglishLevelValues or "" when nothing is stated. When several levels
// are named it returns the lowest (the minimum requirement); an explicit "no English"
// phrase resolves to "none" only when no positive level is present.
func EnglishLevel(description string) string {
	s := strings.ToLower(description)
	if !reEnglishKw.MatchString(s) {
		return ""
	}

	levels := map[string]bool{}
	for _, m := range reCEFRForward.FindAllStringSubmatch(s, -1) {
		levels[m[1]] = true
	}
	for _, m := range reCEFRBack.FindAllStringSubmatch(s, -1) {
		levels[m[1]] = true
	}
	if near(s, reEnglishKw, reNative) {
		levels["native"] = true
	}
	if near(s, reEnglishKw, reFluentAdv) {
		levels["c1"] = true
	}
	if near(s, reEnglishKw, reConvers) {
		levels["b1"] = true
	}
	if near(s, reEnglishKw, reElementary) {
		levels["a1"] = true
	}
	if near(s, reEnglishKw, reBasic) {
		levels["a2"] = true
	}
	for _, m := range reInterFam.FindAllStringSubmatchIndex(s, -1) {
		if !spanNearEnglish(s, m[0], m[1]) {
			continue
		}
		switch {
		case m[2] < 0: // no prefix group — plain "intermediate"
			levels["b1"] = true
		case strings.HasPrefix(s[m[2]:m[3]], "upper"):
			levels["b2"] = true
		case strings.HasPrefix(s[m[2]:m[3]], "pre"):
			levels["a2"] = true
		default:
			levels["b1"] = true
		}
	}
	for _, m := range reRuMidFam.FindAllStringSubmatchIndex(s, -1) {
		if !spanNearEnglish(s, m[0], m[1]) {
			continue
		}
		if m[2] >= 0 { // "выше средн..." — above intermediate
			levels["b2"] = true
		} else {
			levels["b1"] = true
		}
	}

	if len(levels) == 0 {
		if reNoEnglish.MatchString(s) {
			return "none"
		}
		return ""
	}
	best := ""
	for lv := range levels {
		if best == "" || englishRank[lv] < englishRank[best] {
			best = lv
		}
	}
	return best
}

// near reports whether any match of kw and any match of phrase in s lie within
// englishWindow bytes of each other without a sentence boundary between them.
func near(s string, kw, phrase *regexp.Regexp) bool {
	kws := kw.FindAllStringIndex(s, -1)
	for _, p := range phrase.FindAllStringIndex(s, -1) {
		if spanNear(s, kws, p[0], p[1]) {
			return true
		}
	}
	return false
}

// spanNearEnglish reports whether the [start,end) span sits near an English keyword.
func spanNearEnglish(s string, start, end int) bool {
	return spanNear(s, reEnglishKw.FindAllStringIndex(s, -1), start, end)
}

// spanNear reports whether [start,end) is within englishWindow bytes of any span,
// with no sentence boundary (. or newline) in the gap — so a level word and an
// English keyword in different sentences ("native iOS apps. English docs") don't
// bind. An overlap always counts.
func spanNear(s string, spans [][]int, start, end int) bool {
	for _, m := range spans {
		var lo, hi int
		switch {
		case start >= m[1]:
			lo, hi = m[1], start
		case m[0] >= end:
			lo, hi = end, m[0]
		default:
			return true // overlap
		}
		if hi-lo <= englishWindow && !strings.ContainsAny(s[lo:hi], ".\n") {
			return true
		}
	}
	return false
}
