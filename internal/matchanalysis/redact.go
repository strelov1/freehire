package matchanalysis

import (
	"encoding/json"

	"github.com/strelov1/freehire/internal/pii"
)

// contactsFromStructured pulls the authoritative contact values out of the structured-résumé
// JSON so the Redactor masks them wherever they surface (the raw CV and the structured blob
// alike), even if the model detector renders them differently. Empty/unparseable → no known
// contacts, and the redactor relies on the CV text + model spans alone.
func contactsFromStructured(structuredJSON string) pii.Contacts {
	var s struct {
		FullName string   `json:"full_name"`
		Email    string   `json:"email"`
		Phone    string   `json:"phone"`
		Links    []string `json:"links"`
	}
	if json.Unmarshal([]byte(structuredJSON), &s) != nil {
		return pii.Contacts{}
	}
	return pii.Contacts{FullName: s.FullName, Email: s.Email, Phone: s.Phone, Links: s.Links}
}

// redactingEmit wraps emit so every outbound event has its PII restored for the user, on a
// COPY — the internal reqs/verdict threaded into later-stage prompts stay masked, so nothing
// re-leaks. A nil redactor passes events through unchanged.
func redactingEmit(red *pii.Redactor, emit func(Event)) func(Event) {
	if red == nil {
		return emit
	}
	return func(ev Event) {
		ev.Thinking = red.Restore(ev.Thinking)
		if ev.Requirements != nil {
			ev.Requirements = restoreRequirements(red, ev.Requirements)
		}
		if ev.Analysis != nil {
			a := restoreAnalysis(red, *ev.Analysis)
			ev.Analysis = &a
		}
		emit(ev)
	}
}

// restoreAnalysis returns a copy of a with every user-facing string restored. Slices are
// rebuilt so the caller never mutates the masked chain state shared by later stages.
func restoreAnalysis(red *pii.Redactor, a Analysis) Analysis {
	if red == nil {
		return a
	}
	dims := make([]Dimension, len(a.Dimensions))
	for i, d := range a.Dimensions {
		d.Comment = red.Restore(d.Comment)
		dims[i] = d
	}
	a.Dimensions = dims
	a.RequirementMatch = restoreRequirements(red, a.RequirementMatch)
	a.Strengths = restoreStrings(red, a.Strengths)
	a.Gaps = restoreStrings(red, a.Gaps)
	a.Recommendation = red.Restore(a.Recommendation)
	return a
}

func restoreRequirements(red *pii.Redactor, reqs []Requirement) []Requirement {
	if reqs == nil {
		return nil
	}
	out := make([]Requirement, len(reqs))
	for i, r := range reqs {
		r.Text = red.Restore(r.Text)
		r.Evidence = red.Restore(r.Evidence)
		out[i] = r
	}
	return out
}

func restoreStrings(red *pii.Redactor, in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = red.Restore(s)
	}
	return out
}
