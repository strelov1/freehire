package companyname

import (
	"strings"
	"unicode"
)

// genericEmployerPhrases are what a posting says INSTEAD of naming the employer. They are
// really in the text, so grounding alone would admit them.
var genericEmployerPhrases = map[string]bool{
	"the company": true, "our company": true, "company": true, "our client": true,
	"the client": true, "client": true, "the employer": true, "employer": true,
	"confidential": true, "we": true, "our team": true, "the organization": true,
}

// GroundedName admits a candidate employer name only when it appears, as whole words, in
// one of texts — the guard that lets a model READ a name out of a posting while giving it no
// way to invent one. The match ignores case, runs of whitespace and the apostrophe's form
// (a model answers "Children's" for a posting's "Children’s"), and the name returned is the
// TEXT's own spelling of it, never the model's.
func GroundedName(candidate string, texts []string) (string, bool) {
	want := []rune(foldForMatch(strings.Join(strings.Fields(candidate), " ")))
	// The generic check ignores surrounding punctuation ("Our client:"); the grounding below
	// does not, so what is admitted is still exactly what the text says.
	bare := strings.TrimFunc(string(want), func(r rune) bool { return !isWordRune(r) })
	if len(want) < 2 || genericEmployerPhrases[bare] {
		return "", false
	}
	for _, text := range texts {
		orig := []rune(strings.Join(strings.Fields(text), " "))
		folded := []rune(foldForMatch(string(orig)))
		for i := 0; i+len(want) <= len(folded); i++ {
			if string(folded[i:i+len(want)]) != string(want) {
				continue
			}
			before := i == 0 || !isWordRune(orig[i-1])
			after := i+len(want) == len(orig) || !isWordRune(orig[i+len(want)])
			if before && after {
				return string(orig[i : i+len(want)]), true
			}
		}
	}
	return "", false
}

// foldForMatch lowercases and unifies apostrophes rune for rune, so a folded string has
// exactly the runes of its original and an index into one is an index into the other.
func foldForMatch(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '’', '‘', '`':
			return '\''
		}
		return unicode.ToLower(r)
	}, s)
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
