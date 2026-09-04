package atsapply

import (
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/application/autoapply"
)

// ResolvedField is one field ready to fill, with the exact value the widget expects — an
// option's platform VALUE for a select/checkbox_group, the answer text verbatim otherwise.
type ResolvedField struct {
	ID    string
	Kind  string
	Multi bool
	Value string
}

// Plan is the outcome of resolving every merged field against a candidate's known answers.
type Plan struct {
	Fields   []ResolvedField
	Unmapped []autoapply.UnmappedField
}

// FullyResolved reports whether every required question was answered — the gate Submit
// checks before it ever fills or presses submit on a real form.
func (p Plan) FullyResolved() bool {
	return len(p.Unmapped) == 0
}

// answerKeyFor maps a field's DOM identifier to the key it is looked up under in the
// answers map (internal/candidateprofile.Profile.Fields()). Greenhouse happens to name its
// own standard identity fields (first_name, last_name, email, phone) the same as the answer
// keys already, so most of this map is the identity case; candidate-location is the one
// alias the 2026-09-02 spike measured.
//
// NOT covered here, deliberately: "country" has no answer key at all — the candidate
// profile carries one combined `location` string, not a separate country. On Greenhouse,
// where `country` renders as its own required field on nearly every posting, this means a
// posting requiring it always parks in this package's current scope. Widening the answer
// source (a dedicated country fact, or Tier C/LLM-drafted answers) is future work, not a bug
// here — see design.md's Non-Goals.
var answerKeyFor = map[string]string{
	"first_name":              "first_name",
	"last_name":               "last_name",
	"full_name":               "full_name",
	"email":                   "email",
	"phone":                   "phone",
	"location":                "location",
	"candidate-location":      "location",
	"linkedin":                "linkedin",
	"github":                  "github",
	"portfolio":               "portfolio",
	"authorized_countries":    "authorized_countries",
	"visa_sponsorship_needed": "visa_sponsorship_needed",
	"desired_salary":          "desired_salary",
	"notice_period":           "notice_period",
	"willing_to_relocate":     "willing_to_relocate",
	"age_18_or_older":         "age_18_or_older",
}

// Resolve matches every merged field against the candidate's known answers. A required
// field with no usable answer is reported in Unmapped rather than guessed; an optional one
// with no answer is simply left out of both lists — nothing to fill, nothing wrong either.
//
// hasApprovedCV is whether the queue entry carries an approved tailored CV (Claimed.
// TailoredCVID != 0) — the one fact that lets a résumé file field resolve at all (openspec/
// changes/auto-apply-tailored-resume). It says nothing about WHETHER that CV can actually be
// rendered; a render failure at submit time is Client.Submit's own park path, not this
// function's concern.
func Resolve(fields []MergedField, answers map[string]string, hasApprovedCV bool) Plan {
	var plan Plan
	for _, f := range fields {
		resolved, reason, ok := resolveOne(f, answers, hasApprovedCV)
		switch {
		case ok:
			plan.Fields = append(plan.Fields, resolved)
		case f.Required:
			plan.Unmapped = append(plan.Unmapped, autoapply.UnmappedField{
				ID: f.ID, Label: f.Label, Required: true, Reason: reason,
			})
		}
		// An optional, unresolved field: neither filled nor reported. Nothing here drafts
		// an answer for it (no Tier C yet), so leaving it blank is a valid outcome.
	}
	return plan
}

// labelAnswerKeyFor matches a field's LABEL text against a curated, narrow set of semantic
// categories, for the custom employer-authored questions a numeric id (Greenhouse's
// question_NNNNN, and the equivalent on other platforms) can never match by id alone —
// even when candidateprofile holds the exact fact the question is asking for. Measured
// against a live posting (task 7.1 in openspec/changes/auto-apply-worker/tasks.md): visa
// sponsorship was asked as a custom question there.
//
// Deliberately narrow. "Are you authorized to work in [this posting's country]" is NOT
// covered here even though it is the same shape of gap: candidateprofile's
// authorized_countries is a list of countries the candidate holds work authorization in,
// not a yes/no answer to "in THIS specific country" — answering it would need the job's own
// location, which nothing here has, and freehire-apply (a sibling, more mature
// implementation) treats work-authorization questions as sensitive and never auto-answers
// them either. visa_sponsorship_needed has no such ambiguity: it is stored as a plain
// "Yes"/"No" (internal/screeninganswers.Answers.AutofillFields), so matching it here can
// never produce a wrong-country answer the way authorization would.
var labelAnswerKeyFor = []struct {
	answerKey string
	keywords  []string // ALL must appear (case-insensitive) for the rule to fire
}{
	{"visa_sponsorship_needed", []string{"visa", "sponsor"}},
}

