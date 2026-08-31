package job_test

import (
	"testing"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
)

// The clearance facet is derived by the aggregate factory, so every write path —
// board ingest, moderator authoring, Telegram — gets it without opting in. Deriving
// it at a call site instead would let the three drift, which is the failure this
// factory exists to make impossible.
func TestNew_ClearanceFacetIndependentOfWritePath(t *testing.T) {
	content := job.Draft{Input: jobderive.Input{
		Title:       "Senior Go Developer",
		Company:     "Acme",
		Location:    "London",
		Description: "You must hold or be eligible for SC clearance.",
	}}
	tg, board, manual := content, content, content
	tg.Source, tg.ExternalID = "telegram", "chan/1/0"
	board.Source, board.ExternalID = "greenhouse", "acme:42"
	manual.Source, manual.ExternalID = "manual", "7"

	for name, d := range map[string]job.Draft{"telegram": tg, "board": board, "manual": manual} {
		t.Run(name, func(t *testing.T) {
			j, err := job.New(d)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			got := j.Fields().RequiresClearance
			if got == nil || !*got {
				t.Errorf("RequiresClearance = %v, want true", got)
			}
		})
	}
}

// A posting that states nothing leaves the facet nil rather than false: the column
// records "not stated", and a stored false would read as "stated to be unnecessary".
func TestNew_ClearanceUnknownStaysNil(t *testing.T) {
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source:      "greenhouse",
		ExternalID:  "acme:1",
		Title:       "Backend Engineer",
		Company:     "Acme",
		Description: "We use Go and Kubernetes.",
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := j.Fields().RequiresClearance; got != nil {
		t.Errorf("RequiresClearance = %v, want nil", *got)
	}
}
