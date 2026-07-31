package atscheck

// Delta is the change one CV's ATS readiness shows against another's: the overall
// move plus one entry per category, and — only when the overall fell — the category
// that fell furthest. It compares two Reports and computes nothing itself, so the
// five categories, their labels and their weights stay the scorer's business.
//
// JSON is the wire contract shared with the frontend (see cmd/gen-contracts).
type Delta struct {
	Base     int `json:"base"`     // the baseline CV's overall score
	Tailored int `json:"tailored"` // the compared CV's overall score
	Change   int `json:"change"`   // Tailored − Base, signed
	// Categories reports only the categories present in BOTH reports: a category on
	// one side alone is not a difference, and reporting it against an implied zero
	// would invent a change nobody made.
	Categories []CategoryChange `json:"categories"`
	// Regressed is true when the overall score fell. A category may fall while the
	// overall holds; that is not a regression, and WorstCategory stays empty for it.
	Regressed bool `json:"regressed"`
	// WorstCategory is the id of the category with the most negative change, set only
	// when Regressed. An equal drop resolves to the earlier of the two in the compared
	// report's own order, so the answer never depends on map iteration.
	WorstCategory string `json:"worst_category,omitempty"`
}

// CategoryChange is one scoring category's move between the two reports.
type CategoryChange struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Base     int    `json:"base"`
	Tailored int    `json:"tailored"`
	Change   int    `json:"change"` // Tailored − Base, signed
	// Items are the checks behind the TAILORED side's score, so a panel can expand a row
	// into why it stands where it does. The base side's items are deliberately not carried:
	// the candidate is editing the tailored copy, and a before/after list of individual
	// checks is a diff nobody asked for.
	Items []LineItem `json:"items,omitempty"`
}

// Compare reports how `tailored` differs from `base`. Categories are joined by id and
// emitted in the tailored report's order; the worst-drop search walks that same order,
// so an equal drop resolves to the earlier category rather than to whichever the map
// happened to yield.
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
		// Strictly less keeps the first of an equal drop, which is why this reads the
		// tailored order rather than sorting.
		if d.Regressed && change < worst {
			worst = change
			d.WorstCategory = c.ID
		}
	}
	return d
}
