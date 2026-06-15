package location

import "strings"

// descriptionWorkModePhrases maps a work mode to anchored phrases that signal it
// in a job description, checked in priority order (hybrid beats remote, remote
// beats onsite, when several appear). Unlike workModeMarkers — which scan a short
// location string and can use loose tokens — these are tuned for PRECISION over
// recall in long prose: a bare "remote"/"hybrid"/"in office" matches far too much
// ("remote team", "hybrid cloud", "snacks in office"), so every phrase is anchored
// to an unambiguous work-arrangement statement. The detector emits nothing on a
// weak signal (never guesses), consistent with the curated-dictionary doctrine.
var descriptionWorkModePhrases = []struct {
	mode    string
	phrases []string
}{
	{"hybrid", []string{
		"hybrid role", "hybrid working", "hybrid work ", "hybrid work.", "hybrid model",
		"hybrid schedule", "hybrid setup", "hybrid arrangement", "hybrid position",
		"days in the office", "days per week in the office", "days a week in the office",
		"days in office", "days onsite", "days on-site",
	}},
	{"remote", []string{
		"fully remote", "fully-remote", "100% remote", "100 % remote", "remote-first",
		"remote first", "work from anywhere", "work-from-anywhere", "remote position",
		"remote role", "remote job", "remote opportunity", "remote vacancy",
		"this position is remote", "role is remote", "position is remote",
	}},
	{"onsite", []string{
		"on-site only", "onsite only", "on site only", "fully on-site", "fully onsite",
		"100% on-site", "100% onsite", "must be on-site", "must be onsite",
		"on-site position", "onsite position", "in-office position",
		"work from our office", "based in our office", "based in the office",
		"on-site role", "onsite role",
	}},
}

// WorkModeFromDescription derives a work mode from a job description's prose,
// returning "" when no anchored arrangement phrase is present. It is the
// lowest-priority work-mode source (after the structured ATS signal and the parsed
// location marker), so it only ever fills a value the others left empty. Values are
// from enrich.WorkModeValues.
func WorkModeFromDescription(desc string) string {
	lower := strings.ToLower(desc)
	for _, wm := range descriptionWorkModePhrases {
		for _, p := range wm.phrases {
			if strings.Contains(lower, p) {
				return wm.mode
			}
		}
	}
	return ""
}
