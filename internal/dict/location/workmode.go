package location

import (
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// descriptionWorkModePhrases maps a work mode to anchored phrases that signal it in a job
// description, checked in priority order (hybrid beats remote, remote beats onsite). Tuned for
// precision over recall in long prose: a bare "remote"/"hybrid"/"in office" matches far too
// much ("remote team", "hybrid cloud", "snacks in office"), so every phrase is anchored to an
// unambiguous arrangement statement. A weak signal emits nothing.
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
		// The home-working register administrative and virtual-assistant postings
		// actually use, alongside the corporate register above.
		"work from home", "work-from-home", "home-based", "home based", "telecommute",
		"virtual position",
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
// from vocab.WorkModeValues. The English phrases are tried first; German prose is read
// only when none of them is stated (see germanWorkModePhrases), Dutch prose only
// when neither is (see dutchWorkModePhrases), then Spanish (workmode_es.go), then
// Portuguese (workmode_pt.go), then French, Croatian/Serbian and Polish, and the Indian register last
// (workmode_in.go).
func WorkModeFromDescription(desc string) string {
	lower := strings.ToLower(desc)
	for _, wm := range descriptionWorkModePhrases {
		for _, p := range wm.phrases {
			if phraseMatches(lower, p) {
				return wm.mode
			}
		}
	}
	if m := germanWorkMode(desc, lower); m != "" {
		return m
	}
	if m := dutchWorkMode(desc, lower); m != "" {
		return m
	}
	if m := spanishWorkMode(desc, lower); m != "" {
		return m
	}
	if m := portugueseWorkMode(desc, lower); m != "" {
		return m
	}
	if m := frenchWorkMode(desc); m != "" {
		return m
	}
	if m := bcsWorkMode(desc, lower); m != "" {
		return m
	}
	if m := polishWorkMode(desc); m != "" {
		return m
	}
	return indianWorkMode(desc, lower)
}

// phraseGuards are one language's qualifiers that withdraw a remote or hybrid phrase:
// a negation just before it, a refusal just after it in its sentence, a bounded perk
// anywhere in its sentence.
type phraseGuards struct {
	negations, refusals, perks []string
	// negationWindow is how far before the phrase a negation still governs it; zero
	// means germanNegationWindow.
	negationWindow int
}

// guardedPhraseMatches reports whether p occurs in text as a whole phrase, read for the
// family mode it belongs to. A remote or hybrid occurrence that one of g's negations
// precedes, one of its refusals follows in its sentence, or one of its perks shares a
// sentence with, is passed over and the scan continues — the shape germanPhraseMatches
// has, with the same windows. The onsite family is the refusals themselves, so none of
// the guards applies to it. Used by the Dutch and Spanish lists.
func guardedPhraseMatches(text, p, mode string, g phraseGuards) bool {
	offset := 0
	for {
		i := strings.Index(text[offset:], p)
		if i < 0 {
			return false
		}
		start, end := offset+i, offset+i+len(p)
		offset = end
		if r, _ := utf8.DecodeRuneInString(text[end:]); end < len(text) && unicode.IsLetter(r) {
			continue
		}
		if start > 0 {
			if r, _ := utf8.DecodeLastRuneInString(text[:start]); unicode.IsLetter(r) {
				continue
			}
		}
		if mode == "onsite" || !g.withdraws(text, start, end) {
			return true
		}
	}
}

// withdraws reports whether the remote or hybrid statement at text[start:end] is withdrawn:
// one of g's negations precedes it in its clause, one of its refusals follows it in its
// sentence, or one of its perks shares that sentence.
func (g phraseGuards) withdraws(text string, start, end int) bool {
	window := g.negationWindow
	if window == 0 {
		window = germanNegationWindow
	}
	// A negation governs its own clause only: "não é 100% remota; modelo híbrido"
	// states the hybrid.
	from := max(0, start-window)
	if j := strings.LastIndexAny(text[from:start], ".;!?"); j >= 0 {
		from += j + 1
	}
	if startsWordAny(text[:start], from, g.negations) {
		return true
	}
	tail := text[end:min(len(text), end+germanRefusalWindow)]
	if j := strings.IndexAny(tail, ".;!?"); j >= 0 {
		tail = tail[:j]
	}
	return containsAny(tail, g.refusals) || containsAny(sentenceAround(text, start, end), g.perks)
}

// startsWordAny reports whether one of words starts a word in s at or after from. A
// negation is a word of its own: "entorno", "bono", "humano" and "polígono" end in "no "
// without negating anything.
func startsWordAny(s string, from int, words []string) bool {
	for _, w := range words {
		for at := from; at < len(s); {
			i := strings.Index(s[at:], w)
			if i < 0 {
				break
			}
			at += i
			if r, _ := utf8.DecodeLastRuneInString(s[:at]); at == 0 || !unicode.IsLetter(r) {
				return true
			}
			at++
		}
	}
	return false
}

// phraseMatches reports whether p occurs in text (already lowercased) unguarded. A match
// qualified by travelPerkPhrases, counted by allowancePhrases ("45 work-from-anywhere days"),
// or qualifying collectiveNouns ("fully remote teams") does not end the search for p: a
// description may hedge one occurrence as a bounded perk and state another plainly, and that
// one still stands (see TestRemoteContradictedReadsPastAQualifiedDenial).
func phraseMatches(text, p string) bool {
	guarded := travelPerkPhrases[p]
	rest := text
	offset := 0
	for {
		i := strings.Index(rest, p)
		if i < 0 {
			return false
		}
		start, end := offset+i, offset+i+len(p)
		perk := (guarded && travelPerkQualified(text, start, end)) ||
			(allowancePhrases[p] && opensWithNoun(text[end:], allowanceUnits)) ||
			((p == "work from home" || p == "work-from-home") &&
				(strings.HasPrefix(text[end:], " options available") || boundedHomeWeek.MatchString(text[end:])))
		if !perk && !opensWithNoun(text[end:], collectiveNouns) {
			return true
		}
		offset = end
		rest = text[offset:]
	}
}

var boundedHomeWeek = regexp.MustCompile(`^ up to (?:[1-4]|one|two|three|four) days?(?:\s*/\s*| per | a )week\b`)

// travelPerkPhrases are the remote-family phrases that are ALSO a common idiom for a bounded
// travel/PTO allowance ("the freedom to work from anywhere in the world for up to a month")
// rather than the posting's own arrangement. No other phrase in descriptionWorkModePhrases
// showed this ambiguity, so guarding them would only cost coverage.
var travelPerkPhrases = map[string]bool{
	"work from anywhere": true,
	"work-from-anywhere": true,
}

// allowancePhrases are the home-working phrases that also serve as an ADJECTIVE on a counted
// perk ("45 work-from-anywhere days", "one work-from-home day each week") rather than the
// posting's arrangement. No nearby qualifier is needed: a phrase immediately followed by the
// unit it counts is never the sentence that says the job itself is remote.
var allowancePhrases = map[string]bool{
	"work from anywhere": true,
	"work-from-anywhere": true,
	"work from home":     true,
	"work-from-home":     true,
}

// allowanceUnits are the nouns a counted home-working perk is measured in.
var allowanceUnits = []string{"days", "day", "weeks", "week"}

// collectiveNouns are the plural nouns an arrangement phrase can qualify without stating this
// posting's arrangement: "experience working with distributed or fully remote teams" describes
// teams in general. Only the plural: "we are a fully remote team" is the employer describing
// itself, which the phrase list exists to read.
var collectiveNouns = []string{"teams"}

// opensWithNoun reports whether rest (the lowercased text right after a phrase) opens with
// one of nouns as a whole word. "day-to-day" is not a unit: "work from home day-to-day"
// states the arrangement, so a hyphen or letter after the noun rules it out.
func opensWithNoun(rest string, nouns []string) bool {
	rest = strings.TrimLeft(rest, " ")
	for _, u := range nouns {
		if !strings.HasPrefix(rest, u) {
			continue
		}
		after := rest[len(u):]
		if after == "" {
			return true
		}
		r, _ := utf8.DecodeRuneInString(after)
		return r != '-' && !unicode.IsLetter(r)
	}
	return false
}

// travelPerkQualifiers mark a travelPerkPhrases match as a bounded allowance rather than an
// unconditional statement. "up to" alone is broad enough: it is inherently a bounded-quantity
// phrase ("for up to a month", "up to 12 days"). Unlike a denial qualifier, it appears on
// EITHER side of the match ("work from anywhere ... for up to a month", "up to 12 days work
// from anywhere"), so both directions are checked.
var travelPerkQualifiers = []string{"up to"}

// travelPerkQualifierWindow reuses denialQualifierWindow's value: observed distances are well
// inside it.
const travelPerkQualifierWindow = denialQualifierWindow

// travelPerkQualified reports whether a bounded-duration qualifier appears within
// travelPerkQualifierWindow characters before or after the match spanning [start, end) in
// text (already lowercased).
func travelPerkQualified(text string, start, end int) bool {
	before := text[max(0, start-travelPerkQualifierWindow):start]
	after := text[end:min(len(text), end+travelPerkQualifierWindow)]
	return containsAny(before, travelPerkQualifiers) || containsAny(after, travelPerkQualifiers)
}

// remoteDenialPhrases are the sentences with which a posting denies remote work outright. A
// SEPARATE, much smaller list than descriptionWorkModePhrases' onsite family: that one fills a
// work mode nothing else stated, while this one OVERRULES a remote one an ATS or location
// string already stated (see jobderive), so it admits only sentences whose whole purpose is to
// say the job is not remote.
//
// Two families are absent on purpose:
//
//   - "fully on-site" / "fully in-office": routinely qualified in prose, e.g. "fully on-site
//     FOR THE FIRST 90 DAYS … after successful completion", a trial period in a posting that
//     is remote afterwards.
//   - "on-site only" / "must be onsite": they buy no coverage and have obvious false-
//     positive shapes ("parking on-site only", "must be onsite for quarterly planning").
//
// The "100% on-site" family is kept, because denialQualifiers covers exactly the qualification
// that disqualified "fully on-site".
var remoteDenialPhrases = []string{
	"100% on-site", "100% onsite", "100 % on-site", "100 % onsite",
	"100 percent on-site", "100 percent onsite",
	"not a remote position", "not a remote role", "not a remote job", "not a remote opportunity",
	"not a remote work-from-home role",
	"position is not remote", "role is not remote", "job is not remote",
	"does not offer remote", "does not offer a remote",
	"do not offer remote", "do not offer a remote",
	"no remote work", "no remote option", "no remote options",
	"remote work is not available", "remote work is not an option", "remote work is not offered",
}

// denialQualifiers scope a denial to something OTHER than this posting's arrangement: a trial
// period before remote work opens up, or the permanent role an internship might lead to
// ("fully on-site for the first 90 days … after successful completion", "this is not a remote
// job IF HIRED afterward").
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
// The caller asks only about a posting already resolved as remote (jobderive does), which
// keeps the markup fold below off the rest of the catalogue.
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
// denial phrases are written against. Where an editor put a <strong> or an &nbsp; is not
// information ("This is not a <strong>REMOTE POSITION</strong>", "This is&nbsp;not a remote
// position"), and matching the raw string would read the first as silent.
// WorkModeFromDescription matches raw on purpose: it fills a blank, so a missed phrase costs a
// facet nobody had; a phrase THIS misses leaves a wrong facet standing.
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
