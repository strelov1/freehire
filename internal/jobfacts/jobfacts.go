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
// the boundary keeps it out of "internal"/"international".
var (
	reInternship = regexp.MustCompile(`\b(internship|intern|co-?op|working student|praktikum|werkstudent)\b`)
	rePartTime   = regexp.MustCompile(`\bpart[\s-]?time\b`)
	reContract   = regexp.MustCompile(`\b(contractor|contract|freelancer|freelance|fixed[\s-]?term|temporary|b2b)\b`)
	reFullTime   = regexp.MustCompile(`\b(full[\s-]?time|permanent)\b`)
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
	case reContract.MatchString(s):
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
