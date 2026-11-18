// Package autofillagent fills a job-application form in the user's browser: it
// reads the form through the browser-tool wire, maps the stored profile onto it,
// and writes back the fields it can justify.
//
// The model decides the mapping but may not invent: every written value must be
// grounded in the profile, enforced here rather than trusted to the model.
package autofillagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/strelov1/freehire/internal/dict/answertopic"
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
	// Form is which <form> inside Frame the control belongs to; a careers page can
	// carry an application and a job-alert signup in the same document.
	Form int `json:"form"`
}

// Upload is a CV upload the page offers, as `read_form` reported it. Never a fill
// target: it marks which form on the page is the application, which field counts
// cannot tell from a job-alert signup.
type Upload struct {
	Frame int `json:"frame"`
	Form  int `json:"form"`
}

// Fill is one entry of the plan: the value to write into the control carrying
// this label. Frame and Form are set by splitByKind from the field the fill
// resolved to, not by the planner, so a same-labeled control elsewhere is not hit.
//
// Both are needed: Frame separates an ATS iframe from the page around it, Form
// separates the application from a signup in the same document. A fill naming
// neither is matched by label alone and lands in whichever form is walked first.
type Fill struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Frame int    `json:"frame"`
	Form  int    `json:"form"`
}

// Profile is the user's canonical autofill fields, keyed as /me/autofill-profile
// returns them (full_name, email, ...): candidateprofile.Profile.Fields().
//
// It must stay a fixed set. groundedValue admits any value sharing a word-run with
// ANY entry here, and Plan sends the whole map to the provider, so each entry widens
// what the model may write and what leaves the system. Banked screening answers
// (free text, including demographic and disability questions) are kept out for both
// reasons.
type Profile map[string]string

// Banked is the candidate's answer bank, topic → answer, keyed by internal/dict/answertopic
// as internal/candidate/answerbank stores it.
//
// It never meets the model or groundedValue (see Profile). A question the plan left
// unanswered whose topic the bank holds is filled from it deterministically.
type Banked map[string]string

// Report is what the run tells the user.
type Report struct {
	// Filled are the labels that now carry a profile value.
	Filled []string `json:"filled"`
	// Deferred are the custom-widget comboboxes this slice cannot fill yet.
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
	// Separate from Plan because a custom widget renders no options until opened:
	// Plan names which widgets are worth opening, Choose picks from the list read
	// off the page.
	Choose(ctx context.Context, question Field, options []string, profile Profile) (string, error)
}

// Run outcomes that are not faults. Sentinels so the caller can tell them from gateway
// or model failures, which belong in the error inbox; these two read as advice.
var (
	// ErrNoFillableFields is a page with no form on it — nothing to do, nothing wrong.
	ErrNoFillableFields = errors.New("no fillable fields on this page")
	// ErrUnavailable is autofill on a deployment with no language model configured.
	ErrUnavailable = errors.New("agent autofill is unavailable: no language model is configured")
)

// Run drives one autofill turn: read the form, plan, recall from the bank, fill, report.
func Run(ctx context.Context, tools Tools, planner Planner, profile Profile, banked Banked) (Report, error) {
	fields, uploads, err := readForm(ctx, tools)
	if err != nil {
		return Report{}, err
	}
	if len(fields) == 0 {
		return Report{}, ErrNoFillableFields
	}
	fields = scopeToApplication(fields, uploads)

	planned, err := planner.Plan(ctx, fields, profile)
	if err != nil {
		return Report{}, err
	}
	typed, widgets := splitByKind(fields, planned)
	typed = groundedIn(profile, typed)
	recalledTyped, recalledWidgets, recalledChoices := recall(fields, banked, typed, widgets)

	outcomes, err := fillSimple(ctx, tools, append(typed, recalledTyped...))
	if err != nil {
		return Report{}, err
	}
	// Report on failure too: fillSimple's writes already landed in the browser.
	driven := map[string]string{}
	if err := driveWidgets(ctx, tools, planChooser(planner, profile), widgets, driven); err != nil {
		return report(fields, outcomes, driven), err
	}
	if err := driveWidgets(ctx, tools, bankChooser(recalledChoices), recalledWidgets, driven); err != nil {
		return report(fields, outcomes, driven), err
	}
	return report(fields, outcomes, driven), nil
}

