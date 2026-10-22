package classify

import (
	"strings"
	"testing"
)

func TestFoldTerm(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Back-End", "back-end"},
		{"back end", "back-end"},
		{"ёлка", "елка"},
		{"ML / AI", "ml-ai"},
		{"  ", ""},
		{"C++", "c"},
	}
	for _, tc := range cases {
		if got := FoldTerm(tc.in); got != tc.want {
			t.Errorf("FoldTerm(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestGapTerm(t *testing.T) {
	trimmed, normalized, ok := GapTerm("  Back End  ")
	if !ok || trimmed != "Back End" || normalized != "back-end" {
		t.Fatalf("GapTerm = %q %q %v", trimmed, normalized, ok)
	}
	if _, _, ok := GapTerm("   "); ok {
		t.Fatal("blank term accepted")
	}
	if _, _, ok := GapTerm("--"); ok {
		t.Fatal("term that folds to nothing accepted")
	}
	// The limit is in characters, not bytes: 80 Cyrillic letters are 160 bytes.
	if _, _, ok := GapTerm(strings.Repeat("я", 80)); !ok {
		t.Fatal("80-character Cyrillic term refused")
	}
	if _, _, ok := GapTerm(strings.Repeat("a", 81)); ok {
		t.Fatal("81-character term accepted")
	}
}
