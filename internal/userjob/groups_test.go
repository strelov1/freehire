package userjob

import "testing"

// The board shows four columns and the funnel four bands, both of which are groups of stages —
// and each used to carry its own copy of the membership (web/src/lib/board.ts, buckets.go,
// pipeline.ts, HomeFunnel.svelte). This test is what makes one table the only one: a stage added
// to the vocabulary and forgotten here fails the build instead of rendering as a card in no
// column at all.
//
// Exactly one group per stage, in both directions — the shape TestEveryStageIsRankedOrTerminal
// already uses for the rank and terminal tables.
func TestEveryStageBelongsToExactlyOneGroup(t *testing.T) {
	for _, stage := range Stages {
		var in []string
		for _, g := range Groups {
			for _, s := range g.Stages {
				if s == stage {
					in = append(in, g.ID)
				}
			}
		}
		if len(in) != 1 {
			t.Errorf("stage %q belongs to groups %v, want exactly one. A stage in no group has no "+
				"column on the board and no band in the funnel; a stage in two is counted twice.",
				stage, in)
		}
	}

	for _, g := range Groups {
		for _, stage := range g.Stages {
			if !ValidStage(stage) {
				t.Errorf("group %q names %q, which is not in Stages", g.ID, stage)
			}
		}
	}
}

// GroupOf is what every surface calls instead of restating the membership. An unknown stage
// reports no group rather than a plausible one: a card the catalogue cannot place belongs
// nowhere, and guessing `applied` would file it under "waiting for a reply" — a claim about an
// employer we have no basis for.
func TestGroupOf(t *testing.T) {
	tests := []struct {
		stage, want string
	}{
		{"applied", "applied"},
		{"screening", "applied"},
		{"responded", "applied"},
		{"interview", "interview"},
		{"offer", "offer"},
		{"accepted", "closed"},
		{"rejected", "closed"},
		{"withdrawn", "closed"},
		{"definitely-not-a-stage", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := GroupOf(tt.stage); got != tt.want {
			t.Errorf("GroupOf(%q) = %q, want %q", tt.stage, got, tt.want)
		}
	}
}

// Every stage needs the one text a person reads for it. A stage without a label would reach the
// board as a raw identifier — `withdrawn` where `Withdrawn` belongs — so the table is checked
// for completeness rather than trusted.
func TestEveryStageHasALabel(t *testing.T) {
	for _, stage := range Stages {
		if Label(stage) == "" {
			t.Errorf("stage %q has no label; it would render as its own identifier", stage)
		}
	}
	if got := Label("definitely-not-a-stage"); got != "" {
		t.Errorf("Label(unknown) = %q, want empty — an unknown stage has no product text", got)
	}
}
