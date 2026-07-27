package autofillagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/autofillagent"
)

// fakeWidgets answers read_form plus the four combobox primitives, recording the
// tool calls in order so a test can assert the widget was *driven* rather than
// written into.
type fakeWidgets struct {
	fields  []autofillagent.Field
	offers  map[string][]string // label -> the options the widget reveals once open
	opens   map[string]string   // label -> combobox.open status, default "opened"
	commits map[string]string   // label -> what the widget commits, default the selected option

	calls     []string
	selected  map[string]string
	committed map[string]string
}

func (f *fakeWidgets) Call(_ context.Context, tool string, args any) (json.RawMessage, error) {
	label, value := readArgs(args)
	f.calls = append(f.calls, tool+" "+label)

	switch tool {
	case "read_form":
		return json.Marshal(map[string]any{"fields": f.fields})

	case "fill_simple":
		return json.Marshal(map[string]any{"outcomes": fillOutcomes(args)})

	case "combobox.open":
		status := f.opens[label]
		if status == "" {
			status = "opened"
		}
		return json.Marshal(map[string]string{"status": status})

	case "combobox.options":
		offered, ok := f.offers[label]
		if !ok {
			return json.Marshal(map[string]any{"status": "not_found"})
		}
		return json.Marshal(map[string]any{"status": "open", "options": offered})

	case "combobox.select":
		if f.selected == nil {
			f.selected = map[string]string{}
			f.committed = map[string]string{}
		}
		f.selected[label] = value
		f.committed[label] = value
		if forced, ok := f.commits[label]; ok {
			f.committed[label] = forced
		}
		return json.Marshal(map[string]string{"status": "selected"})

	case "combobox.verify":
		got := f.committed[label]
		if got == "" {
			return json.Marshal(map[string]any{"status": "empty", "committed": ""})
		}
		status := "mismatch"
		if got == value {
			status = "verified"
		}
		return json.Marshal(map[string]any{"status": status, "committed": got})

	default:
		return nil, errors.New("unknown tool: " + tool)
	}
}

func readArgs(args any) (label, value string) {
	raw, _ := json.Marshal(args)
	var call struct {
		Label string `json:"label"`
		Value string `json:"value"`
	}
	_ = json.Unmarshal(raw, &call)
	return call.Label, call.Value
}

func fillOutcomes(args any) []map[string]string {
	raw, _ := json.Marshal(args)
	var call struct {
		Fills []autofillagent.Fill `json:"fills"`
	}
	_ = json.Unmarshal(raw, &call)
	out := make([]map[string]string, 0, len(call.Fills))
	for _, fill := range call.Fills {
		out = append(out, map[string]string{"label": fill.Label, "status": "filled"})
	}
	return out
}

// widgetPlanner plans the given fills and answers Choose from a fixed table.
type widgetPlanner struct {
	fills   []autofillagent.Fill
	choices map[string]string
	seen    map[string][]string // what options Choose was shown, per question
}

func (p *widgetPlanner) Plan(context.Context, []autofillagent.Field, autofillagent.Profile) ([]autofillagent.Fill, error) {
	return p.fills, nil
}

func (p *widgetPlanner) Choose(_ context.Context, question autofillagent.Field, options []string, _ autofillagent.Profile) (string, error) {
	if p.seen == nil {
		p.seen = map[string][]string{}
	}
	p.seen[question.Label] = options
	return p.choices[question.Label], nil
}

