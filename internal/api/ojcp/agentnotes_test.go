package ojcp

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/job/jobview"
)

func TestJobPostingFromStatesThePostingRealityVerdict(t *testing.T) {
	j := openPosting()
	j.Ghost = &jobview.Ghost{
		Level:         "likely",
		Criteria:      []string{"evergreen_posting", "ats_absent"},
		CriteriaTotal: 4,
	}

	posting := jobPostingFrom(j, testOrigin)

	if posting.AgentNotes == "" {
		t.Fatal("agent_notes is empty; the posting carries a reality verdict")
	}
	// The criteria are stated as sentences, not as their internal codes: an agent relays
	// this note to a candidate, and "evergreen_posting" reaches them as jargon.
	for _, want := range []string{"2 of 4", "open unusually long", "employer's own ATS"} {
		if !strings.Contains(posting.AgentNotes, want) {
			t.Errorf("agent_notes does not state %q: %s", want, posting.AgentNotes)
		}
	}
	for _, code := range []string{"evergreen_posting", "ats_absent"} {
		if strings.Contains(posting.AgentNotes, code) {
			t.Errorf("agent_notes leaks the internal criterion code %q: %s", code, posting.AgentNotes)
		}
	}
	if err := validateAgainstSchema(t, schemaJobPosting, posting); err != nil {
		t.Fatalf("posting with agent_notes rejected: %v", err)
	}
}

func TestJobPostingFromHedgesTheVerdictItStates(t *testing.T) {
	// The signal observes facts about a POSTING, never an employer's intent. An agent
	// relaying our note to a candidate must not be handed an accusation to relay.
	for _, tc := range []struct{ level, want string }{
		{"likely", "likely"},
		{"possible", "possibly"},
	} {
		t.Run(tc.level, func(t *testing.T) {
			j := openPosting()
			j.Ghost = &jobview.Ghost{Level: tc.level, Criteria: []string{"ats_absent"}, CriteriaTotal: 4}

			notes := jobPostingFrom(j, testOrigin).AgentNotes

			if !strings.Contains(notes, tc.want) {
				t.Errorf("agent_notes = %q, want it hedged with %q", notes, tc.want)
			}
			for _, forbidden := range []string{"fake", "fraud", "scam", "lying"} {
				if strings.Contains(strings.ToLower(notes), forbidden) {
					t.Errorf("agent_notes accuses the employer (%q): %s", forbidden, notes)
				}
			}
		})
	}
}

func TestJobPostingFromSaysNothingWhenThereIsNothingToSay(t *testing.T) {
	// Most of the catalogue carries no verdict. An empty note, or one saying "no concerns",
	// would be a claim we did not make — absence is the only honest rendering.
	for name, j := range map[string]jobview.Job{
		"no verdict":  openPosting(),
		"level none":  withGhost(openPosting(), &jobview.Ghost{Level: "none", CriteriaTotal: 4}),
		"empty level": withGhost(openPosting(), &jobview.Ghost{Level: "", CriteriaTotal: 4}),
	} {
		t.Run(name, func(t *testing.T) {
			if notes := jobPostingFrom(j, testOrigin).AgentNotes; notes != "" {
				t.Errorf("agent_notes = %q, want empty", notes)
			}
		})
	}
}

func withGhost(j jobview.Job, g *jobview.Ghost) jobview.Job {
	j.Ghost = g
	return j
}
