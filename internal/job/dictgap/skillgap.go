// Package dictgap turns LLM enrichment facts already sitting in the catalogue into
// ranked candidate gaps in the deterministic dictionaries (skilltag, classify). Every
// function is a pure transform from fetched data to a ranked report; a curator stays the
// only writer of any dictionary source.
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

// SkillGapCandidates ranks the raw enrichment.skills phrases that skilltag.Parse resolves
// to nothing. Phrases differing only in case, hyphen/underscore separators, whitespace or
// trailing sentence punctuation are one candidate: counts sum, and the display form is the
// most frequent spelling. Symbols in a technology's name ("++", "#") are never stripped.
// Blank phrases are dropped. Sorted by count descending, then phrase.
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

// normalizeSkillPhrase collapses only punctuation that never carries a technology's
// identity, in two passes:
//
//  1. Trim trailing sentence punctuation, but never '+' or '#' (C++, C#, F#).
//  2. Lowercase and collapse '-'/'_'/whitespace runs into one space, the separator
//     insensitivity skilltag applies itself. Every other character is kept.
//
// Stripping all punctuation would merge "C" with "C++" and "F" with "F#".
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