func TestRunDrivesAWidgetTheProfileAnswers(t *testing.T) {
	tools := &fakeWidgets{
		fields: []autofillagent.Field{{Label: "Country", Type: "text", Combo: true}},
		offers: map[string][]string{"Country": {"France", "Germany", "Poland"}},
	}
	planner := &widgetPlanner{
		fills:   []autofillagent.Fill{{Label: "Country"}},
		choices: map[string]string{"Country": "Germany"},
	}

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !contains(rep.Filled, "Country") {
		t.Fatalf("filled = %v, want the widget", rep.Filled)
	}
	// The widget was driven, not written at: the option list was read before a
	// choice was made, and the commit was verified afterwards.
	want := []string{"read_form ", "combobox.open Country", "combobox.options Country", "combobox.select Country", "combobox.verify Country"}
	if len(tools.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", tools.calls, want)
	}
	for i, call := range want {
		if tools.calls[i] != call {
			t.Fatalf("calls = %v, want %v", tools.calls, want)
		}
	}
	if got := planner.seen["Country"]; len(got) != 3 {
		t.Fatalf("Choose saw %v, want the widget's real options", got)
	}
}

func TestRunDoesNotCountAnUnverifiedWidgetWriteAsFilled(t *testing.T) {
	tools := &fakeWidgets{
		fields:  []autofillagent.Field{{Label: "Country", Type: "text", Combo: true, Required: true}},
		offers:  map[string][]string{"Country": {"Germany", "Norfolk Island"}},
		commits: map[string]string{"Country": "Norfolk Island"}, // the widget commits something else
	}
	planner := &widgetPlanner{
		fills:   []autofillagent.Fill{{Label: "Country"}},
		choices: map[string]string{"Country": "Germany"},
	}

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if contains(rep.Filled, "Country") {
		t.Fatal("a widget that committed the wrong value was reported as filled")
	}
	if !contains(rep.Deferred, "Country") {
		t.Fatalf("deferred = %v, want the widget whose write did not verify", rep.Deferred)
	}
}

func TestRunReportsAWidgetThatWillNotOpen(t *testing.T) {
	tools := &fakeWidgets{
		fields: []autofillagent.Field{{Label: "Country", Type: "text", Combo: true}},
		offers: map[string][]string{"Country": {"Germany"}},
		opens:  map[string]string{"Country": "did_not_open"},
	}
	planner := &widgetPlanner{
		fills:   []autofillagent.Fill{{Label: "Country"}},
		choices: map[string]string{"Country": "Germany"},
	}

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(tools.selected) != 0 {
		t.Fatalf("selected %v into a widget that never opened", tools.selected)
	}
	if !contains(rep.Deferred, "Country") {
		t.Fatalf("deferred = %v, want the undrivable widget", rep.Deferred)
	}
}

func TestRunLeavesAWidgetTheProfileDoesNotAnswer(t *testing.T) {
	tools := &fakeWidgets{
		fields: []autofillagent.Field{{Label: "Do you require sponsorship?", Type: "text", Combo: true, Required: true}},
		offers: map[string][]string{"Do you require sponsorship?": {"Yes", "No"}},
	}
	planner := &widgetPlanner{
		fills:   []autofillagent.Fill{{Label: "Do you require sponsorship?"}},
		choices: map[string]string{}, // nothing in the profile answers it
	}

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(tools.selected) != 0 {
		t.Fatalf("selected %v for a question the profile does not answer", tools.selected)
	}
	// Left for the user to answer, not reported as a widget we cannot drive.
	if !contains(rep.Unmapped, "Do you require sponsorship?") {
		t.Fatalf("unmapped = %v, want the unanswered question", rep.Unmapped)
	}
}

// The chosen option goes through the same grounding filter as a typed value: a
// model asked to pick from a list will happily pick a plausible answer to a
// question the profile never addressed.
func TestRunDropsAChosenOptionThatTheProfileDoesNotSupport(t *testing.T) {
	tools := &fakeWidgets{
		fields: []autofillagent.Field{{Label: "Visa status", Type: "text", Combo: true}},
		offers: map[string][]string{"Visa status": {"Citizen", "Permanent resident", "Requires sponsorship"}},
	}
	planner := &widgetPlanner{
		fills:   []autofillagent.Fill{{Label: "Visa status"}},
		choices: map[string]string{"Visa status": "Requires sponsorship"},
	}

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(tools.selected) != 0 {
		t.Fatalf("selected %v, a claim the profile does not make", tools.selected)
	}
	if !contains(rep.Unmapped, "Visa status") {
		t.Fatalf("unmapped = %v, want the ungrounded question", rep.Unmapped)
	}
}

