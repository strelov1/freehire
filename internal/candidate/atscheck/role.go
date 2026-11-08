package atscheck

import "sort"

// RoleTopN is how many of a role's most in-demand skills the CV keyword match is scored
// against. The site's report and the lifecycle letters share it, so they score one CV the
// same way.
const RoleTopN = 20

// TopRoleSkills ranks a skills facet distribution by demand (count desc, slug asc) and
// returns the top n slugs — the role's most in-demand skills, which is the roleTopSkills
// input to Score.
func TopRoleSkills(facet map[string]int64, n int) []string {
	type skillCount struct {
		slug  string
		count int64
	}
	ranked := make([]skillCount, 0, len(facet))
	for slug, count := range facet {
		ranked = append(ranked, skillCount{slug, count})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count != ranked[j].count {
			return ranked[i].count > ranked[j].count
		}
		return ranked[i].slug < ranked[j].slug
	})
	if len(ranked) > n {
		ranked = ranked[:n]
	}
	out := make([]string, len(ranked))
	for i, r := range ranked {
		out[i] = r.slug
	}
	return out
}

// TopGaps returns the texts of the n biggest recoverable line items: those that did not
// pass, ranked by the points they would recover (highest first, ties in report order).
// The texts are the checks' own fixed wording, never anything taken from the CV.
func (r Report) TopGaps(n int) []string {
	var gaps []LineItem
	for _, c := range r.Categories {
		for _, it := range c.Items {
			if it.Status != StatusPass && it.Points > 0 {
				gaps = append(gaps, it)
			}
		}
	}
	sort.SliceStable(gaps, func(i, j int) bool { return gaps[i].Points > gaps[j].Points })
	if len(gaps) > n {
		gaps = gaps[:n]
	}
	out := make([]string, len(gaps))
	for i, g := range gaps {
		out[i] = g.Text
	}
	return out
}
