package cvmatch

// Status and LineItem are this package's own, deliberately not imported from
// internal/candidate/atscheck: the two scorers share no code, and coupling them so one could not be
// re-weighted without the other would buy nothing.
//
// On the wire they are deliberately the SAME shape: the panel renders both through one
// component, so the contract generator excludes these two (see cmd/gen-contracts) and the
// generated LineItem is shared. If the shapes diverge, that exclusion has to go.

// Status is a check outcome inside a category.
type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

// LineItem is one attributed reason inside a category: the points it awarded when the
// check passed, or the points still recoverable when it did not.
type LineItem struct {
	Points int    `json:"points"`
	Text   string `json:"text"`
	Status Status `json:"status"`
}

// tallyStatus grades a "got of wanted" count, the shape both counted categories report in.
// Nothing matched is a failure rather than a warning: a CV sharing no vocabulary at all
// with its vacancy is the one case worth alarming about.
func tallyStatus(got, want int) Status {
	switch got {
	case want:
		return StatusPass
	case 0:
		return StatusFail
	default:
		return StatusWarn
	}
}