// Grounding by raw substring is far too loose for a short option: "no" sits
// inside "norway", so a profile mentioning Oslo would otherwise ground the answer
// "No" to a question about visa sponsorship — the very class of wrong answer this
// whole capability exists to prevent.
func TestRunDoesNotGroundAShortOptionInAnUnrelatedProfileWord(t *testing.T) {
	tools := &fakeWidgets{
		fields: []autofillagent.Field{{Label: "Are you authorized to work?", Type: "text", Combo: true}},
		offers: map[string][]string{"Are you authorized to work?": {"Yes", "No"}},
	}
	planner := &widgetPlanner{
		fills:   []autofillagent.Fill{{Label: "Are you authorized to work?"}},
		choices: map[string]string{"Are you authorized to work?": "No"},
	}
	norway := autofillagent.Profile{"full_name": "Ilya Strelov", "location": "Oslo, Norway"}

	rep, err := autofillagent.Run(context.Background(), tools, planner, norway)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(tools.selected) != 0 {
		t.Fatalf("selected %v — \"No\" is not answered by living in Norway", tools.selected)
	}
	if !contains(rep.Unmapped, "Are you authorized to work?") {
		t.Fatalf("unmapped = %v, want the question handed back", rep.Unmapped)
	}
}

// The widget's own list is the authority on what can be picked. A model that
// returns something close but absent must not have it approximated.
func TestRunNeverSelectsAnOptionTheWidgetDoesNotOffer(t *testing.T) {
	tools := &fakeWidgets{
		fields: []autofillagent.Field{{Label: "Country", Type: "text", Combo: true}},
		offers: map[string][]string{"Country": {"Germany", "Poland"}},
	}
	planner := &widgetPlanner{
		fills:   []autofillagent.Fill{{Label: "Country"}},
		choices: map[string]string{"Country": "Berlin, Germany"}, // grounded, but not on offer
	}

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(tools.selected) != 0 {
		t.Fatalf("selected %v, which the widget never offered", tools.selected)
	}
	if contains(rep.Filled, "Country") {
		t.Fatal("a widget nothing was written into was reported as filled")
	}
}

func TestRunLeavesWidgetsTheePlanDidNotTargetAlone(t *testing.T) {
	tools := &fakeWidgets{
		fields: []autofillagent.Field{
			{Label: "Country", Type: "text", Combo: true},
			{Label: "Degree", Type: "text", Combo: true},
		},
		offers: map[string][]string{"Country": {"Germany"}, "Degree": {"BSc"}},
	}
	planner := &widgetPlanner{
		fills:   []autofillagent.Fill{{Label: "Country"}},
		choices: map[string]string{"Country": "Germany"},
	}

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, call := range tools.calls {
		if call == "combobox.open Degree" {
			t.Fatal("opened a widget the plan never targeted — 27 widgets would be 27 needless round trips")
		}
	}
	if !contains(rep.Deferred, "Degree") && !contains(rep.Unmapped, "Degree") {
		t.Fatalf("Degree fell out of the report entirely: %+v", rep)
	}
}

// A grouped question arrives from read_form as one field carrying its options, so
// the report names the question rather than each of its 29 countries.
func TestRunNamesAGroupedQuestionOnceInTheReport(t *testing.T) {
	tools := &fakeWidgets{fields: []autofillagent.Field{{
		Label:    "Which countries do you anticipate working in?",
		Type:     "checkbox",
		Required: true,
		Options:  []string{"Australia", "Belgium", "Germany", "Poland"},
	}}}
	planner := &widgetPlanner{}

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.Unmapped) != 1 || rep.Unmapped[0] != "Which countries do you anticipate working in?" {
		t.Fatalf("unmapped = %v, want the question named once", rep.Unmapped)
	}
}
