package jobhash

import "strings"

// descMinTokenLen is the shortest word kept in a description signature. Two letters and
// under are articles, prepositions and stray markup letters — they appear in every posting,
// so they inflate the overlap of unrelated jobs without ever distinguishing two related ones.
const descMinTokenLen = 3

// DescriptionSignature reduces a description to the set of distinct meaningful words in its
// VISIBLE text, which is what DescriptionSimilarity compares. It reuses the same
// normalization RoleFingerprint applies (tags stripped, entities decoded, lowercased), then
// keeps alphanumeric runs longer than descMinTokenLen.
//
// A set, not a bag: repeating "Kubernetes" twelve times says no more about a role than saying
// it once, and weighting by count would let one company's verbose boilerplate dominate the
// comparison.
func DescriptionSignature(description string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, word := range strings.FieldsFunc(normalizeRoleText(description), func(r rune) bool {
		return !isAlphanumeric(r)
	}) {
		if len(word) > descMinTokenLen {
			out[word] = struct{}{}
		}
	}
	return out
}

// DescriptionSimilarity is the Jaccard index of two signatures: shared words over total
// distinct words. 1 means the same vocabulary, 0 means none in common.
//
// An EMPTY signature is never similar to anything, itself included. Jaccard of two empty sets
// is undefined (0/0), and treating it as 1 would merge every posting whose description failed
// to normalize into a single cluster — the exact failure the caller cannot detect afterwards.
//
// This is the signal a spike VALIDATED where embedding cosine failed: per-city variants of one
// role score ≥0.95 while genuinely distinct roles in the same bucket score ≤0.5, because
// boilerplate dominates an embedding but shared vocabulary still tracks the specialty. It is
// only sound INSIDE a company+title bucket: across buckets, two boilerplate-heavy postings of
// unrelated roles at one company can score 0.98.
func DescriptionSimilarity(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	small, large := a, b
	if len(large) < len(small) {
		small, large = large, small
	}
	shared := 0
	for word := range small {
		if _, ok := large[word]; ok {
			shared++
		}
	}
	return float64(shared) / float64(len(a)+len(b)-shared)
}

func isAlphanumeric(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
}
