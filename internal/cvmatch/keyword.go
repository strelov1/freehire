package cvmatch

import (
	"fmt"
	"math"
)

// keywordCategory scores how many of the vacancy's canonical skills appear in the skills
// parsed from the document's rendered text, and splits them into the ones present and the
// ones to add. Both sides come from the same dictionary the ATS-readiness score uses over
// the same rendered text, so the two surfaces can differ in weight but never in WHICH
// skills they think the document contains.
//
// Results keep the vacancy's own order: it is the employer's ordering of what matters, and
// re-sorting it would invent a priority we do not have.
func keywordCategory(cvSkills, jobSkills []string) (c ScoredCategory, matched, missing []string) {
	c = ScoredCategory{ID: CategoryKeyword, Label: "Keyword Match", Weight: WeightKeyword}

	// A vacancy whose skills the dictionary never resolved is a gap on our side. Awarding
	// full marks would flatter the CV and awarding none would libel it.
	if len(jobSkills) == 0 {
		c.Reason = "this vacancy lists no recognised skills to match against"
		return c, nil, nil
	}
	c.Available = true

	owned := make(map[string]bool, len(cvSkills))
	for _, s := range cvSkills {
		owned[s] = true
	}
	for _, s := range jobSkills {
		if owned[s] {
			matched = append(matched, s)
		} else {
			missing = append(missing, s)
		}
	}

	c.Earned = int(math.Round(float64(len(matched)) / float64(len(jobSkills)) * WeightKeyword))
	c.Items = []LineItem{{
		Points: c.Earned,
		Text:   fmt.Sprintf("%d of %d skills this vacancy names are in your CV", len(matched), len(jobSkills)),
		Status: tallyStatus(len(matched), len(jobSkills)),
	}}
	if len(missing) > 0 {
		c.Items = append(c.Items, LineItem{
			Points: c.Weight - c.Earned,
			Text:   "Add the remaining skills where you have genuinely used them",
			Status: StatusWarn,
		})
	}
	return c, matched, missing
}
