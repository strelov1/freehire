package main

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/classify"
)

// A stop word that also occurs inside a known non-tech role phrase would silently
// hide that whole family from mining: every group built from the phrase is dropped
// before the operator ever sees it, and the cluster reads as absent rather than
// filtered. "home" would cost "home health aide", "call" would cost "call center".
// The non-tech dictionary is the closest thing to a ground truth for role-bearing
// words, so it is the guard. Connectors are exempt by design — they may sit inside
// a phrase, which is the whole reason they are a separate class.
func TestNoStopWordIsPartOfAKnownNonTechRole(t *testing.T) {
	stop := set(stopWords)

	for _, term := range classify.NonTechTerms() {
		for _, tok := range strings.Fields(term) {
			if stop[tok] {
				t.Errorf("stop word %q is a token of non-tech term %q — mining would never surface that role family", tok, term)
			}
		}
	}
}

// A connector is only ever dropped at a group's edge, so one that opens or closes a
// real role phrase would truncate it. The Portuguese entries are the live case:
// "de" is legitimate mid-phrase in "operador de caixa" but must not be the first or
// last word of any term, or the miner could never reproduce that term.
func TestNoConnectorEdgesAKnownNonTechRole(t *testing.T) {
	conn := set(connectors)

	for _, term := range classify.NonTechTerms() {
		toks := strings.Fields(term)
		if len(toks) == 0 {
			continue
		}
		if conn[toks[0]] {
			t.Errorf("connector %q opens non-tech term %q — a group reproducing that term would be dropped at its edge", toks[0], term)
		}
		if last := toks[len(toks)-1]; conn[last] {
			t.Errorf("connector %q closes non-tech term %q — a group reproducing that term would be dropped at its edge", last, term)
		}
	}
}

// The two classes mean opposite things to the query, so a word in both would be
// ambiguous: dropped everywhere by one rule, allowed mid-phrase by the other.
func TestStopWordsAndConnectorsDoNotOverlap(t *testing.T) {
	conn := set(connectors)
	for _, w := range stopWords {
		if conn[w] {
			t.Errorf("%q is both a stop word and a connector — the two rules contradict", w)
		}
	}
}

func TestVocabulariesAreLowercaseAndUnique(t *testing.T) {
	for name, list := range map[string][]string{"stopWords": stopWords, "connectors": connectors} {
		seen := map[string]bool{}
		for _, w := range list {
			if w != strings.ToLower(w) {
				t.Errorf("%s: %q must be lowercase — titles are lowercased before tokenizing", name, w)
			}
			if seen[w] {
				t.Errorf("%s: %q is listed twice", name, w)
			}
			seen[w] = true
		}
	}
}

func set(words []string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}
