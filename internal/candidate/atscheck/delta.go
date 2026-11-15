package atscheck

// Delta is the change one CV's ATS readiness shows against another's: the overall move,
// one entry per category, and (only when the overall fell) the category that fell furthest.
// It only compares two Reports; the categories, labels and weights stay the scorer's. JSON is
// the wire contract shared with the frontend (see cmd/gen-contracts).
type Delta struct {
	Base     int `json:"base"`     // the baseline CV's overall score
	Tailored int `json:"tailored"` // the compared CV's overall score
	Change   int `json:"change"`   // Tailored − Base, signed
	// Categories reports only the categories present in BOTH reports: one-sided ones would
	// invent a change against an implied zero.
	Categories []CategoryChange `json:"categories"`
	// Regressed is true when the overall score fell. A category may fall while the overall
	// holds; that is not a regression and leaves WorstCategory empty.
	Regressed bool `json:"regressed"`
	// WorstCategory is the id of the category with the most negative change, set only when
	// Regressed. Ties go to the earlier one in the compared report's order, never map order.
	WorstCategory string `json:"worst_category,omitempty"`
}

// CategoryChange is one scoring category's move between the two reports.
type CategoryChange struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Base     int    `json:"base"`
	Tailored int    `json:"tailored"`
	Change   int    `json:"change"` // Tailored − Base, signed
	// Items are the checks behind the TAILORED side's score, so a panel can expand a row. The
	// base side's items are deliberately not carried.
	Items []LineItem `json:"items,omitempty"`
}

// Compare reports how `tailored` differs from `base`. Categories are joined by id and
// emitted in the tailored report's order; the worst-drop search walks that order, so an
// equal drop resolves to the earlier category, not to map order.
func Compare(base, tailored Report) Delta {
	baseByID := make(map[string]ScoreCategory, len(base.Categories))
	for _, c := range base.Categories {
		baseByID[c.ID] = c
	}

	d := Delta{
		Base:     base.Overall,
		Tailored: tailored.Overall,
		Change:   tailored.Overall - base.Overall,
	}
	d.Regressed = d.Change < 0

	worst := 0
	for _, c := range tailored.Categories {
		b, ok := baseByID[c.ID]
		if !ok {
			continue
		}
		change := c.Score - b.Score
		d.Categories = append(d.Categories, CategoryChange{
			ID:       c.ID,
			Label:    c.Label,
			Base:     b.Score,
			Tailored: c.Score,
			Change:   change,
			Items:    c.Items,
		})
		// Strictly less keeps the first of an equal drop, hence the tailored order, not a sort.
		if d.Regressed && change < worst {
			worst = change
			d.WorstCategory = c.ID
		}
	}
	return d
}
