package atsapply

import (
	"strings"
	"testing"
)

// A refusal names our own detector, not the board's reason. The one time this fired on a
// live Lever posting the operator learned only that "please try again" appeared somewhere
// on the page — which sentence said it, and therefore what the board actually objected to,
// cost a second real submission to discover. A submit click cannot be withdrawn, so the
// evidence has to come back with the first one.
func TestRefusalEvidence_CarriesTheSentenceAroundTheMarker(t *testing.T) {
	body := "Apply for Staff Engineer\n\nWe could not process your resume. Please try again with a PDF or DOCX file.\n\nPowered by Lever"

	got := refusalEvidence(body, "please try again")

	for _, want := range []string{"could not process your resume", "PDF or DOCX"} {
		if !strings.Contains(got, want) {
			t.Fatalf("evidence = %q, want it to carry %q", got, want)
		}
	}
}

// The page text is the whole rendered document — on a real posting, tens of kilobytes of
// description. All of it lands in the queue row's last_error, which an operator reads in a
// terminal.
func TestRefusalEvidence_IsBounded(t *testing.T) {
	body := strings.Repeat("x", 5_000) + " please try again " + strings.Repeat("y", 5_000)

	if got := refusalEvidence(body, "please try again"); len(got) > 400 {
		t.Fatalf("evidence length = %d, want it bounded well under the page", len(got))
	}
}

// Collapsed, because the page text arrives full of the newlines and runs of spaces the
// layout put there, and a log line broken across forty lines is not read.
func TestRefusalEvidence_CollapsesWhitespace(t *testing.T) {
	body := "Resume\n\n\n   missing.\n\tPlease try again\n\n   now."

	if got := refusalEvidence(body, "please try again"); strings.ContainsAny(got, "\n\t") || strings.Contains(got, "  ") {
		t.Fatalf("evidence = %q, want it collapsed onto one line", got)
	}
}
