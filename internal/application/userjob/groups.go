package userjob

// Group is the coarse state the board shows as a column and the funnel as a band;
// Stages are the fine state inside it. An application is in exactly one group, but the
// group alone does not determine the stage, so dropping a card into Closed must ask
// which outcome applies.
//
// TestEveryStageBelongsToExactlyOneGroup keeps this the only membership table.
type Group struct {
	// ID is the stable key. The generated frontend contract keys off it, so it is not a label.
	ID string
	// Label is the one text a person reads for this group, on the board and funnel alike.
	Label string
	// Stages are the vocabulary members, in pipeline order.
	Stages []string
}

// Groups is the stage→group membership, in pipeline order.
// Groups is the stage→group membership, in pipeline order.
//
// `closed` holds all three settled outcomes: separate columns would be almost always
// empty. The card keeps its own stage label.
var Groups = []Group{
	{ID: "preparing", Label: "Preparing", Stages: []string{"preparing"}},
	{ID: "applied", Label: "Applied", Stages: []string{"applied", "screening", "responded"}},
	{ID: "interview", Label: "Interview", Stages: []string{"interview"}},
	{ID: "offer", Label: "Offer", Stages: []string{"offer"}},
	{ID: "closed", Label: "Closed", Stages: []string{"accepted", "rejected", "withdrawn", "expired"}},
}

// stageLabels is the one text a person reads for each stage.
//
// Spelled out rather than title-cased from the key: these are the product's words and
// deriving them would leave no place to change one. Lives here so the in-app
// assistant, which bypasses Fiber, can see it.
var stageLabels = map[string]string{
	"preparing": "Preparing",
	"applied":   "Applied",
	"screening": "Screening",
	"responded": "Responded",
	"interview": "Interview",
	"offer":     "Offer",
	"accepted":  "Accepted",
	"rejected":  "Rejected",
	"withdrawn": "Withdrawn",
	"expired":   "Expired",
}

// GroupOf reports the group a stage belongs to, or "" for a stage outside the vocabulary.
//
// Unknown reports nothing rather than falling back to `applied`: an unplaceable stage
// is not evidence the employer went quiet, which the board's silence badge reads from
// the applied group.
func GroupOf(stage string) string {
	for _, g := range Groups {
		for _, s := range g.Stages {
			if s == stage {
				return g.ID
			}
		}
	}
	return ""
}

// Label reports the human label of a stage, or "" for one outside the vocabulary.
func Label(stage string) string { return stageLabels[stage] }
