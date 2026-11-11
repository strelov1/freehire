package location

import (
	"regexp"
	"strings"
)

// HybridArrangementStated reads explicit work-model statements
// that contradict an ATS remote flag. Home-working perks and incidental technology
// terms do not establish that contradiction. Conflicting explicit remote prose leaves
// the structured signal intact rather than choosing between two arrangements.
func HybridArrangementStated(desc string) bool {
	text := dropAccents(foldMarkup(desc))
	remoteGuards := phraseGuards{
		negations:      []string{"not ", "no ", "never ", "experience ", "previous ", "prior "},
		refusals:       []string{"not offered", "not available", "not possible", "not an option"},
		negationWindow: 60,
	}
	for _, p := range []string{
		"fully remote", "fully-remote", "100% remote", "100 % remote",
		"this position is remote", "position is remote", "role is remote",
		"remote position", "remote role",
	} {
		if phraseMatches(text, p) && guardedPhraseMatches(text, p, "remote", remoteGuards) {
			return false
		}
	}
	if spanishWorkMode(desc, text) == "remote" {
		return false
	}
	english := phraseGuards{
		negations:      []string{"not ", "no ", "experience ", "previous ", "prior ", "optional ", "optionally "},
		refusals:       []string{"not available", "not offered", "preferred", "experience", "optional"},
		negationWindow: 60,
	}
	for _, p := range []string{
		"hybrid role", "hybrid working", "hybrid work arrangement", "hybrid work model",
		"hybrid schedule", "hybrid position",
		"hybrid session environment",
		"hybrid opportunity combines in-person",
		"work from home regularly but must be able to commute",
	} {
		if guardedPhraseMatches(text, p, "hybrid", english) {
			return true
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
		if !english.withdraws(text, match[0], match[1]) {
			return true
		}
	}
	return hybridLocationHeader(text, english)
}

// The employer explicitly requires a partial office week; a generic corporate
// hybrid benefit or occasional client travel does not establish that obligation.
var mandatoryOfficeWeek = regexp.MustCompile(`\bworking from the office in [^.!?]{1,160}, [1-4] days per week, is required\b`)

// A parenthesized hybrid marker directly after a known city is an arrangement
// header, not a technical reference such as "infrastructure (hybrid)". A labeled
// location may name several cities or end with a state instead.
func hybridLocationHeader(text string, guards phraseGuards) bool {
	for offset := 0; offset < len(text); {
		at := strings.Index(text[offset:], "(hybrid)")
		if at < 0 {
			return false
		}
		at += offset
		offset = at + len("(hybrid)")
		if guards.withdraws(text, at, offset) {
			continue
		}
		before := text[max(0, at-180):at]
		labeled := strings.LastIndex(before, "location:")
		if labeled >= 0 {
			if hybridPlaceList(before[labeled+len("location:"):]) {
				return true
			}
		} else if at < 400 {
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
	city := false
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
		city = city || len(place.Cities) > 0
	}
	return city
}
