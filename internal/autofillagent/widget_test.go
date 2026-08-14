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
	// errs fails the named "tool label" call outright (a transport-level error, not
	// a status reply), so a test can exercise a widget that fails partway through.
	errs map[string]error

	calls     []string
	requested []autofillagent.Fill // what fill_simple was actually asked to write
	frames    map[string]int       // "tool label" -> the frame arg that call carried
	selected  map[string]string
	committed map[string]string
}

func (f *fakeWidgets) Call(_ context.Context, tool string, args any) (json.RawMessage, error) {
	label, value, frame := readArgs(args)
	f.calls = append(f.calls, tool+" "+label)
	if f.frames == nil {
		f.frames = map[string]int{}
	}
	f.frames[tool+" "+label] = frame

	if err, ok := f.errs[tool+" "+label]; ok {
		return nil, err
	}

	switch tool {
	case "read_form":
		return json.Marshal(map[string]any{"fields": f.fields})

	case "fill_simple":
		raw, _ := json.Marshal(args)
		var call struct {
			Fills []autofillagent.Fill `json:"fills"`
		}
		_ = json.Unmarshal(raw, &call)
		f.requested = call.Fills
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

func readArgs(args any) (label, value string, frame int) {
	raw, _ := json.Marshal(args)
	var call struct {
		Label string `json:"label"`
		Value string `json:"value"`
		Frame int    `json:"frame"`
	}
	_ = json.Unmarshal(raw, &call)
	return call.Label, call.Value, call.Frame
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

// Run must not discard a widget loop's earlier successes when a later widget's
// tool call fails outright: fillSimple's write and the first widget's commit
// already happened for real in the browser, and the caller needs to know that
// even though the run as a whole reports an error.
func TestRunReturnsThePartialReportWhenAWidgetFails(t *testing.T) {
	tools := &fakeWidgets{
		fields: []autofillagent.Field{
			{Label: "Email", Type: "email"},
			{Label: "Country", Type: "text", Combo: true},
			{Label: "Degree", Type: "text", Combo: true},
		},
		offers: map[string][]string{"Country": {"Germany"}, "Degree": {"BSc"}},
		errs:   map[string]error{"combobox.open Degree": errors.New("browser-tool socket hiccup")},
	}
	planner := &widgetPlanner{
		fills: []autofillagent.Fill{
			{Label: "Email", Value: profile()["email"]},
			{Label: "Country"},
			{Label: "Degree"},
		},
		choices: map[string]string{"Country": "Germany", "Degree": "BSc"},
	}

	rep, err := autofillagent.Run(context.Background(), tools, planner, profile())
	if err == nil {
		t.Fatal("Run succeeded despite the widget failure")
	}
	if !contains(rep.Filled, "Email") {
		t.Errorf("Filled = %v, want the already-written Email field kept despite the later failure", rep.Filled)
	}
	if !contains(rep.Filled, "Country") {
		t.Errorf("Filled = %v, want the widget driven before the failure kept", rep.Filled)
	}
	if contains(rep.Filled, "Degree") {
		t.Errorf("Filled = %v, the widget whose tool call failed must not be reported as filled", rep.Filled)
	}
}

// A plain field and a combobox sharing a label — e.g. the same "City" in the top
// frame and inside an ATS iframe — must be routed and reported independently by
// Field.Frame rather than collapsed onto whichever field happened to be last in
// the form. Before this fix, byLabel's single-field map meant the LAST field's
// Combo-ness decided the whole plan entry's routing, so the plain frame-0 field
// was silently dropped from fill_simple entirely and left unmapped despite the
// widget having been driven successfully.
func TestRunRoutesSameLabeledFieldsInDifferentFramesIndependently(t *testing.T) {
	tools := &fakeWidgets{
		fields: []autofillagent.Field{
			{Label: "City", Type: "text", Frame: 0},              // plain field, top frame
			{Label: "City", Type: "text", Combo: true, Frame: 3}, // widget, an ATS iframe
		},
		offers: map[string][]string{"City": {"Berlin"}},
	}
	planner := &widgetPlanner{
		fills:   []autofillagent.Fill{{Label: "City", Value: "Berlin"}},
		choices: map[string]string{"City": "Berlin"},
	}
	p := profile()
	p["location"] = "Berlin, Germany"

	rep, err := autofillagent.Run(context.Background(), tools, planner, p)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The plain frame-0 field was actually written, carrying its own frame.
	if len(tools.requested) != 1 || tools.requested[0].Label != "City" || tools.requested[0].Frame != 0 {
		t.Fatalf("fill_simple requested %+v, want one City fill tagged frame 0", tools.requested)
	}
	// The widget was driven scoped to its own frame, not the plain field's.
	if got := tools.frames["combobox.open City"]; got != 3 {
		t.Errorf("combobox.open frame = %d, want 3 (the widget's own frame)", got)
	}
	// Both physical fields ended up correctly reported as filled — neither the
	// widget's success masked the plain field's write, nor the reverse.
	filled := 0
	for _, label := range rep.Filled {
		if label == "City" {
			filled++
		}
	}
	if filled != 2 {
		t.Fatalf("Filled = %v, want City reported filled for both fields", rep.Filled)
	}
	if len(rep.Unmapped) != 0 {
		t.Fatalf("Unmapped = %v, want nothing left over", rep.Unmapped)
	}
}
