package autofillagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/autofillagent"
)

// fakeTools answers read_form from a canned form and records what fill_simple
// was asked to write, reporting every requested fill as written unless the field
// was flagged as a combobox.
type fakeTools struct {
	fields    []autofillagent.Field
	requested []autofillagent.Fill
	readErr   error
}

func (f *fakeTools) Call(_ context.Context, tool string, args any) (json.RawMessage, error) {
	switch tool {
	case "read_form":
		if f.readErr != nil {
			return nil, f.readErr
		}
		return json.Marshal(map[string]any{"fields": f.fields})
	case "fill_simple":
		raw, _ := json.Marshal(args)
		var call struct {
			Fills []autofillagent.Fill `json:"fills"`
		}
		_ = json.Unmarshal(raw, &call)
		f.requested = call.Fills

		combo := map[string]bool{}
		for _, field := range f.fields {
			combo[field.Label] = field.Combo
		}
		outcomes := make([]map[string]string, 0, len(call.Fills))
		for _, fill := range call.Fills {
			status := "filled"
			if combo[fill.Label] {
				status = "deferred_combobox"
			}
			outcomes = append(outcomes, map[string]string{"label": fill.Label, "status": status})
		}
		return json.Marshal(map[string]any{"outcomes": outcomes})
	default:
		return nil, errors.New("unknown tool: " + tool)
	}
}

// plannerFunc adapts a function to autofillagent.Planner.
type plannerFunc func([]autofillagent.Field, autofillagent.Profile) ([]autofillagent.Fill, error)

func (p plannerFunc) Plan(_ context.Context, f []autofillagent.Field, pr autofillagent.Profile) ([]autofillagent.Fill, error) {
	return p(f, pr)
}

// None of the forms in this file has a widget the plan targets, so reaching
// Choose means the loop drove something it was not asked to.
func (p plannerFunc) Choose(context.Context, autofillagent.Field, []string, autofillagent.Profile) (string, error) {
	return "", errors.New("Choose was called for a form with no widget targets")
}

func profile() autofillagent.Profile {
	return autofillagent.Profile{
		"full_name": "Ilya Strelov",
		"email":     "ilya@example.com",
		"location":  "Berlin, Germany",
	}
}

func TestRunFillsThePlannedFieldsAndReportsThem(t *testing.T) {
	tools := &fakeTools{fields: []autofillagent.Field{
		{Label: "Full name", Type: "text"},
		{Label: "Email", Type: "email"},
	}}
	planner := plannerFunc(func(_ []autofillagent.Field, p autofillagent.Profile) ([]autofillagent.Fill, error) {
		return []autofillagent.Fill{
			{Label: "Full name", Value: p["full_name"]},
			{Label: "Email", Value: p["email"]},
		}, nil
	})

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.Filled) != 2 {
		t.Fatalf("filled = %v, want both fields", rep.Filled)
	}
	if len(tools.requested) != 2 {
		t.Fatalf("fill_simple asked for %d fills, want 2", len(tools.requested))
	}
}

func TestRunDropsAValueThatIsNotGroundedInTheProfile(t *testing.T) {
	tools := &fakeTools{fields: []autofillagent.Field{
		{Label: "Email", Type: "email"},
		{Label: "Why do you want this job?", Type: "textarea"},
	}}
	planner := plannerFunc(func(_ []autofillagent.Field, p autofillagent.Profile) ([]autofillagent.Fill, error) {
		return []autofillagent.Fill{
			{Label: "Email", Value: p["email"]},
			// The model invents an answer the profile never contained.
			{Label: "Why do you want this job?", Value: "I am deeply passionate about your mission."},
		}, nil
	})

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, fill := range tools.requested {
		if fill.Label == "Why do you want this job?" {
			t.Fatal("an invented value reached the browser")
		}
	}
	if !contains(rep.Unmapped, "Why do you want this job?") {
		t.Fatalf("unmapped = %v, want the ungrounded field reported", rep.Unmapped)
	}
}

// A yes/no question the profile does not answer is exactly what the agent must
// not answer on the user's behalf: "willing to relocate" and "authorized to
// work" are claims, not contact details. Nothing grounds "true" in a profile, so
// the filter drops it and the field is handed back to the user.
func TestRunDoesNotAnswerAYesNoQuestionTheProfileSaysNothingAbout(t *testing.T) {
	tools := &fakeTools{fields: []autofillagent.Field{
		{Label: "Willing to relocate", Type: "checkbox"},
	}}
	planner := plannerFunc(func(_ []autofillagent.Field, _ autofillagent.Profile) ([]autofillagent.Fill, error) {
		return []autofillagent.Fill{{Label: "Willing to relocate", Value: "true"}}, nil
	})

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(tools.requested) != 0 {
		t.Fatalf("fill_simple was asked to write %v; the claim is not the agent's to make", tools.requested)
	}
	if !contains(rep.Unmapped, "Willing to relocate") {
		t.Fatalf("unmapped = %v, want the unanswered question handed back", rep.Unmapped)
	}
}

func TestRunAcceptsAValueDerivedFromAProfileValue(t *testing.T) {
	tools := &fakeTools{fields: []autofillagent.Field{{Label: "City", Type: "text"}}}
	planner := plannerFunc(func(_ []autofillagent.Field, _ autofillagent.Profile) ([]autofillagent.Fill, error) {
		// "Berlin" is part of the profile's "Berlin, Germany".
		return []autofillagent.Fill{{Label: "City", Value: "Berlin"}}, nil
	})

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !contains(rep.Filled, "City") {
		t.Fatalf("filled = %v, want City", rep.Filled)
	}
}

