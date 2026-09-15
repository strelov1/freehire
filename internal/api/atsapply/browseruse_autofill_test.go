package atsapply

import (
	"strings"
	"testing"
)

// A live Lever run parked with: "The résumé upload auto-populated unlisted Current company
// and GitHub fields, so I did not submit." The agent was right to be careful and wrong about
// what the rule meant: the PAGE filled those, by parsing the résumé we gave it. Reading the
// rule as "no field outside the list may end up filled" stops every application on a board
// that parses résumés, which is most of them.
func TestTask_DoesNotTreatThePagesOwnAutofillAsAReasonToStop(t *testing.T) {
	task := buildTask(Plan{Fields: []ResolvedField{{ID: "name", Value: "Ilya"}}}, nil, "https://jobs.lever.co/acme/1/apply")

	for _, want := range []string{"parses the attached", "not a reason to stop"} {
		if !strings.Contains(task, want) {
			t.Errorf("task does not mention %q, so an agent still reads the page's own autofill as a violation", want)
		}
	}
}

// The permission is about what the PAGE did, never about what the agent may do. The original
// rule is what keeps an agent from inventing an answer to a question nobody answered.
func TestTask_StillForbidsTheAgentTouchingUnlistedFields(t *testing.T) {
	task := buildTask(Plan{Fields: []ResolvedField{{ID: "name", Value: "Ilya"}}}, nil, "https://jobs.lever.co/acme/1/apply")

	for _, want := range []string{"Do not touch, select, or fill any field not listed above", "Do not guess an answer"} {
		if !strings.Contains(task, want) {
			t.Errorf("task lost %q, which is what stops an agent inventing an answer", want)
		}
	}
}
