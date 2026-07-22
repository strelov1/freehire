package pii

import (
	"strings"
	"testing"
)

// Finding 1: a detected value that abuts a word character in the text must still be masked
// (word-boundary anchoring must never leave PII in the prompt).
func TestRedactMasksValueAbuttingWordChar(t *testing.T) {
	text := "mail: strelov1@gmail.com2024 archived"
	r := mustBuild(t, text, Contacts{}, nameDetector{})
	if masked := r.Redact(text); strings.Contains(masked, "strelov1@gmail.com") {
		t.Fatalf("email abutting a digit leaked: %q", masked)
	}
}

// Finding 1b: a NAME span whose offsets fall inside a larger token must still be masked.
func TestRedactMasksNameSpanInsideToken(t *testing.T) {
	text := "IlyaStrelovX updated the resume"
	r := mustBuild(t, text, Contacts{}, nameDetector{names: []string{"Strelov"}})
	if masked := r.Redact(text); strings.Contains(masked, "Strelov") {
		t.Fatalf("name span inside a token leaked: %q", masked)
	}
}

// Finding 2: an employment year range with spaces must not be masked as a phone number.
func TestSpacedYearRangeIsNotPhone(t *testing.T) {
	text := "worked there 2012 - 2016 as engineer"
	if got := collect(text, regexSpans(text), KindPhone); got != nil {
		t.Fatalf("spaced year range detected as phone: %q", got)
	}
}
