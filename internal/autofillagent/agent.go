// Package autofillagent fills a job-application form in the user's browser: it
// reads the form through the browser-tool wire, maps the user's stored profile
// onto it, and writes back the fields it can justify.
//
// It is deliberately not the old deterministic filler (a fixed label dictionary):
// the agent sees the actual form and decides. What it may *not* do is invent —
// every value it writes has to be grounded in the profile, which is enforced here
// rather than trusted to the model.
package autofillagent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// Field is one control as `read_form` reported it.
type Field struct {
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Value    string   `json:"value"`
	Required bool     `json:"required"`
	Combo    bool     `json:"combo"`
	Options  []string `json:"options,omitempty"`
	Frame    int      `json:"frame"`
}

// Fill is one entry of the plan: the value to write into the control carrying
// this label. Frame names which of the page's frames the target field was read
// from (see Field.Frame) — set by splitByKind from the field it resolved the fill
// against, not by the planner, so a same-labeled control in a different frame is
// not addressed by a Fill meant for another.
type Fill struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Frame int    `json:"frame"`
}

// Profile is the user's canonical autofill fields, keyed as
// /me/autofill-profile returns them (full_name, email, …).
type Profile map[string]string

// Report is what the run tells the user.
type Report struct {
	// Filled are the labels that now carry a profile value.
	Filled []string `json:"filled"`
	// Deferred are the custom-widget comboboxes this slice cannot fill yet —
	// reported rather than written into with a wrong value.
	Deferred []string `json:"deferred"`
	// Unmapped are the fields the agent found no basis for in the profile.
	Unmapped []string `json:"unmapped"`
}

// Tools issues browser-tool calls and returns their raw results.
type Tools interface {
	Call(ctx context.Context, tool string, args any) (json.RawMessage, error)
}

// Planner maps the observed form onto the profile.
type Planner interface {
	Plan(ctx context.Context, fields []Field, profile Profile) ([]Fill, error)
	// Choose picks one of a widget's options for the question it answers, or ""
	// when the profile does not answer it.
	//
	// Separate from Plan because a custom widget renders no options until it is
	// opened — measured on two live Greenhouse forms, not one of their 40
	// comboboxes carries an option list while closed. So Plan cannot name a value
	// it has never seen; it names which widgets are worth opening, and the choice
	// is made here against the list actually read from the page.
	Choose(ctx context.Context, question Field, options []string, profile Profile) (string, error)
}

// Run drives one autofill turn: read the form, plan, fill, report.
func Run(ctx context.Context, tools Tools, planner Planner, profile Profile) (Report, error) {
	fields, err := readForm(ctx, tools)
	if err != nil {
		return Report{}, err
	}
	if len(fields) == 0 {
		return Report{}, fmt.Errorf("no fillable fields on this page")
	}

	planned, err := planner.Plan(ctx, fields, profile)
	if err != nil {
		return Report{}, err
	}
	typed, widgets := splitByKind(fields, planned)

	outcomes, err := fillSimple(ctx, tools, groundedIn(profile, typed))
	if err != nil {
		return Report{}, err
	}
	// driveWidgets returns whatever it drove before a failure, not just on success:
	// fillSimple's writes above already landed in the browser, and dropping them
	// from the report on a later widget's failure would tell the user nothing
	// succeeded when part of the form is, in fact, already filled.
	driven, err := driveWidgets(ctx, tools, planner, profile, widgets)
	if err != nil {
		return report(fields, outcomes, driven), err
	}
	return report(fields, outcomes, driven), nil
}

// splitByKind separates the plan into values that can simply be written and the
// widgets that have to be driven. Only widgets the plan named are driven: a form
// with 27 of them would otherwise cost 27 needless round trips and 27 needless
// model calls to discover the profile answers none of them.
//
// Fields sharing a label are grouped, not deduped to one: a form can carry two
// controls under the same label — a repeated question in a multi-entry section,
// or the same label in two different frames, which Field.Frame exists to
// disambiguate. The plan addresses a fill by label alone (the model has no way to
// name a frame), so a fill naming that label is routed to EVERY field carrying it,
// each tagged with its own Frame — never collapsed onto whichever field happened
// to be last in the page, which is what silently misrouted a plain field's value
// to a combobox target (or vice versa) whenever the two shared a label.
func splitByKind(fields []Field, planned []Fill) (typed []Fill, widgets []Field) {
	byLabel := make(map[string][]Field, len(fields))
	for _, field := range fields {
		byLabel[field.Label] = append(byLabel[field.Label], field)
	}
	for _, fill := range planned {
		matches, ok := byLabel[fill.Label]
		if !ok {
			typed = append(typed, fill)
			continue
		}
		for _, field := range matches {
			if field.Combo {
				widgets = append(widgets, field)
				continue
			}
			typed = append(typed, Fill{Label: fill.Label, Value: fill.Value, Frame: field.Frame})
		}
	}
	return typed, widgets
}

