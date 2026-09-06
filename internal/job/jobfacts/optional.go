package jobfacts

import (
	"slices"
	"strings"
	"unicode"

	"github.com/strelov1/freehire/internal/job/reqextract"
)

// optionalMarkers is the closed vocabulary of phrases with which a posting says the
// qualification beside them is wanted rather than required. Same dict-only discipline
// as every other matcher here: a phrasing outside this list is not guessed at, and its
// clause is read as a requirement.
//
// The languages are the ones the matchers above already claim — English, Russian and
// Polish for EnglishLevel — plus Hungarian, whose boards are where this was reported.
//
// Literal phrases, matched with strings.Index over an already-lowercased description,
// rather than one alternating regexp. The regexp is the obvious shape and is what this
// replaced: Go's regexp runs an alternation of this size at about 3 MB/s, which put
// 650µs on EVERY job in the catalogue for a scan that now costs single-digit
// microseconds. It also reads like the heading vocabulary in internal/job/reqextract,
// which is the same kind of list doing the same kind of job.
//
// Two shapes are deliberately absent. A bare "bonus" is not here: it is the ordinary
// word for a payment ("annual bonus", "signing bonus"), so only the phrases that
// qualify a QUALIFICATION are. And a bare "asset" is not, because "assets" contains it;
// the idiom it appears in is spelled out instead.
//
// Every entry must be lowercase, and every entry that a posting may hyphenate is
// spelled both ways — there is no normalization pass between the description and this
// list, on purpose: rewriting the text to match a dictionary is how a mask starts
// changing what the matchers downstream see.
var optionalMarkers = []string{
	// English.
	"preferred", "preferably", "preferable",
	"optional", "optionally",
	"desirable", "desired",
	"nice to have", "nice-to-have",
	"good to have", "good-to-have",
	"great to have",
	"a plus", "a big plus", "a strong plus",
	"a bonus", "bonus points",
	"an advantage", "an asset",
	"not required", "not mandatory",
	// Russian — stems, which is what carries the inflections ("желательно" /
	// "желательна"). Note that these cannot be \b-anchored the way the English ones
	// could: Go's RE2 defines a word boundary on ASCII bytes, so an anchor against
	// Cyrillic lands mid-rune. Being literals, they need no anchor.
	"желательн", "приветствуетс", "будет плюсом", "как плюс", "преимуществ",
	"необязательн", "не обязательн",
	// Polish.
	"mile widzian", "będzie plusem", "atutem", "opcjonaln",
	// Hungarian.
	"előny", "elony",
}

// hardRequirementText returns the description LOWERCASED, with everything the posting
// marks as optional blanked out, so the matchers below derive facts only from what it
// actually requires. It works in two passes because a posting states optionality in two
// places: as a whole SECTION ("Nice to have", "Előnyt jelent"), which reqextract's
// heading vocabulary already recognizes, and as a CLAUSE inside a sentence, which it
// does not.
//
// Both passes blank rather than delete. The matchers read punctuation as structure —
// EnglishLevel binds a level word to an English keyword only when no "." or newline
// lies between them — so removing a clause and rejoining what surrounds it would
// introduce a boundary the posting never wrote, and drop the level from a phrasing as
// ordinary as "English, B2 required". Replacing letters and digits with spaces leaves
// every boundary where the posting put it.
//
// Lowercasing here rather than in each matcher is what lets the vocabulary above be
// literals: it is done once, before the offsets the clause walk works in are taken, and
// every caller lowercased its input as its first act anyway.
func hardRequirementText(description string) string {
	return maskOptionalClauses(strings.ToLower(reqextract.MaskPreferred(description)))
}

// maskOptionalClauses blanks the words of every clause carrying an optional marker and
// leaves the rest of the string, punctuation included, exactly as it was. A description
// naming nothing optional comes back unchanged.
func maskOptionalClauses(lowered string) string {
	markers := markerSpans(lowered)
	if len(markers) == 0 {
		return lowered
	}
	var b strings.Builder
	b.Grow(len(lowered))
	next := 0 // the first marker that may still fall inside a later clause
	start := 0
	writeClause := func(end int) {
		clause := lowered[start:end]
		for next < len(markers) && markers[next][1] <= start {
			next++
		}
		if next < len(markers) && markers[next][0] < end {
			b.WriteString(reqextract.BlankWords(clause))
		} else {
			b.WriteString(clause)
		}
		start = end
	}
	for i, r := range lowered {
		if !clauseBreak(r) && !sentenceBreak(lowered, i, r) {
			continue
		}
		writeClause(i)
		b.WriteRune(r)
		start = i + len(string(r))
	}
	writeClause(len(lowered))
	return b.String()
}

// markerSpans returns every occurrence of the vocabulary in a lowercased description,
// sorted by start offset. Overlaps are left in: the clause walk only asks whether a
// marker falls inside a span, so collapsing them would buy nothing.
func markerSpans(lowered string) [][2]int {
	var out [][2]int
	for _, phrase := range optionalMarkers {
		for at := 0; ; {
			i := strings.Index(lowered[at:], phrase)
			if i < 0 {
				break
			}
			out = append(out, [2]int{at + i, at + i + len(phrase)})
			at += i + len(phrase)
		}
	}
	slices.SortFunc(out, func(a, b [2]int) int { return a[0] - b[0] })
	return out
}

// clauseBreak reports whether a rune ends a clause. A comma or semicolon is enough:
// "Bachelor's required, PhD preferred" states two different things about two degrees,
// and reading the sentence whole is what let the second overwrite the first.
func clauseBreak(r rune) bool {
	return r == ',' || r == ';' || r == '\n'
}

// sentenceBreak reports whether the rune at i ends a sentence: a terminator followed by
// whitespace. The trailing whitespace is what keeps "3.5 years" and "min. B2" — the
// standard Polish phrasing — from being read as two clauses.
func sentenceBreak(s string, i int, r rune) bool {
	if r != '.' && r != '!' && r != '?' {
		return false
	}
	rest := s[i+1:]
	return rest == "" || unicode.IsSpace(rune(rest[0]))
}
