// Package dictgap turns LLM enrichment facts already sitting in the catalogue into
// ranked candidate gaps in the deterministic dictionaries (internal/dict/skilltag,
// internal/dict/classify). It reads nothing itself and writes nothing: every function
// here is a pure transform from already-fetched data to a ranked report, so a curator
// deciding what to add stays the only writer of any dictionary source file.
package dictgap

import (
	"sort"
	"strings"
	"unicode"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// SkillGapCandidate is one raw enrichment skill phrase the skill-tagging dictionary
// resolves to no canonical skill, with how many jobs' enrichment recorded it.
type SkillGapCandidate struct {
	Phrase string
	Count  int
}

// SkillGapCandidates ranks the raw enrichment.skills phrases that
// internal/dict/skilltag.Parse resolves to nothing, given each phrase's occurrence
// count exactly as recorded. Phrases differing only in case, a hyphen/underscore
// used as a word separator, surrounding/internal whitespace, or a trailing
// sentence-punctuation mark are treated as one candidate; its count is their sum
// and its display form is whichever original spelling occurred most often. A
// symbol that is part of a technology's own name (e.g. the "++" in C++, the "#" in
// C#/F#) is never stripped — doing so would fold distinct, already-individually-
// resolvable skills into the same bucket as an unrelated bare-letter gap. Blank
// phrases are dropped. The result is sorted by count descending, then by phrase
// for a deterministic order among ties.
func SkillGapCandidates(counts map[string]int) []SkillGapCandidate {
	type bucket struct {
		total   int
		display string
		best    int
	}
	buckets := make(map[string]*bucket)
	for phrase, count := range counts {
		key := normalizeSkillPhrase(phrase)
		if key == "" {
			continue
		}
		b, ok := buckets[key]
		if !ok {
			b = &bucket{}
			buckets[key] = b
		}
		b.total += count
		if count > b.best || (count == b.best && phrase < b.display) {
			b.best = count
			b.display = phrase
		}
	}

	candidates := make([]SkillGapCandidate, 0, len(buckets))
	for _, b := range buckets {
		if len(skilltag.Parse(b.display)) > 0 {
			continue
		}
		candidates = append(candidates, SkillGapCandidate{Phrase: b.display, Count: b.total})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Count != candidates[j].Count {
			return candidates[i].Count > candidates[j].Count
		}
		return candidates[i].Phrase < candidates[j].Phrase
	})
	return candidates
}

// normalizeSkillPhrase collapses only the punctuation this codebase already treats
// as insignificant, never a character that can carry a technology's identity.
// Two passes:
//
//  1. Trim trailing sentence-punctuation (a stray period, comma, etc. an LLM's list
//     formatting can leave glued to the last word) — but never a trailing '+' or
//     '#', since those END identity-bearing names (C++, C#, F#) rather than
//     punctuating a sentence.
//  2. Lowercase, and collapse '-'/'_'/whitespace runs into a single space — the
//     same separator-insensitivity internal/dict/skilltag itself applies when
//     resolving a multi-word phrase (see its package doc: hyphenated, underscored,
//     and spaced forms of a term all resolve alike). Every other character
//     (including '+', '#', '.', '/') is kept exactly as written.
//
// This is deliberately narrower than "strip all punctuation": widening it invited
// bare "C" and "C++" (or "F" and "F#") into the same bucket, corrupting or hiding
// the report's real signal for two actually-distinct technologies.
func normalizeSkillPhrase(phrase string) string {
	trimmed := strings.TrimRightFunc(phrase, func(r rune) bool {
		return r != '+' && r != '#' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	var b strings.Builder
	pendingSpace := false
	for _, r := range trimmed {
		if r == '-' || r == '_' || unicode.IsSpace(r) {
			if b.Len() > 0 {
				pendingSpace = true
			}
			continue
		}
		if pendingSpace {
			b.WriteByte(' ')
			pendingSpace = false
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