func readForm(ctx context.Context, tools Tools) ([]Field, error) {
	raw, err := tools.Call(ctx, "read_form", nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Fields []Field `json:"fields"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("read_form returned unreadable fields: %w", err)
	}
	return result.Fields, nil
}

type outcome struct {
	Label  string `json:"label"`
	Status string `json:"status"`
}

func fillSimple(ctx context.Context, tools Tools, fills []Fill) ([]outcome, error) {
	if len(fills) == 0 {
		return nil, nil
	}
	raw, err := tools.Call(ctx, "fill_simple", map[string]any{"fills": fills})
	if err != nil {
		return nil, err
	}
	var result struct {
		Outcomes []outcome `json:"outcomes"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("fill_simple returned unreadable outcomes: %w", err)
	}
	return result.Outcomes, nil
}

// Widget outcomes this package decides itself, alongside the statuses the
// extension's primitives report (opened, no_option, verified, mismatch, …).
const (
	// The profile answers nothing the widget offers.
	widgetUnanswered = "not_answered"
	// The model's pick is not traceable to the profile.
	widgetUngrounded = "not_grounded"
	// The model's pick is not among the options the widget offers.
	widgetNotOffered = "not_offered"
	// Open, and offering nothing: a typeahead that fetches over the network.
	widgetNoOptions = "no_options"
)

// driveWidgets works each named widget the way a person does — open it, read what
// it offers, choose, commit, confirm — and reports what became of each. A step
// that does not do what it should stops that widget rather than pressing on:
// nothing is written into a widget whose state is not understood.
//
// On a widget's failure it returns what it drove so far alongside the error,
// rather than discarding it: the earlier widgets in questions already committed
// their choices in the browser, and the caller (Run) reports that real progress
// instead of losing it behind the error.
func driveWidgets(ctx context.Context, tools Tools, planner Planner, profile Profile, questions []Field) (map[string]string, error) {
	driven := make(map[string]string, len(questions))
	for _, question := range questions {
		status, err := driveWidget(ctx, tools, planner, profile, question)
		if err != nil {
			return driven, err
		}
		driven[widgetKey(question.Label, question.Frame)] = status
	}
	return driven, nil
}

// widgetKey composite-keys a driven widget's outcome by label and frame, so two
// widgets sharing a label in different frames (see splitByKind) are tracked as the
// distinct controls they are instead of one overwriting the other's status.
func widgetKey(label string, frame int) string {
	return fmt.Sprintf("%s\x00%d", label, frame)
}

func driveWidget(ctx context.Context, tools Tools, planner Planner, profile Profile, question Field) (string, error) {
	opened, err := comboStep(ctx, tools, "combobox.open", question.Label, "", question.Frame)
	if err != nil {
		return "", err
	}
	if opened.Status != "opened" {
		return opened.Status, nil
	}

	offered, err := comboStep(ctx, tools, "combobox.options", question.Label, "", question.Frame)
	if err != nil {
		return "", err
	}
	if len(offered.Options) == 0 {
		return widgetNoOptions, nil
	}

	choice, err := planner.Choose(ctx, question, offered.Options, profile)
	if err != nil {
		return "", err
	}
	if choice == "" {
		return widgetUnanswered, nil
	}
	// The widget's own list is the authority: a pick that is merely close is not
	// approximated, because committing the nearest option is how a question
	// wanting "No" ends up holding "Norfolk Island".
	if !offers(offered.Options, choice) {
		return widgetNotOffered, nil
	}
	if !groundedValue(profile, choice) {
		return widgetUngrounded, nil
	}

	selected, err := comboStep(ctx, tools, "combobox.select", question.Label, choice, question.Frame)
	if err != nil {
		return "", err
	}
	if selected.Status != "selected" {
		return selected.Status, nil
	}

	// The commit is only real once the widget says so; `verified` is the single
	// status the report is allowed to count as filled.
	verified, err := comboStep(ctx, tools, "combobox.verify", question.Label, choice, question.Frame)
	if err != nil {
		return "", err
	}
	return verified.Status, nil
}

type comboReply struct {
	Status    string   `json:"status"`
	Options   []string `json:"options"`
	Committed string   `json:"committed"`
}

// comboStep runs one combobox primitive against the widget carrying label in the
// given frame — frame is what lets the browser tell apart two same-labeled
// widgets in different frames/forms (see Field.Frame), the same way it already
// does for read_form.
func comboStep(ctx context.Context, tools Tools, tool, label, value string, frame int) (comboReply, error) {
	raw, err := tools.Call(ctx, tool, map[string]any{"label": label, "value": value, "frame": frame})
	if err != nil {
		return comboReply{}, err
	}
	var reply comboReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return comboReply{}, fmt.Errorf("%s returned an unreadable reply: %w", tool, err)
	}
	return reply, nil
}

