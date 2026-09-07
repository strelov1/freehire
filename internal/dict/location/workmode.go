package location

import (
	"html"
	"strings"
	"unicode"
)

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
// from vocab.WorkModeValues.
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

// remoteDenialPhrases are the sentences with which a posting denies remote work outright.
// They are a SEPARATE, much smaller list from descriptionWorkModePhrases' onsite family, and
// deliberately so: that list fills a work mode nothing else stated, while this one OVERRULES a
// remote one an ATS or a location string already stated (see jobderive). A phrase strong enough
// to fill a blank is not automatically strong enough to overrule a stated value, so this list
// admits only sentences whose whole purpose is to say the job is not remote.
//
// Two families that look like they belong here are absent, both on measured evidence over ~9.8k
// prod postings the catalogue serves as remote:
//
//   - "fully on-site" / "fully in-office" — every live hit was one employer writing "fully
//     on-site FOR THE FIRST 90 DAYS … after successful completion", a trial period in a posting
//     that is remote afterwards. The phrase reads absolute in a list and is routinely qualified
//     in prose.
//   - "on-site only" / "must be onsite" — they matched nothing at all across the sample, so they
//     buy no coverage, and both have an obvious false-positive shape ("parking on-site only",
//     "must be onsite for quarterly planning"). An entry that cannot be shown to help and can be
//     argued to hurt does not go in.
//
// The "100% on-site" family is the one absolute-sounding phrase kept, because the guard below
// covers exactly the qualification that disqualified "fully on-site".
var remoteDenialPhrases = []string{
	"100% on-site", "100% onsite", "100 % on-site", "100 % onsite",
	"100 percent on-site", "100 percent onsite",
	"not a remote position", "not a remote role", "not a remote job", "not a remote opportunity",
	"position is not remote", "role is not remote", "job is not remote",
	"does not offer remote", "does not offer a remote",
	"do not offer remote", "do not offer a remote",
	"no remote work", "no remote option", "no remote options",
	"remote work is not available", "remote work is not an option", "remote work is not offered",
}

// denialQualifiers scope a denial to something OTHER than this posting's arrangement — a trial
// period before remote work opens up, or the permanent role an internship might lead to. Each
// entry comes from a real posting that the unqualified list read backwards: two ADP postings
// ("fully on-site for the first 90 days … after successful completion") and a DoD SkillBridge
// internship whose own header says "This is a remote position" and whose body adds "this is not
// a remote job IF HIRED afterward".
var denialQualifiers = []string{
	"for the first", "after the first", "for the initial", "after the initial",
	"after successful", "if hired", "once hired",
}

// denialQualifierWindow is how far past a denial a qualifier still governs it. Wide enough for
// the observed sentences ("100% on-site for the first 30-90 days at headquarters"), narrow
// enough that an unrelated later sentence cannot excuse a denial that stood on its own.
const denialQualifierWindow = 60

// RemoteContradicted reports whether a description explicitly denies that the posting is
// remote. It answers only that one question — it never proposes a work mode — because its
// caller uses it to overrule a "remote" the ordinary precedence produced, and "not remote"
// does not distinguish onsite from hybrid on its own.
//
// The caller is expected to ask only about a posting already resolved as remote (jobderive
// does), which is what keeps the markup fold below off the ~97% of the catalogue that could
// not be affected by the answer.
func RemoteContradicted(desc string) bool {
	text := foldMarkup(desc)
	for _, p := range remoteDenialPhrases {
		rest := text
		for {
			i := strings.Index(rest, p)
			if i < 0 {
				break
			}
			rest = rest[i+len(p):]
			tail := rest
			if len(tail) > denialQualifierWindow {
				tail = tail[:denialQualifierWindow]
			}
			if !containsAny(tail, denialQualifiers) {
				return true
			}
			// A qualified denial is not the last word: a posting may hedge one sentence and
			// state another plainly, so the scan continues rather than returning false.
		}
	}
	return false
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// foldMarkup renders a stored description down to the lowercased, single-spaced prose the
// denial phrases are written against. Descriptions are HTML, and where an editor put a
// <strong> or an &nbsp; is not information: prod carries both "This is not a
// <strong>REMOTE POSITION</strong>" and "This is&nbsp;not a remote position", and matching
// the raw string would read the first as silent. WorkModeFromDescription above does match
// raw, and stays that way — it fills a blank, so a phrase it misses costs a facet nobody had;
// a phrase THIS misses costs a wrong facet left standing.
func foldMarkup(desc string) string {
	text := html.UnescapeString(stripTags(desc))
	var b strings.Builder
	b.Grow(len(text))
	pendingSpace := false
	for _, r := range strings.ToLower(text) {
		if unicode.IsSpace(r) {
			pendingSpace = b.Len() > 0
			continue
		}
		if pendingSpace {
			b.WriteByte(' ')
			pendingSpace = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// stripTags replaces every HTML element with a space, so two words a tag separated do not fuse.
// A '<' with no '>' after it is left alone rather than swallowing the remainder: descriptions
// reach storage through sources.sanitizeHTML, which escapes a stray '<' into &lt;, so an
// unterminated one here means the input is not the sanitized HTML this expects.
func stripTags(s string) string {
	if !strings.ContainsRune(s, '<') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for {
		i := strings.IndexByte(s, '<')
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		j := strings.IndexByte(s[i:], '>')
		if j < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		b.WriteByte(' ')
		s = s[i+j+1:]
	}
}
