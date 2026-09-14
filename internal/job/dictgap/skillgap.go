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
// count exactly as recorded. Phrases differing only in case, punctuation, or
// whitespace are treated as one candidate; its count is their sum and its display
// form is whichever original spelling occurred most often. Blank phrases are
// dropped. The result is sorted by count descending, then by phrase for a
// deterministic order among ties.
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

// normalizeSkillPhrase collapses case, punctuation, and whitespace differences so
// trivial spelling variants of the same phrase group together: letters and digits
// are lowercased and kept, every other run of characters becomes a single space,
// and the result is trimmed. It never guesses a semantic equivalence beyond that.
func normalizeSkillPhrase(phrase string) string {
	var b strings.Builder
	pendingSpace := false
	for _, r := range phrase {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if pendingSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(unicode.ToLower(r))
			pendingSpace = false
			continue
		}
		pendingSpace = true
	}
	return b.String()
}
