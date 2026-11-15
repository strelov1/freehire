package skillvec

import "math"

// ballastPosition is the vector slot only the JOB side fills. It sits in the headroom
// past the registry, so no skill can collide with it.
const ballastPosition = Dimensions - 1

// ballastPerSkill and ballastFloorSkills shape how hard a posting dilutes itself.
//
// Without ballast the cosine rewards the SIZE of the overlap, so a 79-skill listing the
// reader mostly holds beats a 5-skill one they hold entirely. With it the score reduces
// to roughly overlap / |B| (coverage), because the ballast dominates the job vector's
// length.
//
// The floor stops one-skill postings from winning: a single held tag is 100% covered.
// Both values come from a parameter sweep; results plateau from k=4.
const (
	ballastPerSkill    = 4.0
	ballastFloorSkills = 6
)

// JobVector builds the vector stored on a JOB document: the skills it asks for, plus a
// ballast the profile side never sets. The ballast adds nothing to the cosine's
// numerator and lengthens this vector alone, so a posting is discounted in proportion
// to how much it asks — which is what makes coverage decide the order.
func JobVector(skills []string) []float32 { return vector(skills, true) }

// ProfileVector builds the vector for the READER. No ballast: the position must stay
// zero on this side, since that is what keeps it out of the numerator. If both sides
// set it, every pair would match on it and the ranking would flatten.
func ProfileVector(skills []string) []float32 { return vector(skills, false) }

// ballast is how much a posting asking for n skills dilutes itself by. n counts what
// REACHED the vector: unknown slugs and repeats add no component, so ["go","go","retired-slug"]
// asks for one skill.
func ballast(n int) float64 {
	if n < ballastFloorSkills {
		n = ballastFloorSkills
	}
	return ballastPerSkill * float64(n)
}

// vector is the shared construction: every recognised skill contributes the SAME amount
// (see the package doc), and withBallast sets ballastPosition before normalisation.
func vector(skills []string, withBallast bool) []float32 {
	if len(skills) == 0 {
		return nil
	}
	v := make([]float32, Dimensions)
	var placed int
	for _, s := range skills {
		pos, ok := Position(s)
		if !ok || v[pos] != 0 {
			// Unknown slugs add nothing; a repeated slug must not weigh double.
			continue
		}
		v[pos] = 1
		placed++
	}
	if placed == 0 {
		// Ballast alone would be a vector about nothing.
		return nil
	}
	sumSq := float64(placed)
	if withBallast {
		b := ballast(placed)
		v[ballastPosition] = float32(b)
		sumSq += b * b
	}
	norm := float32(math.Sqrt(sumSq))
	for i, x := range v {
		if x != 0 {
			v[i] = x / norm
		}
	}
	return v
}
