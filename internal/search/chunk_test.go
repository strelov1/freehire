package search

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// chunkText must leave a short, well-under-budget description as a single chunk — the
// common case (most descriptions fit in one e5 window) should not be needlessly split.
func TestChunkTextShortInputYieldsOneChunk(t *testing.T) {
	text := "Senior Go Engineer.\nBuild payment APIs.\nRequires 5+ years of backend experience."
	got := chunkText(text)
	if len(got) != 1 {
		t.Fatalf("chunkText(short) = %d chunks, want 1: %v", len(got), got)
	}
	if got[0] != text {
		t.Fatalf("chunkText(short) = %q, want unchanged %q", got[0], text)
	}
}

// A long description (well past the chunk budget) must split into multiple chunks, none
// grossly over budget — this is what lets the full text reach the model instead of being
// silently dropped past TEI's 512-token limit (proposal.md motivation #2).
//
// The fixture is fully deterministic (40 identical 117-rune paragraphs against a
// 2000-rune budget), so the expected split — 16/16/8 paragraphs per chunk — is asserted
// exactly rather than with a loose tolerance: a loose "well under some big number" check
// would still pass under an off-by-one in the packing guard (an extra paragraph per
// chunk lands at ~2005 runes, comfortably under a padded threshold like +200) without
// ever going red on that regression.
func TestChunkTextLongInputSplitsWithinBudget(t *testing.T) {
	const marker = "end of paragraph."
	var paras []string
	for i := 0; i < 40; i++ {
		paras = append(paras, strings.Repeat("word ", 20)+marker)
	}
	text := strings.Join(paras, "\n")

	got := chunkText(text)
	wantParasPerChunk := []int{16, 16, 8}
	if len(got) != len(wantParasPerChunk) {
		t.Fatalf("chunkText(long) = %d chunks, want %d", len(got), len(wantParasPerChunk))
	}
	total := 0
	for i, c := range got {
		if n := utf8.RuneCountInString(c); n > chunkBudgetRunes {
			t.Errorf("chunk %d = %d runes, exceeds budget %d: %q", i, n, chunkBudgetRunes, c)
		}
		n := strings.Count(c, marker)
		if n != wantParasPerChunk[i] {
			t.Errorf("chunk %d has %d paragraphs, want %d", i, n, wantParasPerChunk[i])
		}
		total += n
	}
	if total != len(paras) {
		t.Fatalf("chunkText(long) total paragraphs across chunks = %d, want %d (content lost or duplicated)", total, len(paras))
	}
}

// A chunk boundary must fall on whitespace, never mid-word — splitting "requirements"
// into "require" + "ments" across two chunks would corrupt both chunks' embeddings.
//
// Every chunk must consist ENTIRELY of whole copies of the one repeated word — not just
// avoid two specific hardcoded split points — and the word count across all chunks must
// account for every occurrence, so a split at any other offset (or a dropped/duplicated
// word at a boundary) is caught too.
func TestChunkTextDoesNotSplitMidWord(t *testing.T) {
	const word = "supercalifragilisticexpialidocious"
	const count = 400
	text := strings.Repeat(word+" ", count)
	got := chunkText(text)
	if len(got) < 2 {
		t.Fatalf("chunkText(no-newline long) = %d chunks, want > 1", len(got))
	}
	wholeWordChunk := regexp.MustCompile(`^(` + word + ` ?)+$`)
	total := 0
	for i, c := range got {
		trimmed := strings.TrimSpace(c)
		if trimmed == "" {
			t.Errorf("chunk %d is empty/blank: %q", i, c)
			continue
		}
		if !wholeWordChunk.MatchString(trimmed) {
			t.Errorf("chunk %d contains a mid-word split: %q", i, c)
			continue
		}
		total += len(strings.Fields(trimmed))
	}
	if total != count {
		t.Fatalf("chunkText(no-newline long) total words across chunks = %d, want %d (word lost or duplicated)", total, count)
	}
}

// A single whitespace-delimited token longer than the budget (e.g. a long URL) has no
// word boundary to split on — it must still be split by rune count so it cannot produce
// a chunk TEI silently truncates, rather than passing through unsplit.
func TestChunkTextSplitsOversizedWord(t *testing.T) {
	giant := strings.Repeat("a", chunkBudgetRunes*2+500)
	text := "before\n" + giant + "\nafter"
	got := chunkText(text)
	for i, c := range got {
		if n := utf8.RuneCountInString(c); n > chunkBudgetRunes {
			t.Errorf("chunk %d = %d runes, exceeds budget %d", i, n, chunkBudgetRunes)
		}
	}
	var rebuilt strings.Builder
	for _, c := range got {
		rebuilt.WriteString(strings.ReplaceAll(c, "\n", ""))
	}
	if got := rebuilt.String(); !strings.Contains(got, giant) {
		t.Fatalf("chunkText(oversized word) lost or corrupted the giant token; reassembled = %q", got)
	}
}
