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

func TestContainsASCIIDot(t *testing.T) {
	// A leading '.' is not a valid left boundary: ".net" must not match "asp.net".
	if Contains("we use asp.net here", ".net", ASCIIBoundary) {
		t.Error("matched a dotted suffix")
	}
	// A trailing '.' (sentence period) is a valid right boundary.
	if !Contains("we use c#.", "c#", ASCIIBoundary) {
		t.Error("missed a term before a period")
	}
	if !Contains("react native app", "react native", ASCIIBoundary) {
		t.Error("missed a multi-word phrase")
	}
}

func TestEmptyTermNeverMatches(t *testing.T) {
	if Contains("anything", "", UnicodeBoundary) || Contains("anything", "", ASCIIBoundary) {
		t.Error("empty term must never match")
	}
}
