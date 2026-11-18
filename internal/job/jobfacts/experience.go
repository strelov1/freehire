package jobfacts

import (
	"regexp"
	"strconv"
	"strings"
)

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

// reNoExperience matches an explicit statement that the ROLE needs no prior
// experience. The requirement word must follow the noun with at most a copula
// between, because the same words scoped to an object say the opposite thing about
// the job: "no prior experience WITH Kubernetes is required" makes the tool
// optional while the role may still want a decade.
var reNoExperience = regexp.MustCompile(`\bno\s+(?:prior\s+|previous\s+)?experience\s+(?:is\s+)?(?:required|necessary|needed)\b`)

// reScopedToObject rejects a match whose object TRAILS the requirement word
// instead of leading it — "no prior experience is required WITH our CRM" scopes
// the statement exactly as "…with our CRM is required" does, and guarding only one
// word order let the other through. Go's regexp is RE2 and has no lookahead, so
// the tail is tested separately rather than folded into the pattern above.
//
// A trailing "for" is deliberately absent: "no experience necessary FOR this
// position" is the ordinary entry-level phrasing, where the object is the role
// itself rather than a tool.
var reScopedToObject = regexp.MustCompile(`^\s*(?:with|in|using)\b`)

// statesNoExperience reports whether the description carries an unscoped
// no-experience statement. A description may carry both — a scoped mention and a
// plain one — so every match is examined and one clean statement is enough.
func statesNoExperience(s string) bool {
	for _, loc := range reNoExperience.FindAllStringIndex(s, -1) {
		if !reScopedToObject.MatchString(s[loc[1]:]) {
			return true
		}
	}
	return false
}

// ExperienceYearsMin extracts the minimum required years of experience from the
// description, or nil when none is stated. It takes the smallest year figure
// mentioned next to a year word (the conservative floor) and ignores age phrases
// and out-of-range numbers. An explicit "no prior experience required" resolves
// to 0 — the entry-level population states its requirement in prose rather than
// as a figure, and reading digits alone left it indistinguishable from silence.
func ExperienceYearsMin(description string) *int {
	s := ageNoise.ReplaceAllString(strings.ToLower(hardRequirementText(description)), " ")
	best := -1
	// Zero is the smallest value the walk below can reach, so seeding it also settles
	// precedence: an explicit statement outranks any figure mentioned elsewhere, which
	// is the conservative floor already in force between competing figures.
	if statesNoExperience(s) {
		best = 0
	}
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

// englishRank orders the vocabulary lowest→highest so the minimum named level is
// returned — the conservative floor, matching "minimum English level required".
var englishRank = map[string]int{"a1": 1, "a2": 2, "b1": 3, "b2": 4, "c1": 5, "c2": 6, "native": 7}

// englishWindow is the byte gap allowed between an English keyword and a level word
// for the two to count as one signal. Sized for Russian (2 bytes/rune), so ~15 runes.
const englishWindow = 30

// truncateRunes clamps s to at most max runes, rune-boundary safe. Local rather than
// shared: this package stays free of any dependency beyond its own dict-only matching.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// near reports whether any match of phrase in s lies within englishWindow bytes of one
// of kws (English keyword match spans, precomputed once by the caller) without a
// sentence boundary between them.
func near(s string, kws [][]int, phrase *regexp.Regexp) bool {
	for _, p := range phrase.FindAllStringIndex(s, -1) {
		if spanNear(s, kws, p[0], p[1]) {
			return true
		}
	}
	return false
}