// A screening-question field (notice period, salary, visa, relocation, 18+) reaches the
// form through exactly the same profile-map + grounding path every other field does —
// screeninganswers.Answers.AutofillFields just adds more keys to the flat map Run already
// grounds fills against. This is a regression test for that wiring, not new Run behavior.
func TestRunFillsAScreeningQuestionFromTheProfile(t *testing.T) {
	tools := &fakeTools{fields: []autofillagent.Field{{Label: "What is your notice period?", Type: "text"}}}
	planner := plannerFunc(func(_ []autofillagent.Field, p autofillagent.Profile) ([]autofillagent.Fill, error) {
		return []autofillagent.Fill{{Label: "What is your notice period?", Value: p["notice_period"]}}, nil
	})
	profileWithScreening := profile()
	profileWithScreening["notice_period"] = "30 days"

	rep, err := autofillagent.Run(context.Background(), tools, planner, profileWithScreening)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !contains(rep.Filled, "What is your notice period?") {
		t.Fatalf("filled = %v, want the notice-period question", rep.Filled)
	}
	if len(tools.requested) != 1 || tools.requested[0].Value != "30 days" {
		t.Fatalf("requested fills = %v, want notice period 30 days", tools.requested)
	}
}

func TestRunReportsComboboxesInsteadOfFillingThem(t *testing.T) {
	tools := &fakeTools{fields: []autofillagent.Field{
		{Label: "Email", Type: "email"},
		{Label: "Country", Type: "text", Combo: true},
	}}
	planner := plannerFunc(func(_ []autofillagent.Field, p autofillagent.Profile) ([]autofillagent.Fill, error) {
		return []autofillagent.Fill{{Label: "Email", Value: p["email"]}}, nil
	})

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !contains(rep.Deferred, "Country") {
		t.Fatalf("deferred = %v, want the combobox", rep.Deferred)
	}
	if contains(rep.Filled, "Country") {
		t.Fatal("a combobox was reported as filled")
	}
}

func TestRunSkipsTheBrowserEntirelyWhenNothingIsPlanned(t *testing.T) {
	tools := &fakeTools{fields: []autofillagent.Field{{Label: "Favourite colour", Type: "text"}}}
	planner := plannerFunc(func(_ []autofillagent.Field, _ autofillagent.Profile) ([]autofillagent.Fill, error) {
		return nil, nil
	})

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if tools.requested != nil {
		t.Fatalf("fill_simple was called with %v despite an empty plan", tools.requested)
	}
	if !contains(rep.Unmapped, "Favourite colour") {
		t.Fatalf("unmapped = %v, want the unplanned field", rep.Unmapped)
	}
}

// A real ATS form carries controls with no label of their own — a widget's inner
// input, a stray hidden field. They cannot be addressed by label, so telling the
// user they were "left for you" is noise about something they cannot act on.
func TestRunLeavesUnlabelledControlsOutOfTheReport(t *testing.T) {
	tools := &fakeTools{fields: []autofillagent.Field{
		{Label: "Email", Type: "email"},
		{Label: "", Type: "text"},
		{Label: "   ", Type: "text"},
	}}
	planner := plannerFunc(func(_ []autofillagent.Field, p autofillagent.Profile) ([]autofillagent.Fill, error) {
		return []autofillagent.Fill{{Label: "Email", Value: p["email"]}}, nil
	})

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.Unmapped) != 0 {
		t.Fatalf("unmapped = %#v, want nothing the user cannot address", rep.Unmapped)
	}
}

// A Greenhouse form contributes a checkbox per country, which swamps the two or
// three required questions the user actually has to answer. The report is read
// top-down and truncated, so what must be answered comes first.
func TestRunReportsRequiredFieldsBeforeOptionalOnes(t *testing.T) {
	tools := &fakeTools{fields: []autofillagent.Field{
		{Label: "Australia", Type: "checkbox"},
		{Label: "Belgium", Type: "checkbox"},
		{Label: "Who is your current employer?", Type: "text", Required: true},
		{Label: "Canada", Type: "checkbox"},
		{Label: "What is your job title?", Type: "text", Required: true},
	}}
	planner := plannerFunc(func([]autofillagent.Field, autofillagent.Profile) ([]autofillagent.Fill, error) {
		return nil, nil
	})

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{"Who is your current employer?", "What is your job title?"}
	if len(rep.Unmapped) < 2 || rep.Unmapped[0] != want[0] || rep.Unmapped[1] != want[1] {
		t.Fatalf("unmapped = %v, want the required questions first", rep.Unmapped)
	}
	if len(rep.Unmapped) != 5 {
		t.Fatalf("unmapped = %v, want every unfilled field still reported", rep.Unmapped)
	}
}

func TestRunFailsWhenThePageHasNoForm(t *testing.T) {
	tools := &fakeTools{fields: nil}
	planner := plannerFunc(func([]autofillagent.Field, autofillagent.Profile) ([]autofillagent.Fill, error) {
		t.Fatal("planned despite an empty form")
		return nil, nil
	})

	if _, err := autofillagent.Run(context.Background(), tools, planner, profile()); err == nil {
		t.Fatal("Run succeeded on a page with no fields")
	}
}

func TestRunSurfacesAToolFailure(t *testing.T) {
	tools := &fakeTools{readErr: errors.New("the browser extension is not connected")}
	planner := plannerFunc(func([]autofillagent.Field, autofillagent.Profile) ([]autofillagent.Fill, error) {
		return nil, nil
	})

	_, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err == nil || err.Error() != "the browser extension is not connected" {
		t.Fatalf("err = %v, want the tool's message", err)
	}
}

func contains(list []string, want string) bool {
	for _, got := range list {
		if got == want {
			return true
		}
	}
	return false
}