// matchLabelAnswerKey returns the answer key a field's label matches, if any.
func matchLabelAnswerKey(label string) (string, bool) {
	lower := strings.ToLower(label)
	for _, rule := range labelAnswerKeyFor {
		matched := true
		for _, kw := range rule.keywords {
			if !strings.Contains(lower, kw) {
				matched = false
				break
			}
		}
		if matched {
			return rule.answerKey, true
		}
	}
	return "", false
}

// resolveOne resolves a single field's answer. For a Multi field (a checkbox group taking
// several answers) this matches at most ONE option's value, even when more than one would
// be correct — AnswerSource only ever supplies single-value identity/work-authorization
// facts today (see runner.go's AnswerSource doc), so there is never more than one candidate
// value to match against a Multi field's options in the first place. Widening AnswerSource
// to a source that can state several values for one question is what would make this a real
// gap; until then it is a direct consequence of the answer shape, not a shortcut taken here.
func resolveOne(f MergedField, answers map[string]string, hasApprovedCV bool) (ResolvedField, string, bool) {
	if f.Kind == "file" {
		// A cover letter (or any other file field) still has no artifact-resolution
		// plumbing here — see the package doc. The résumé/CV upload resolves once the
		// entry carries an approved tailored CV (openspec/changes/auto-apply-tailored-
		// resume); Value is empty here and set by Client.Submit once the CV is actually
		// rendered, right before fillAndSubmit runs — resolution only decides WHETHER the
		// field can be filled, not what bytes it gets.
		if isResumeField(f) && hasApprovedCV {
			return ResolvedField{ID: f.ID, Kind: f.Kind, Multi: f.Multi}, "", true
		}
		if isResumeField(f) {
			return ResolvedField{}, "no approved tailored CV for this attempt", false
		}
		return ResolvedField{}, "file uploads other than the résumé are not resolved by this package", false
	}

	key, known := answerKeyFor[f.ID]
	if !known {
		// The id is opaque (a custom employer-authored question) — fall back to matching
		// its label against the narrow set of known semantic categories. An id match, when
		// one exists, is always more specific/trustworthy and is never shadowed by this.
		key, known = matchLabelAnswerKey(f.Label)
	}
	if !known {
		return ResolvedField{}, fmt.Sprintf("no known answer source for %q", f.ID), false
	}
	value, stated := answers[key]
	if !stated || strings.TrimSpace(value) == "" {
		return ResolvedField{}, fmt.Sprintf("candidate has not stated %q", key), false
	}

	platformValue, matched := matchOption(f, value)
	if !matched {
		return ResolvedField{}, fmt.Sprintf("answer %q matches none of this field's offered options", value), false
	}
	return ResolvedField{ID: f.ID, Kind: f.Kind, Multi: f.Multi, Value: platformValue}, "", true
}

// isResumeField reports whether a file-kind field is the résumé/CV upload — the only file
// field this package ever resolves — as opposed to a cover letter or any other attachment.
// "resume" is Greenhouse's own field id for it (internal/ingest/applyform/display.go's
// vocabulary agrees: "resume"/"resume_text" vs. "cover_letter"/"cover_letter_text"); the
// label fallback covers a custom-labeled field the id alone would miss.
func isResumeField(f MergedField) bool {
	if strings.EqualFold(strings.TrimSpace(f.ID), "resume") {
		return true
	}
	lower := strings.ToLower(f.Label)
	return strings.Contains(lower, "resume") || strings.Contains(lower, "résumé")
}

// matchOption resolves free text against a field's offered options, returning the
// PLATFORM value (not the label) for whichever option it case-insensitively matches. A
// field with no enumerated options at all takes the text verbatim — there is nothing to
// validate it against. Shared by resolveOne (a deterministic answer) and draft.go's
// ResolveWithDrafting (a drafted one): the "never answer with an option the platform did
// not offer" rule applies identically to both, per
// auto-apply-question-drafting's spec.
func matchOption(f MergedField, text string) (value string, ok bool) {
	text = strings.TrimSpace(text)
	if len(f.Options) == 0 {
		return text, true
	}
	for _, opt := range f.Options {
		if strings.EqualFold(strings.TrimSpace(opt.Label), text) {
			return opt.Value, true
		}
	}
	return "", false
}
