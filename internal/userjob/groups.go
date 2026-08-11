package userjob

// Group is the coarse state the board shows as a column and the funnel as a band; Stages are the
// fine state inside it. An application is always in exactly one group, and the group alone does
// not determine the stage — which is why dropping a card into Closed has to ask which outcome
// applies.
//
// The membership lives here, next to activeRank and silenceThresholds, because it answers a
// question about the same vocabulary and had drifted into four separate copies: the board's
// STAGE_COLUMN, the seven pipeline buckets, the funnel's own list, and a fourth inside
// HomeFunnel.svelte. TestEveryStageBelongsToExactlyOneGroup is what keeps this the only one.
type Group struct {
	// ID is the stable key. The generated frontend contract keys off it, so it is not a label.
	ID string
	// Label is the one text a person reads for this group, on the board column and the funnel
	// band alike. Two screens naming one state differently is the defect this table exists to
	// prevent.
	Label string
	// Stages are the vocabulary members, in pipeline order.
	Stages []string
}

// Groups is the stage→group membership, in pipeline order.
//
// `closed` holds all three settled outcomes rather than giving each its own column: a board with
// Accepted, Rejected and Withdrawn side by side is three columns that are almost always empty
// next to one that holds most of the history. The card keeps its own stage label, so the precise
// outcome stays legible inside the coarse column.
var Groups = []Group{
	{ID: "preparing", Label: "Preparing", Stages: []string{"preparing"}},
	{ID: "applied", Label: "Applied", Stages: []string{"applied", "screening", "responded"}},
	{ID: "interview", Label: "Interview", Stages: []string{"interview"}},
	{ID: "offer", Label: "Offer", Stages: []string{"offer"}},
	{ID: "closed", Label: "Closed", Stages: []string{"accepted", "rejected", "withdrawn", "expired"}},
}

// stageLabels is the one text a person reads for each stage.
//
// Spelled out rather than derived by title-casing the key, even though every label happens to be
// exactly that today: these are the product's words, and deriving them would mean the day
// `responded` should read "They replied" the change has nowhere to live. The label previously sat
// in web/src/lib/stages.ts, where the in-app assistant — which calls the tracking service
// directly and never passes through Fiber — could not see it.
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
// Unknown reports nothing rather than falling back to `applied`: a stage we cannot place is not
// evidence that an employer has gone quiet, and the board's silence badge reads exactly that from
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
