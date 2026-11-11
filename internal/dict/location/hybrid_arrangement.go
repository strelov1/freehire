package location

import (
	"regexp"
	"strings"
)

// HybridArrangementStated reads explicit work-model statements
// that contradict an ATS remote flag. Home-working perks and incidental technology
// terms do not establish that contradiction. Conflicting fully remote prose leaves
// the structured signal intact rather than choosing between two arrangements.
func HybridArrangementStated(desc string) bool {
	text := dropAccents(foldMarkup(desc))
	remoteGuards := phraseGuards{
		negations:      []string{"not ", "no ", "never ", "experience ", "previous ", "prior "},
		refusals:       []string{"not offered", "not available", "not possible", "not an option"},
		negationWindow: 60,
	}
	// A later contrast or transition can state the permanent remote arrangement.
	remoteText := remoteArrangementClauses.Replace(text)
	for _, p := range []string{
		"fully remote", "fully-remote", "100% remote", "100 % remote",
	} {
		if phraseMatches(remoteText, p) && guardedPhraseMatches(remoteText, p, "remote", remoteGuards) {
			return false
		}
	}
	if spanishWorkMode(desc, text) == "remote" {
		return false
	}
	english := phraseGuards{
		negations:      []string{"not ", "no ", "never ", "experience ", "previous ", "prior "},
		refusals:       []string{"not available", "not offered", "not possible", "not an option", "preferred", "experience"},
		negationWindow: 60,
	}
	for _, pattern := range []*regexp.Regexp{englishHybridStatements, roleHybridContrast} {
		for _, match := range pattern.FindAllStringIndex(text, -1) {
			if !english.withdraws(text, match[0], match[1]) && !optionalArrangementAt(text, match[0], match[1]) {
				return true
			}
		}
	}
	for _, p := range []string{
		"modelo hibrido", "modelo de trabajo hibrido", "modalidad hibrida", "modalidad hibrido",
		"modalidad de trabajo hibrida", "modalidad de trabajo hibrido",
		"modalidad de trabajo: hibrida", "modalidad de trabajo: hibrido",
		"esquema de trabajo hibrido", "esquema hibrido",
	} {
		if guardedPhraseMatches(text, p, "hybrid", spanishGuards) {
			return true
		}
	}
	for _, match := range mandatoryOfficeWeek.FindAllStringIndex(text, -1) {
		if !english.withdraws(text, match[0], match[1]) && !optionalArrangementAt(text, match[0], match[1]) {
			return true
		}
	}
	return hybridLocationHeader(text, english)
}

var remoteArrangementClauses = strings.NewReplacer(" but ", "; ", " then ", "; ")

var englishHybridStatements = regexp.MustCompile(
	`\b(?:hybrid role|hybrid working|hybrid work arrangement|hybrid work model|hybrid schedule|hybrid position` +
		`|hybrid session environment|hybrid opportunity combines in-person` +
		`|work from home regularly but must be able to commute)\b`)

// This is an explicit statement about the role, rather than a contrast inside
// an experience requirement such as "managing office-based but hybrid working teams".
var roleHybridContrast = regexp.MustCompile(
	`\b(?:this|the) (?:role|position|job) is not (?:fully|100%|100 %) remote,? but (?:is )?(?:a )?hybrid (?:role|position|job)\b`)

var (
	optionalArrangementBefore = regexp.MustCompile(`\boptional(?:ly)?\s*$`)
	optionalArrangementAfter  = regexp.MustCompile(`^\s*(?:(?:work )?(?:arrangements?|models?|schedules?|options?)\s+)?(?:(?:is|are|remains?|may be|can be)\s+)?(?:(?:entirely|completely|totally|fully|purely|wholly|always|strictly)\s+){0,2}optional(?:ly)?\b`)
	conditionalArrangement    = regexp.MustCompile(`^\s*(?:if|when) you (?:choose|select|opt for) (?:the |an? )?optional hybrid\b`)
)

// Optional qualifies the work arrangement only when attached to its statement;
// unrelated benefits such as optional dental insurance do not withdraw it.
func optionalArrangementAt(text string, start, end int) bool {
	return optionalArrangementBefore.MatchString(text[:start]) ||
		optionalArrangementAfter.MatchString(text[end:]) || conditionalArrangement.MatchString(text[end:])
}

// The employer explicitly requires a partial office week; a generic corporate
// hybrid benefit or occasional client travel does not establish that obligation.
var mandatoryOfficeWeek = regexp.MustCompile(
	`\b(?:working from the office in [^.!?]{1,160}, [1-4] days per week, is required` +
		`|(?:you must|you are required to|this role requires you to) work (?:in|from) the office (?:[1-4]|one|two|three|four) days? (?:per|a) week` +
		`|hybrid work with (?:[1-4]|one|two|three|four) days? in the office)\b`)

// A parenthesized hybrid marker directly after a known city is an arrangement
// header, not a technical reference such as "infrastructure (hybrid)". A labeled
// location may name countries, states or several cities instead.
func hybridLocationHeader(text string, guards phraseGuards) bool {
	for offset := 0; offset < len(text); {
		at := strings.Index(text[offset:], "(hybrid)")
		if at < 0 {
			return false
		}
		at += offset
		offset = at + len("(hybrid)")
		if optionalArrangementAt(text, at, offset) {
			continue
		}
		before := text[max(0, at-180):at]
		labeled := strings.LastIndex(before, "location:")
		if labeled >= 0 {
			headerStart := at - len(before) + labeled + len("location:")
			if !guards.withdraws(text[headerStart:], at-headerStart, offset-headerStart) &&
				hybridPlaceList(before[labeled+len("location:"):]) {
				return true
			}
		} else if at < 400 && !guards.withdraws(text, at, offset) {
			if split := strings.LastIndexAny(before, "/,:;\n"); split >= 0 {
				before = before[split+1:]
			}
			if len(Parse(strings.TrimSpace(before)).Cities) > 0 {
				return true
			}
		}
	}
	return false
}

func hybridPlaceList(text string) bool {
	resolved := false
	countries := Parse(text).Countries
	for _, part := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == '/' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		place := ParseIn(part, countries)
		if len(place.Cities) == 0 && len(place.Countries) == 0 && len(place.Regions) == 0 {
			return false
		}
		resolved = true
	}
	return resolved
}
