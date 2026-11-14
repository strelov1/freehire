package search

import (
	"errors"
	"strings"
	"unicode"
)

// maxAlternatives bounds how many OR alternatives one q may carry. The search
// endpoints are public and each alternative is one more Meilisearch query.
const maxAlternatives = 5

// titlePrefix scopes the alternative it starts to the title field.
const titlePrefix = "title:"

// ErrTooManyAlternatives is returned for a q with more than maxAlternatives OR
// alternatives. Handlers render its message as a 400.
var ErrTooManyAlternatives = errors.New("q has more than 5 OR alternatives")

// Alternative is one OR branch of q: the text handed to Meilisearch as written
// (so its quotes and leading-minus exclusions keep working) and, when the branch
// began with title:, the fields it is restricted to. Nil Fields means the
// caller's q_fields, or every searchable field.
type Alternative struct {
	Text   string
	Fields []string
}

// ParseQuery splits q on standalone uppercase OR tokens outside double quotes
// and reads a leading title: on each branch. A q that uses neither operator comes
// back as ONE alternative whose Text is q byte-for-byte, so the request built
// from it is exactly the request built before operators existed.
func ParseQuery(q string) ([]Alternative, error) {
	groups := [][]string{{}}
	hasOperator := false
	for _, tok := range splitOutsideQuotes(q) {
		if tok == "OR" {
			hasOperator = true
			groups = append(groups, []string{})
			continue
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], tok)
	}

	var alts []Alternative
	for _, g := range groups {
		var fields []string
		if len(g) > 0 && strings.HasPrefix(g[0], titlePrefix) {
			hasOperator = true
			fields = []string{"title"}
			g[0] = strings.TrimPrefix(g[0], titlePrefix)
			if g[0] == "" {
				g = g[1:]
			}
		}
		if len(g) == 0 {
			continue
		}
		alts = append(alts, Alternative{Text: strings.Join(g, " "), Fields: fields})
	}
	if !hasOperator || len(alts) == 0 {
		return []Alternative{{Text: q}}, nil
	}
	if len(alts) > maxAlternatives {
		return nil, ErrTooManyAlternatives
	}
	return alts, nil
}

// splitOutsideQuotes splits q on whitespace that is not inside double quotes,
// keeping the quotes in the tokens.
func splitOutsideQuotes(q string) []string {
	var (
		tokens  []string
		b       strings.Builder
		inQuote bool
	)
	for _, r := range q {
		switch {
		case r == '"':
			inQuote = !inQuote
			b.WriteRune(r)
		case unicode.IsSpace(r) && !inQuote:
			if b.Len() > 0 {
				tokens = append(tokens, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		tokens = append(tokens, b.String())
	}
	return tokens
}

// wholeWords ends text with a space, which tells Meilisearch its last word is complete.
// Otherwise the engine expands that word as a prefix ("engineer" → engineers,
// engineering, …), and over a catalogue of millions of postings with long descriptions
// a two-word query ran into the engine's 1.5s search cutoff and came back partly
// ranked: "data engineer" took ~1.6s and put "Machine Learning Engineer" first,
// against ~30ms and "Data Engineer" first with the space (measured on prod, 2026-11-13).
// Typo tolerance still applies to every word.
func wholeWords(text string) string {
	if text == "" || strings.HasSuffix(text, " ") {
		return text
	}
	return text + " "
}

// literalSearchText preserves title words and language tokens such as C++ and C#,
// while removing the quote and unary-minus syntax Meilisearch would execute.
func literalSearchText(text string) string {
	words := strings.Fields(strings.ReplaceAll(text, "\"", " "))
	for i, word := range words {
		words[i] = strings.TrimLeft(word, "-")
	}
	return strings.Join(words, " ")
}