// recall answers, from the bank, the questions the plan left alone. It runs AFTER the
// plan so the profile wins over the bank's free text (as in internal/api/atsapply's
// resolver). A planned value the grounding gate refused counts as left alone.
//
// A work-authorization question is never recalled (answertopic.Recallable): its answer
// depends on the posting's country.
//
// Recalled widgets' answers are keyed by widgetKey for bankChooser.
func recall(fields []Field, banked Banked, typed []Fill, widgets []Field) ([]Fill, []Field, map[string]string) {
	if len(banked) == 0 {
		return nil, nil, nil
	}
	named := make(map[string]bool, len(typed)+len(widgets))
	for _, fill := range typed {
		named[widgetKey(fill.Label, fill.Frame)] = true
	}
	for _, widget := range widgets {
		named[widgetKey(widget.Label, widget.Frame)] = true
	}

	var fills []Fill
	var recalledWidgets []Field
	answers := map[string]string{}
	for _, field := range fields {
		key := widgetKey(field.Label, field.Frame)
		if named[key] || !answertopic.Recallable(field.Label) {
			continue
		}
		topic, ok := answertopic.Of(field.Label)
		if !ok {
			continue
		}
		answer, ok := banked[topic]
		if !ok {
			continue
		}
		if field.Combo {
			recalledWidgets = append(recalledWidgets, field)
			answers[key] = answer
			continue
		}
		fills = append(fills, Fill{Label: field.Label, Value: answer, Frame: field.Frame, Form: field.Form})
	}
	return fills, recalledWidgets, answers
}

// splitByKind separates the plan into values that can simply be written and the
// widgets that have to be driven. Only widgets the plan named are driven, to avoid
// a round trip and a model call per unanswerable combobox.
//
// Fields sharing a label (repeated questions, or the same label in two frames) are
// grouped, not deduped. The plan addresses a fill by label alone, so a fill is routed
// to EVERY field carrying that label, each tagged with its own Frame and Form.
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
			typed = append(typed, Fill{Label: fill.Label, Value: fill.Value, Frame: field.Frame, Form: field.Form})
		}
	}
	return typed, widgets
}

