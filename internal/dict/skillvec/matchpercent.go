package skillvec

import (
	"math"
	"strings"
)

// MatchPercent is the share of a posting's skills the reader holds, rounded to a whole
// percent: the figure on the job card (computeClientMatch in web/src/lib/jobMatch.ts).
// Slugs compare case-insensitively. A posting with no skills has no share to state and
// reads as 0.
//
// It counts the raw lists, not the vector positions: the card cannot see which slugs the
// registry knows, so the filter must not either or the two would disagree.
func MatchPercent(jobSkills, held []string) int {
	if len(jobSkills) == 0 {
		return 0
	}
	have := make(map[string]struct{}, len(held))
	for _, s := range held {
		have[strings.ToLower(s)] = struct{}{}
	}
	matched := 0
	for _, s := range jobSkills {
		if _, ok := have[strings.ToLower(s)]; ok {
			matched++
		}
	}
	return int(math.Round(float64(matched) / float64(len(jobSkills)) * 100))
}

// Similarity is the cosine the match sort ranks a posting by: the reader's vector
// against the vector the index stores for that posting, ballast included. Zero when the
// posting has no recognised skill, which is also how the index ranks it.
func Similarity(profile []float32, jobSkills []string) float64 {
	job := JobVector(jobSkills)
	if len(job) != len(profile) {
		return 0
	}
	var dot float64
	for i, p := range profile {
		dot += float64(p) * float64(job[i])
	}
	return dot
}

// MinCosineAtPercent is the lowest cosine a posting asking for six skills or more can
// score while still showing percent or better, for a reader whose vector is profile.
//
// Such a posting's cosine is coverage * c / sqrt(16 + 1/n), n being what it asks for:
// c is the reader's own component (1/sqrt(held)) and 16 is ballastPerSkill squared.
// 1/n is largest at the floor, so n = ballastFloorSkills gives the smallest cosine, and
// coverage is at least percent - 0.5 before rounding. Beneath it nothing of that size
// can still qualify.
//
// A posting asking for fewer than the floor is ballasted as if it asked for six, so the
// same percent scores LOWER and may sit below this bound: a rule that stops here misses
// it. That is the ballast working as designed, not a gap to close here.
func MinCosineAtPercent(profile []float32, percent int) float64 {
	var c float64
	for _, x := range profile {
		if x != 0 {
			c = float64(x)
			break
		}
	}
	coverage := (float64(percent) - 0.5) / 100
	return coverage * c / math.Sqrt(ballastPerSkill*ballastPerSkill+1.0/ballastFloorSkills)
}
