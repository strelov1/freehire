// Package roletype resolves whether a job title names a people-management role.
// It is a curated dictionary, not a model — the same doctrine as internal/classify,
// internal/roletag and internal/skilltag: it emits the canonical value for what it
// can resolve and nothing for what it cannot, and it never guesses.
//
// The vocabulary (vocab.RoleTypeValues) holds ONE value. "This role manages people"
// is legible from a title; "this role does not" is not, and the absence of a marker
// is therefore an unresolved posting rather than a resolved individual contributor.
// Measured on production, the titles that state IC status outright — the
// staff/principal ladder, "member of technical staff", the literal phrase — are
// about 9,100 of 3.1M postings, 0.3%. Nothing downstream may present the absence of
// a value as evidence of the opposite.
//
// The signal is orthogonal to the facets that already exist: of the 171,726
// production titles carrying a marker, only 15,760 sit in category=management and
// only 46,330 carry seniority lead-or-c_level, leaving 111,786 reachable by neither.
package roletype

import (
	"strings"

	"github.com/strelov1/freehire/internal/wordmatch"
)

// managerMarkers name a person who has reports, whatever the craft. Matched as whole
// words against the lowercased title.
var managerMarkers = []string{
	"head of",
	"director",
	// The graded forms are separate entries because matching is whole-word: "vp"
	// cannot fire inside "svp", so an SVP/EVP/AVP title would otherwise resolve to
	// nothing despite naming the same office.
	"vp",
	"svp",
	"evp",
	"avp",
	"vice president",
	"chief",
	"supervisor",
}

// craftManagers are the "… manager" forms whose managed noun is a team of people.
// They are an explicit allowlist because the bare word is not a marker: on production
// 150,779 of the 378,410 titles containing "manager" are individual-contributor roles
// where the managed noun is a product, a project, a program or an account. Admitting
// "manager" alone would make this facet wrong for two matches in five, so a title
// earns the value only by naming the craft it manages.
var craftManagers = []string{
	"engineering manager",
	"development manager",
	"software development manager",
	"software manager",
	"technology manager",
	"technical manager",
	"it manager",
	"qa manager",
	"quality assurance manager",
	"data manager",
	"design manager",
	"infrastructure manager",
	"security manager",
	"operations manager",
}

// blindPhrases contain a managerMarkers word while naming no people-management role.
// "Chief" is the marker that needs this: Chief Engineer, Chief Architect and Chief
// Scientist are the top of an individual-contributor ladder, and a Chief of Staff is
// an exec aide whose reports are usually nobody. This mirrors
// classify.gradeBlindPhrases, which solves the same shape of problem for grades.
//
// The phrases are CUT from the title before matching rather than used to reject it,
// so a genuine marker elsewhere in the same title survives — "Director of
// Engineering, Chief Architect Track" is still a manager because of "director".
//
// The cut is word-boundary aware for the same reason the matching is. A bare
// substring cut would also swallow any LONGER word a phrase happens to prefix:
// "chief engineer" prefixes "chief engineering", so "Chief Engineering Officer" —
// who runs the org — lost its marker and resolved to nothing.
var blindPhrases = []string{
	"chief of staff",
	"chief engineer",
	"chief architect",
	"chief scientist",
}

// Derive resolves title to "people_manager" or to "". It lowercases, cuts the blind
// phrases, then looks for a marker or a craft-qualified manager. Matching is
// whole-word (via wordmatch), so "vp" never fires inside "vps" and "director" never
// inside "directorate".
//
// Note what is deliberately absent. "Lead" is not a marker: production's
// seniority=lead holds 116,893 postings and only 3,303 carry any management marker,
// so in this catalogue the word names the IC ladder (Tech Lead, Lead Engineer) far
// more often than a manager. Resolving it either way would be a guess, so a Lead
// title stays unresolved and reachable through the seniority facet instead.
func Derive(title string) string {
	s := strings.ToLower(title)
	for _, phrase := range blindPhrases {
		s = cutWord(s, phrase)
	}
	for _, term := range managerMarkers {
		if wordmatch.Contains(s, term, wordmatch.UnicodeBoundary) {
			return "people_manager"
		}
	}
	for _, term := range craftManagers {
		if wordmatch.Contains(s, term, wordmatch.UnicodeBoundary) {
			return "people_manager"
		}
	}
	return ""
}

// cutWord replaces every whole-word occurrence of phrase in s with a space. It is
// the mask's counterpart to wordmatch.Contains: cutting on a bare substring would
// consume any longer word the phrase prefixes, which is how the "chief engineer"
// mask silently blinded "Chief Engineering Officer".
func cutWord(s, phrase string) string {
	var b strings.Builder
	for from := 0; ; {
		i := strings.Index(s[from:], phrase)
		if i < 0 {
			b.WriteString(s[from:])
			return b.String()
		}
		i += from
		end := i + len(phrase)
		if !wordmatch.UnicodeBoundary(s, i, end) {
			// Not a standalone occurrence — keep it and resume just past its start, so
			// an occurrence beginning inside this one is still considered.
			b.WriteString(s[from : i+1])
			from = i + 1
			continue
		}
		b.WriteString(s[from:i])
		b.WriteString(" ")
		from = end
	}
}
