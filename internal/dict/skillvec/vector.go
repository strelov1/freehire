package skillvec

import "math"

// ballastPosition is the vector slot the JOB side fills and the profile never does.
// It sits past the registry, in the headroom Dimensions leaves — so it can never
// collide with a skill, and adding skills to the registry can never reach it.
const ballastPosition = Dimensions - 1

// ballastPerSkill and ballastFloorSkills shape how hard a posting dilutes itself.
//
// Without ballast, the cosine rewards the SIZE of the overlap: a vacancy sitting almost
// entirely inside a large profile scores about √(overlap) / ‖profile‖, so one listing 79
// skills of which the reader holds 63 beats one listing 5 they hold entirely. Measured
// on production against a 162-skill profile, the whole top ten was postings with 52-92
// skills — from a catalogue whose median is 7.
//
// With it, the score reduces to roughly (overlap / |B|) — coverage — because the ballast
// dominates the job vector's length. That is the whole point: coverage decides.
//
// The floor is not a detail. A single tag the reader holds is 100% covered, so without
// it the feed fills with one-skill postings; swept over real data, the top ten became
// single-skill nursing vacancies. Six prices that out while leaving real postings alone.
//
// Values come from sweeping k ∈ [2,12] and floor ∈ [5,12] over a stratified production
// sample; results plateau from k=4, so this is the gentlest setting that reaches it.
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

// ballast is how much a posting asking for n skills dilutes itself by.
//
// n counts what actually REACHED the vector, not the raw slice: unknown slugs and
// repeats contribute no component, so charging for them would push a posting down for
// asking more than it does. ["go","go","retired-slug"] asks for one skill.
func ballast(n int) float64 {
	if n < ballastFloorSkills {
		n = ballastFloorSkills
	}
	return ballastPerSkill * float64(n)
}

// vector is the shared construction. Every recognised skill contributes the SAME
// amount — see the package doc for why the rarity weighting was removed — and
// withBallast places a value at ballastPosition before normalisation.
func vector(skills []string, withBallast bool) []float32 {
	if len(skills) == 0 {
		return nil
	}
	v := make([]float32, Dimensions)
	var placed int
	for _, s := range skills {
		pos, ok := Position(s)
		if !ok || v[pos] != 0 {
			// Unknown slugs contribute nothing, and a slug listed twice must not
			// weigh double.
			continue
		}
		v[pos] = 1
		placed++
	}
	if placed == 0 {
		// Nothing recognised: the ballast alone would be a vector about nothing.
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