func readForm(ctx context.Context, tools Tools) ([]Field, []Upload, error) {
	raw, err := tools.Call(ctx, "read_form", nil)
	if err != nil {
		return nil, nil, err
	}
	var result struct {
		Fields  []Field  `json:"fields"`
		Uploads []Upload `json:"uploads"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, nil, fmt.Errorf("read_form returned unreadable fields: %w", err)
	}
	return result.Fields, result.Uploads, nil
}

// scopeToApplication narrows an observation to the one form the application is
// asking, identified by the CV upload sitting in it. Without it a page with an
// application and a job-alert signup gets both filled (each has an "Email").
//
// Mirrors the extension's function of the same name; Go cannot call into it, so a
// change to either belongs in both.
//
// Frame and Form together identify the group: an ATS iframe numbers its own forms
// from zero. When the upload's group holds no questions (a page rendering them
// outside its form element) every field is kept, since filling nothing is worse.
func scopeToApplication(fields []Field, uploads []Upload) []Field {
	if len(uploads) == 0 {
		return fields
	}
	target := uploads[0]
	scoped := make([]Field, 0, len(fields))
	for _, f := range fields {
		if f.Frame == target.Frame && f.Form == target.Form {
			scoped = append(scoped, f)
		}
	}
	if len(scoped) == 0 {
		return fields
	}
	return scoped
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

// Widget outcomes this package decides itself, besides the statuses the extension's
// primitives report (opened, no_option, verified, mismatch, ...).
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

// chooser picks one of a widget's options for its question. It returns the choice, or
// "" with the widget status that says why there is none (widgetUnanswered, …).
type chooser func(ctx context.Context, question Field, options []string) (choice, refusal string, err error)

// planChooser asks the planner, then requires the pick to be one of the widget's
// options and supported by the profile.
func planChooser(planner Planner, profile Profile) chooser {
	return func(ctx context.Context, question Field, options []string) (string, string, error) {
		choice, err := planner.Choose(ctx, question, options, profile)
		if err != nil {
			return "", "", err
		}
		if choice == "" {
			return "", widgetUnanswered, nil
		}
		// No approximating to the nearest option: that is how "No" becomes "Norfolk Island".
		if !offers(options, choice) {
			return "", widgetNotOffered, nil
		}
		if !groundedValue(profile, choice) {
			return "", widgetUngrounded, nil
		}
		return choice, "", nil
	}
}

// bankChooser picks the option the banked answer names, in the widget's own spelling
// (what select and verify compare against), and nothing merely near it.
func bankChooser(answers map[string]string) chooser {
	return func(_ context.Context, question Field, options []string) (string, string, error) {
		answer := answers[widgetKey(question.Label, question.Frame)]
		for _, option := range options {
			if normalize(option) == normalize(answer) {
				return option, "", nil
			}
		}
		return "", widgetNotOffered, nil
	}
}

// driveWidgets works each widget the way a person does (open, read options, choose,
// commit, confirm) and records each outcome in driven. A step that misbehaves stops
// that widget: nothing is written into a widget whose state is not understood.
//
// On failure it returns the error with driven holding the widgets driven so far.
func driveWidgets(ctx context.Context, tools Tools, choose chooser, questions []Field, driven map[string]string) error {
	for _, question := range questions {
		status, err := driveWidget(ctx, tools, choose, question)
		if err != nil {
			return err
		}
		driven[widgetKey(question.Label, question.Frame)] = status
	}
	return nil
}

// widgetKey keys a widget's outcome by label and frame, so same-labeled widgets in
// different frames do not overwrite each other.
func widgetKey(label string, frame int) string {
	return fmt.Sprintf("%s\x00%d", label, frame)
}

func driveWidget(ctx context.Context, tools Tools, choose chooser, question Field) (string, error) {
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

	choice, refusal, err := choose(ctx, question, offered.Options)
	if err != nil {
		return "", err
	}
	if choice == "" {
		return refusal, nil
	}

	selected, err := comboStep(ctx, tools, "combobox.select", question.Label, choice, question.Frame)
	if err != nil {
		return "", err
	}
	if selected.Status != "selected" {
		return selected.Status, nil
	}

	// `verified` is the only status the report counts as filled.
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
// given frame, which tells apart same-labeled widgets (see Field.Frame).
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

// groundedIn drops any planned value that cannot be traced back to the profile, so
// "no fabricated values" does not rest on the prompt. A value is grounded when it
// appears within one of the profile's values ("Berlin" from "Berlin, Germany") or
// contains one.
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
// Matching is on whole words, in either direction. Raw substrings would be too
// generous for short answers: "no" sits inside "norway", so "No" would be grounded
// for a candidate living in Oslo.
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
	// fill_simple's outcomes carry no frame, so simple fields are looked up by label
	// alone; driven is keyed label+frame (see widgetKey).
	byLabel := make(map[string]string, len(outcomes))
	for _, o := range outcomes {
		byLabel[o.Label] = o.Status
	}

	rep := Report{Filled: []string{}, Deferred: []string{}, Unmapped: []string{}}
	// Required unfilled fields lead: ATS forms add dozens of optional controls and
	// the reader sees only the head of the list.
	var optional []string
	for _, field := range fields {
		// The report names fields by label; a control without one cannot be named.
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

// placement is the report's three-way split: filled, not fillable yet, left for the user.
type placement int

const (
	wasFilled placement = iota
	notFillableYet
	leftForUser
)

// place decides where one field belongs. `written` is what fill_simple reported
// for it, `driven` what the widget loop reported.
//
// Only a verified commit counts as filled; an unverified write never does.
func place(field Field, written, driven string) placement {
	if written == "filled" {
		return wasFilled
	}
	if field.Combo {
		switch driven {
		case "verified":
			return wasFilled
		case "", widgetUnanswered, widgetUngrounded, widgetNotOffered:
			// The answer is missing, not the ability to drive the widget. "" is a widget
			// the plan never targeted: the profile answers nothing it offers.
			return leftForUser
		default:
			// Would not open, committed something else, or offers its options only
			// over the network.
			return notFillableYet
		}
	}
	// The page re-rendered between read and write, and the control turned out a widget.
	if written == "deferred_combobox" {
		return notFillableYet
	}
	// Never planned, or the browser could not write it (no matching option, control gone).
	return leftForUser
}
