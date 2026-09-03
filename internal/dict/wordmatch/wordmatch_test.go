package wordmatch

import "testing"

func TestContainsUnicode(t *testing.T) {
	// Unicode letter/digit boundaries: "lead" must not match inside "leading".
	if Contains("leading role", "lead", UnicodeBoundary) {
		t.Error("matched inside a longer word")
	}
	if !Contains("team lead wanted", "lead", UnicodeBoundary) {
		t.Error("missed a whole-word occurrence")
	}
	// Cyrillic boundaries behave like Latin.
	if !Contains("ищем сеньор разработчика", "сеньор", UnicodeBoundary) {
		t.Error("missed a Cyrillic whole word")
	}
}

func TestContainsTechTermDot(t *testing.T) {
	// A leading '.' is not a valid left boundary: ".net" must not match "asp.net".
	if Contains("we use asp.net here", ".net", TechTermBoundary) {
		t.Error("matched a dotted suffix")
	}
	// A trailing '.' (sentence period) is a valid right boundary.
	if !Contains("we use c#.", "c#", TechTermBoundary) {
		t.Error("missed a term before a period")
	}
	if !Contains("react native app", "react native", TechTermBoundary) {
		t.Error("missed a multi-word phrase")
	}
}

// TestTechTermBoundaryNonASCIINeighbour pins the boundary rule that a byte-level
// ASCII test got wrong: a letter outside ASCII is still a letter, so a term
// sitting inside an accented or Cyrillic word is not a standalone term. Reading
// only ASCII bytes made "elk" a whole word inside the Hungarian "elkészítése"
// (the byte after "elk" is the first of "é"), which tagged ELK/Elasticsearch on
// 16% of a live Hungarian IT crawl.
func TestTechTermBoundaryNonASCIINeighbour(t *testing.T) {
	for _, tc := range []struct{ text, term string }{
		{"a dokumentáció elkészítése a feladatod", "elk"},
		{"münchen", "m"},
		{"разработка", "раз"},
		{"señor developer", "se"},
	} {
		if Contains(tc.text, tc.term, TechTermBoundary) {
			t.Errorf("%q matched inside %q", tc.term, tc.text)
		}
	}
	// The rule cuts only fragments: a whole term beside non-ASCII text still matches.
	if !Contains("elk stack üzemeltetése", "elk", TechTermBoundary) {
		t.Error("missed a whole term next to non-ASCII text")
	}
}

func TestEmptyTermNeverMatches(t *testing.T) {
	if Contains("anything", "", UnicodeBoundary) || Contains("anything", "", TechTermBoundary) {
		t.Error("empty term must never match")
	}
}
