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
	"strings"
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
// this label.
type Fill struct {
	Label string `json:"label"`
	Value string `json:"value"`
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
	fills := groundedIn(profile, planned)

	outcomes, err := fillSimple(ctx, tools, fills)
	if err != nil {
		return Report{}, err
	}
	return report(fields, outcomes), nil
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

// groundedIn drops any planned value that cannot be traced back to the profile.
// A model asked to fill a form will happily supply a plausible answer to a
// question the profile never answered; this is what keeps "no fabricated values"
// a property of the system rather than a hope about the prompt. A value counts as
// grounded when it appears within one of the profile's own values (so "Berlin"
// from "Berlin, Germany" survives) or contains one.
func groundedIn(profile Profile, fills []Fill) []Fill {
	known := make([]string, 0, len(profile))
	for _, value := range profile {
		if v := normalize(value); v != "" {
			known = append(known, v)
		}
	}

	kept := make([]Fill, 0, len(fills))
	for _, fill := range fills {
		if grounded(normalize(fill.Value), known) {
			kept = append(kept, fill)
		}
	}
	return kept
}

func grounded(value string, known []string) bool {
	if value == "" {
		return false
	}
	for _, k := range known {
		if strings.Contains(k, value) || strings.Contains(value, k) {
			return true
		}
	}
	return false
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func report(fields []Field, outcomes []outcome) Report {
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
		switch {
		case byLabel[field.Label] == "filled":
			rep.Filled = append(rep.Filled, field.Label)
		case field.Combo || byLabel[field.Label] == "deferred_combobox":
			rep.Deferred = append(rep.Deferred, field.Label)
		default:
			// Never planned, or planned and the browser could not write it (no
			// matching option, the control vanished) — either way the user still
			// has to fill it themselves.
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
