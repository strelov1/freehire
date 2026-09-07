package atsapply

import (
	"strings"
	"testing"
)

func TestBuildTask_IncludesEveryFieldsExactLabelAndValue(t *testing.T) {
	plan := Plan{Fields: []ResolvedField{
		{ID: "first_name", Value: "Ada"},
		{ID: "question_42", Value: "Yes"},
	}}
	merged := []MergedField{
		{ID: "first_name", Label: "First name"},
		{ID: "question_42", Label: "Are you authorized to work here?"},
	}

	task := buildTask(plan, merged, "https://example.com/apply")

	for _, want := range []string{
		"https://example.com/apply",
		`"First name" (id "first_name"): Ada`,
		`"Are you authorized to work here?" (id "question_42"): Yes`,
		"Do not touch",
		string(outcomeConfirmed) + ":",
		string(outcomeUnconfirmed),
		string(outcomeParked) + ":",
	} {
		if !strings.Contains(task, want) {
			t.Errorf("task missing %q\n---\n%s", want, task)
		}
	}
}

func TestBuildTask_FallsBackToIDWhenNoLabelKnown(t *testing.T) {
	plan := Plan{Fields: []ResolvedField{{ID: "custom_field", Value: "42"}}}
	task := buildTask(plan, nil, "https://example.com/apply")
	if !strings.Contains(task, `"custom_field" (id "custom_field"): 42`) {
		t.Errorf("task = %s, want it to fall back to the id as the label", task)
	}
}

func TestParseOutcome_Confirmed(t *testing.T) {
	report := "I filled everything.\nCONFIRMED: Application received!"
	outcome, detail := parseOutcome(report)
	if outcome != outcomeConfirmed || detail != "Application received!" {
		t.Errorf("outcome=%v detail=%q, want confirmed/\"Application received!\"", outcome, detail)
	}
}

func TestParseOutcome_Unconfirmed(t *testing.T) {
	report := "I clicked submit but nothing happened.\nUNCONFIRMED"
	outcome, _ := parseOutcome(report)
	if outcome != outcomeUnconfirmed {
		t.Errorf("outcome = %v, want unconfirmed", outcome)
	}
}

func TestParseOutcome_Parked(t *testing.T) {
	report := "The relocation question was required and unanswered.\nPARKED: relocation question unanswered"
	outcome, detail := parseOutcome(report)
	if outcome != outcomeParked || detail != "relocation question unanswered" {
		t.Errorf("outcome=%v detail=%q, want parked/\"relocation question unanswered\"", outcome, detail)
	}
}

func TestParseOutcome_NoMarkerDefaultsToUnconfirmed(t *testing.T) {
	report := "I think it probably worked out fine in the end."
	outcome, _ := parseOutcome(report)
	if outcome != outcomeUnconfirmed {
		t.Errorf("outcome = %v, want unconfirmed for a report with no explicit marker", outcome)
	}
}

func TestParseOutcome_EmptyReportDefaultsToUnconfirmed(t *testing.T) {
	outcome, _ := parseOutcome("")
	if outcome != outcomeUnconfirmed {
		t.Errorf("outcome = %v, want unconfirmed for an empty report", outcome)
	}
}

func TestParseOutcome_MarkerNotOnTheLastLineDoesNotCount(t *testing.T) {
	// The instruction asks for the marker as the LAST line — one buried earlier followed
	// by more narrative must not be picked up as confirmation.
	report := "CONFIRMED: looked right at the time\nActually wait, I'm not sure that went through."
	outcome, _ := parseOutcome(report)
	if outcome != outcomeUnconfirmed {
		t.Errorf("outcome = %v, want unconfirmed when the marker isn't the last line", outcome)
	}
}

func TestBrowserUseEligible(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		plan     Plan
		want     bool
	}{
		{"ashby with no file field", "ashby", Plan{Fields: []ResolvedField{{ID: "email", Kind: "text"}}}, true},
		{"workable with no file field", "workable", Plan{}, true},
		{"greenhouse is never eligible", "greenhouse", Plan{}, false},
		{"unknown provider is never eligible", "lever", Plan{}, false},
		// Recruitee has no registered applyform.Fetcher at all (found during
		// implementation — see browserUseProviders' own doc comment), so Client.Submit
		// never even reaches browserUseEligible for it; excluded here for the same reason.
		{"recruitee is never eligible", "recruitee", Plan{}, false},
		{"a resume file field disqualifies the plan", "ashby", Plan{Fields: []ResolvedField{{ID: "resume", Kind: "file"}}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := browserUseEligible(tc.provider, tc.plan); got != tc.want {
				t.Errorf("browserUseEligible(%q, ...) = %v, want %v", tc.provider, got, tc.want)
			}
		})
	}
}

func TestRunSpendGuard_AllowsUntilLimitThenShadowLogsInsteadOfRefusing(t *testing.T) {
	g := &runSpendGuard{limitUSD: 1.0}
	g.record(0.5)
	if !g.allow(false) {
		t.Fatal("want allowed below the limit")
	}
	g.record(0.6) // now over the 1.0 limit
	if !g.allow(false) {
		t.Error("want allowed in shadow mode even over the limit — it only logs")
	}
	if g.allow(true) {
		t.Error("want refused once enforced and over the limit")
	}
}

func TestRunSpendGuard_UnlimitedWhenZero(t *testing.T) {
	g := &runSpendGuard{limitUSD: 0}
	g.record(1000)
	if !g.allow(true) {
		t.Error("want a zero limit to mean unlimited, even enforced")
	}
}
