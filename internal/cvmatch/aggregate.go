package cvmatch

import "math"

// aggregate folds the scored categories into a Score.
//
// The overall is the points earned across the AVAILABLE categories over the sum of those
// categories' weights, expressed out of 100. A category the scorer could not evaluate
// leaves both sides of that fraction — it is never scored as zero, which would punish the
// candidate for a gap in our dictionaries rather than in their CV.
//
// The same rule recurses one level down, inside Requirements Coverage: an unverifiable
// requirement leaves that category's own denominator. The two places state one idea —
// an unverifiable input leaves the denominator, never the numerator.
//
// No available category means no score: Overall is 0 and Contributing is empty, and the
// caller renders the absence rather than a zero nobody earned.
func aggregate(categories []ScoredCategory) Score {
	s := Score{Categories: categories}
	earned, possible := 0, 0
	for _, c := range categories {
		if !c.Available {
			continue
		}
		earned += c.Earned
		possible += c.Weight
		s.Contributing = append(s.Contributing, c.ID)
	}
	if possible == 0 {
		return s
	}
	s.Overall = int(math.Round(float64(earned) / float64(possible) * 100))
	return s
}
