package location

import (
	"regexp"
	"strings"
)

// Indian postings state the arrangement in a register of their own: "WFO" (work from
// office) for an office job, "WFH" for a remote one, and a hybrid one by counting office
// days — "Hybrid (3 days office)", "3 days WFO". Measured 2026-10-25 over 48,568 Indian
// descriptions: "work from office" appears in 852 and "WFO" in 225, and the English
// phrase list read none of them.
//
// This list is read only when none of the English, German and Dutch ones matched, so it
// never changes an answer they gave. It is matched on folded markup (foldMarkup).

// indianWorkModeTriggers gate the fold: a description mentioning none of them cannot match.
var indianWorkModeTriggers = []string{"wfo", "wfh", "office", "work from home"}

// officeDays matches a count of office (or home) days and what follows it, so the count
// decides the mode: one to four days in the office is hybrid, five or six is the whole
// working week. A count of days a MONTH is not a weekly pattern and is left to hybrid by
// the first group only when stated per week or bare.
var officeDays = regexp.MustCompile(`\b([1-6]|one|two|three|four|five|six)\s*-?\s*days?\s*(?:a week|per week|/\s*week|in a week|weekly)?\s*(?:-\s*)?(wfo|wfh|work(?:ing)? from (?:the )?(?:office|home)|from (?:the )?office|(?:in|at) (?:the )?office|office)\b`)

// officeDaysBefore matches the count written after the marker: "WFO - 5 days".
var officeDaysBefore = regexp.MustCompile(`\b(wfo|work from office)\s*(?:-|:|–)?\s*\(?([1-6]|five|six)\s*days?\b`)

// indianRemotePhrases are the anchored WFH statements. A bare "WFH" is not one: it names
// a benefit ("occasional WFH"), a shift ("a rotational WFH shift") or the staff an HR
// role manages ("remote/WFH employees") as often as the arrangement.
var indianRemotePhrases = []string{
	"permanent wfh", "fully wfh", "100% wfh", "complete wfh", "wfh role", "wfh job",
	"wfh opportunity", "wfh position", "wfh / remote", "wfh/remote", "remote wfh",
	"wfh (remote)", "permanent work from home",
}

// indianLabelRemote matches a labelled field that states remote work: "Location: WFH",
// "Work mode - WFH".
var indianLabelRemote = regexp.MustCompile(`\b(?:location|work mode|work type|mode of work|work location|job location|working mode)\s*[:\-–]\s*(?:wfh|work from home)\b`)

// indianOnsitePhrases state an office job outright.
var indianOnsitePhrases = []string{"work from office", "work-from-office", "working from office", "wfo"}

func indianWorkMode(desc, lower string) string {
	if !containsAny(lower, indianWorkModeTriggers) {
		return ""
	}
	text := foldMarkup(desc)
	if mode := officeDaysMode(text); mode != "" {
		return mode
	}
	for _, p := range indianRemotePhrases {
		if strings.Contains(text, p) {
			return "remote"
		}
	}
	if indianLabelRemote.MatchString(text) {
		return "remote"
	}
	for _, p := range indianMixedPhrases {
		if strings.Contains(text, p) {
			return "hybrid"
		}
	}
	for _, p := range indianOnsitePhrases {
		if i := wordIndex(text, p); i >= 0 {
			// "Hybrid (10 days WFO in a month)": the office days of a hybrid pattern.
			if strings.Contains(text[max(0, i-hybridLookBehind):i], "hybrid") {
				return "hybrid"
			}
			return "onsite"
		}
	}
	return ""
}

// indianMixedPhrases name office AND home working in one breath.
var indianMixedPhrases = []string{
	"work from office and home", "work from office and work from home",
	"work from home and office", "wfo and wfh", "wfh and wfo", "wfo/wfh", "wfh/wfo",
}

// hybridLookBehind is how far before an office marker a "hybrid" still qualifies it: wide
// enough for "work mode – hybrid (10days wfo", narrow enough to stay in one field.
const hybridLookBehind = 30

// wordIndex returns the index of the first occurrence of p in text with no word byte on
// either side, or -1.
func wordIndex(text, p string) int {
	for off := 0; ; {
		i := strings.Index(text[off:], p)
		if i < 0 {
			return -1
		}
		start, end := off+i, off+i+len(p)
		if (start == 0 || !isWordByte(text[start-1])) && (end == len(text) || !isWordByte(text[end])) {
			return start
		}
		off = start + 1
	}
}

// officeDaysMode reads the first counted office or home days in text: home days, or
// fewer than five office days, are hybrid; five or six office days are onsite.
func officeDaysMode(text string) string {
	if m := officeDaysBefore.FindStringSubmatch(text); m != nil {
		return "onsite"
	}
	m := officeDays.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	if strings.Contains(m[2], "h") && !strings.Contains(m[2], "office") {
		return "hybrid" // "2 days WFH": some days at home, the rest in the office
	}
	switch m[1] {
	case "5", "6", "five", "six":
		return "onsite"
	}
	return "hybrid"
}