func offers(options []string, choice string) bool {
	for _, option := range options {
		if normalize(option) == normalize(choice) {
			return true
		}
	}
	return false
}

// groundedIn drops any planned value that cannot be traced back to the profile.
// A model asked to fill a form will happily supply a plausible answer to a
// question the profile never answered; this is what keeps "no fabricated values"
// a property of the system rather than a hope about the prompt. A value counts as
// grounded when it appears within one of the profile's own values (so "Berlin"
// from "Berlin, Germany" survives) or contains one.
func groundedIn(profile Profile, fills []Fill) []Fill {
	kept := make([]Fill, 0, len(fills))
	for _, fill := range fills {
		if groundedValue(profile, fill.Value) {
			kept = append(kept, fill)
		}
	}
	return kept
}

// groundedValue reports whether the profile actually supports this answer.
//
// Matching is on whole words, in either direction, so "Germany" is supported by
// "Berlin, Germany" and "Berlin" is supported by it too. Comparing raw substrings
// instead would be far too generous once the answer is short: "no" sits inside
// "norway", so a candidate living in Oslo would have the answer "No" grounded for
// any question at all — and a widget's options are routinely this short.
func groundedValue(profile Profile, value string) bool {
	want := words(value)
	if len(want) == 0 {
		return false
	}
	for _, known := range profile {
		have := words(known)
		if runOf(have, want) || runOf(want, have) {
			return true
		}
	}
	return false
}

// runOf reports whether needle appears in haystack as a consecutive run of words.
func runOf(haystack, needle []string) bool {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return false
	}
	for start := 0; start+len(needle) <= len(haystack); start++ {
		if slices.Equal(haystack[start:start+len(needle)], needle) {
			return true
		}
	}
	return false
}

func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func report(fields []Field, outcomes []outcome, driven map[string]string) Report {
	// outcomes (fill_simple's reply) carries no frame — the wire's own contract, not
	// something this function can widen — so a simple (non-combo) field is still
	// looked up by label alone here. driven (this package's own widget-loop
	// bookkeeping) has no such limit and is keyed label+frame; see widgetKey.
	byLabel := make(map[string]string, len(outcomes))
	for _, o := range outcomes {
		byLabel[o.Label] = o.Status
	}

	rep := Report{Filled: []string{}, Deferred: []string{}, Unmapped: []string{}}
	// The unfilled fields are split so the required ones lead: a real ATS form
	// contributes dozens of optional controls (Greenhouse adds a checkbox per
	// country), and the reader sees the head of the list, not all of it.
	var optional []string
	for _, field := range fields {
		// Everything here is addressed by label, so a control without one is not
		// something the report can meaningfully name to the user.
		if strings.TrimSpace(field.Label) == "" {
			continue
		}
		switch place(field, byLabel[field.Label], driven[widgetKey(field.Label, field.Frame)]) {
		case wasFilled:
			rep.Filled = append(rep.Filled, field.Label)
		case notFillableYet:
			rep.Deferred = append(rep.Deferred, field.Label)
		case leftForUser:
			if field.Required {
				rep.Unmapped = append(rep.Unmapped, field.Label)
			} else {
				optional = append(optional, field.Label)
			}
		}
	}
	rep.Unmapped = append(rep.Unmapped, optional...)
	return rep
}

// placement is the three-way distinction the report makes: what the agent filled,
// what it cannot fill yet, and what is left for the user to answer.
type placement int

const (
	wasFilled placement = iota
	notFillableYet
	leftForUser
)

// place decides where one field belongs. `written` is what fill_simple reported
// for it, `driven` what the widget loop reported.
//
// Only a verified commit counts as filled — an unverified write is a failure and
// never a fill, or the user is told a field is done while it is still empty.
func place(field Field, written, driven string) placement {
	if written == "filled" {
		return wasFilled
	}
	if field.Combo {
		switch driven {
		case "verified":
			return wasFilled
		case widgetUnanswered, widgetUngrounded, widgetNotOffered:
			// The widget works; it is the answer that is missing.
			return leftForUser
		default:
			// Never targeted, would not open, committed something else, or offers
			// its options only over the network.
			return notFillableYet
		}
	}
	// A control the form did not present as a widget but the browser found to be
	// one — the page re-rendered between reading it and writing to it.
	if written == "deferred_combobox" {
		return notFillableYet
	}
	// Never planned, or planned and the browser could not write it (no matching
	// option, the control vanished) — either way the user still has to fill it.
	return leftForUser
}
