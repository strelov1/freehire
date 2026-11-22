package atsapply

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/dict/answertopic"
)

// ResolvedField is one field ready to fill, with the exact value the widget expects: an
// option's platform value for a select/checkbox_group, the answer text verbatim otherwise.
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

// FullyResolved reports whether every required question was answered; Submit gates on it.
func (p Plan) FullyResolved() bool {
	return len(p.Unmapped) == 0
}

// answerKeyFor maps a field's DOM identifier to its key in the answers map
// (candidateprofile.Profile.Fields()). Greenhouse's identity fields already share the key
// names; candidate-location is its one alias. Ashby names its controls
// "_systemfield_name" / "_systemfield_email" (ingest/applyform/ashby.go).
// "_systemfield_resume" needs no entry: isResumeField matches it by label.
//
// "country" deliberately has no key: the profile has one combined `location` string, so a
// posting requiring a separate country always parks until the answer source widens.
var answerKeyFor = map[string]string{
	"first_name": "first_name",
	"last_name":  "last_name",
	// Workable's control names for the same two fields.
	"firstname": "first_name",
	"lastname":  "last_name",
	// Workable's `address`, required on the same form, answered from the profile's location.
	// By id and never by label: "Email address" and "IP address" are labels too.
	"address":   "location",
	"full_name": "full_name",
	// Recruitee's control name for the same field.
	"name":                    "full_name",
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
	"_systemfield_name":       "full_name",
	"_systemfield_email":      "email",
}

// Resolve matches every merged field against the candidate's known answers. A required
// field with no usable answer goes to Unmapped rather than being guessed; an optional one
// is left out of both lists.
//
// hasApprovedCV (Claimed.TailoredCVID != 0) is the one fact that lets a résumé file field
// resolve. Whether that CV can be rendered is Client.Submit's park path, not this
// function's.
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
		// An optional, unresolved field is neither filled nor reported.
	}
	return plan
}

// labelAnswerKeyFor matches a field's label against a narrow set of semantic categories,
// for custom employer questions whose opaque numeric id (Greenhouse's question_NNNNN)
// never matches by id even when candidateprofile holds the fact.
//
// Deliberately narrow: "Are you authorized to work in [this posting's country]" is NOT
// covered. authorized_countries is a list, not a yes/no for THIS country, the job's
// location is unavailable here, and freehire-apply treats work authorization as sensitive.
// visa_sponsorship_needed is a plain "Yes"/"No"
// (screeninganswers.Answers.AutofillFields), so it cannot give a wrong-country answer.
//
// Enforced, not just observed: the answer bank is a second route to a label, so
// matchBankAnswerKey refuses the same category through answertopic.Recallable.
var labelAnswerKeyFor = []struct {
	answerKey string
	keywords  []string // ALL must appear (case-insensitive) for the rule to fire
}{
	{"visa_sponsorship_needed", []string{"visa", "sponsor"}},
	// Ashby asks LinkedIn as a custom question with a random uuid id, so only a label rule
	// matches.
	{"linkedin", []string{"linkedin"}},
	// Desired salary: Greenhouse gives custom questions opaque ids, so only a label rule
	// reaches it.
	//
	// Four specific pairs rather than a bare {"salary"}: that would also match a question
	// about current pay, a different fact (candidate_survey holds it separately) that a desired
	// figure would misreport to the employer.
	//
	// `salary` stays on sensitive.go's list: a model may never draft a figure, but using the
	// one the candidate stored is fine.
	{"desired_salary", []string{"desired", "salary"}},
	{"desired_salary", []string{"desired", "compensation"}},
	{"desired_salary", []string{"salary", "expect"}},
	{"desired_salary", []string{"compensation", "expect"}},
	// Being of age: screening_answers.age_18_or_older holds the answer and forms give it an
	// opaque id. Each rule carries the number 18, which keeps an age RANGE question (a
	// demographic one) out.
	{"age_18_or_older", []string{"at least 18"}},
	{"age_18_or_older", []string{"over the age of 18"}},
	{"age_18_or_older", []string{"18 years of age or older"}},
	{"age_18_or_older", []string{"18 or older"}},
}

// labelExactAnswerKey answers a basic contact field asked under an opaque id ("Phone
// Number" as question_123, field-email) by its whole label, never by a keyword in it:
// "Referrer email" or "Preferred location" must not pick up the candidate's own.
var labelExactAnswerKey = map[string]string{
	"phone": "phone", "phone number": "phone", "mobile": "phone", "mobile number": "phone", "mobile phone": "phone",
	"first name": "first_name", "legal first name": "first_name", "what is your legal first name?": "first_name",
	"last name": "last_name", "legal last name": "last_name", "surname": "last_name", "what is your legal last name?": "last_name",
	"name": "full_name", "full name": "full_name", "legal name": "full_name", "full legal name": "full_name",
	"email": "email", "email address": "email",
	"location": "location", "current location": "location", "your location": "location", "location (city, state)": "location",
}

// matchExactLabelAnswerKey looks a label up in labelExactAnswerKey, ignoring case, the
// spacing around it and a required-field asterisk.
func matchExactLabelAnswerKey(label string) (string, bool) {
	key, ok := labelExactAnswerKey[strings.ToLower(strings.TrimSpace(strings.TrimRight(strings.TrimSpace(label), "*")))]
	return key, ok
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

// bankAnswerKeyPrefix namespaces banked answers inside the same map the deterministic
// facts use (a banked answer is an answer; the resolver need not know the source). The
// prefix avoids collisions: a topic is a folded question and can be any text, even "email".
//
// Must match the literal Profile.FieldsWithBankedAnswers() writes in
// internal/api/candidateprofile; plain Fields() carries none, so the bank reaches an
// application form and nothing else. They cannot share a constant (candidateprofile
// importing this package would invert the layering), so only the two comments tie them.
const bankAnswerKeyPrefix = "topic:"

// matchBankAnswerKey returns the answers-map key a field's question is banked under, if it
// can be keyed. Checked after the id and label rules: those are typed, validated facts and
// the bank is free text, so two sources must resolve the same way every time.
//
// It keys on questionText (reconcile.go), not the raw Label, because that is the text the
// answer was banked under.
//
// A work-authorization question is refused whatever the bank holds: the answer depends on
// THIS posting's country (see labelAnswerKeyFor) and one topic covers every posting worded
// that way. See answertopic.Recallable.
func matchBankAnswerKey(f MergedField) (string, bool) {
	question := questionText(f)
	if !answertopic.Recallable(question) {
		return "", false
	}
	topic, ok := answertopic.Of(question)
	if !ok {
		return "", false
	}
	return bankAnswerKeyPrefix + topic, true
}

// resolveOne resolves a single field's answer. A Multi field (checkbox group) matches at
// most ONE option's value: AnswerSource supplies only single-value facts today (see
// runner.go), so there is never a second candidate value.
func resolveOne(f MergedField, answers map[string]string, hasApprovedCV bool) (ResolvedField, string, bool) {
	if f.Kind == "file" {
		// Other file fields (a cover letter) have no artifact plumbing. The résumé resolves once
		// the entry has an approved tailored CV; Value stays empty here and Client.Submit sets it
		// after rendering, so resolution only decides whether the field can be filled.
		if isResumeField(f) && hasApprovedCV {
			return ResolvedField{ID: f.ID, Kind: f.Kind, Multi: f.Multi}, "", true
		}
		if isResumeField(f) {
			return ResolvedField{}, "no approved tailored CV for this attempt", false
		}
		return ResolvedField{}, "file uploads other than the résumé are not resolved by this package", false
	}

	// Before the keyed sources: a consent's value is whichever option agrees, which
	// matchOption cannot find from a stored "yes" when the form words it "I acknowledge". An
	// ungranted consent falls through to the bank like any question.
	if resolved, ok := resolvePrivacyConsent(f, answers); ok {
		return resolved, "", true
	}

	keys := answerKeysFor(f)
	if len(keys) == 0 {
		return ResolvedField{}, fmt.Sprintf("no known answer source for %q", f.ID), false
	}
	value, stated := firstStated(answers, keys)
	if !stated {
		// Named after the most specific key; the others were tried and are empty too.
		return ResolvedField{}, fmt.Sprintf("candidate has not stated %q", keys[0]), false
	}

	platformValue, matched := matchOption(f, value)
	if !matched {
		return ResolvedField{}, fmt.Sprintf("answer %q matches none of this field's offered options", value), false
	}
	return ResolvedField{ID: f.ID, Kind: f.Kind, Multi: f.Multi, Value: platformValue}, "", true
}

// answerKeysFor returns every answers-map key this field may be answered from, most
// specific first: its id, the narrow label rules, then the answer bank.
//
// A list resolved by firstStated rather than the first matching rule's key: a rule can
// match a fact the candidate has not stated ("Compensation expectations" matches the
// desired_salary rule), and the bank must still be consulted then. A typed fact still wins
// wherever it answers the question.
func answerKeysFor(f MergedField) []string {
	var keys []string
	if key, ok := answerKeyFor[f.ID]; ok {
		keys = append(keys, key)
	}
	// An opaque id (a custom question): match its label against the known categories. An id
	// match is more trustworthy and is never shadowed by this.
	if key, ok := matchLabelAnswerKey(f.Label); ok {
		keys = append(keys, key)
	}
	if key, ok := matchExactLabelAnswerKey(f.Label); ok {
		keys = append(keys, key)
	}
	// The bank: an answer given to this same question earlier. Last, so a typed fact always
	// wins (see matchBankAnswerKey).
	if key, ok := matchBankAnswerKey(f); ok {
		keys = append(keys, key)
	}
	return keys
}

// firstStated returns the value of the first key the candidate has stated. A blank value
// counts as unstated, as answerbank refuses to store one.
func firstStated(answers map[string]string, keys []string) (string, bool) {
	for _, key := range keys {
		if value := strings.TrimSpace(answers[key]); value != "" {
			return value, true
		}
	}
	return "", false
}

// resumeLabelWords are what a résumé upload is called in the languages employers label
// forms in; English-only matching parked an Ashby field labelled "Резюме".
var resumeLabelWords = []string{"resume", "résumé", "резюме", "cv", "lebenslauf", "currículum", "curriculum"}

// isResumeField reports whether a file upload is the résumé, the one file this package can
// produce. "resume" is Greenhouse's field id (applyform/display.go agrees). Not a
// catch-all: treating a cover letter or portfolio as the résumé would upload the CV into it.
func isResumeField(f MergedField) bool {
	// Ashby keys built-in fields as "_systemfield_<name>" (ingest/applyform/ashby.go), which a
	// bare-id check misses.
	id := strings.ToLower(strings.TrimSpace(f.ID))
	id = strings.TrimPrefix(id, "_systemfield_")
	if id == "resume" || id == "cv" {
		return true
	}
	// Word-bounded: "cv" is two letters that turn up inside ordinary words, and the label is
	// free text.
	for _, w := range strings.FieldsFunc(strings.ToLower(f.Label), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if slices.Contains(resumeLabelWords, w) {
			return true
		}
	}
	return false
}

// isCoverLetterTextField reports whether a free-text field asks for a cover letter, the
// free-text sibling of the file field "cover_letter" ("cover_letter_text" in
// applyform/display.go). The label check is a PREFIX match: the answer is the full letter
// used verbatim, so a substring would also fire on "If you don't have a cover letter,
// explain why". A real field's label opens with the words.
func isCoverLetterTextField(f MergedField) bool {
	if strings.EqualFold(strings.TrimSpace(f.ID), "cover_letter_text") {
		return true
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(f.Label)), "cover letter")
}

// matchOption resolves free text against a field's offered options, returning the platform
// value (not the label) of the option it matches case-insensitively. A field with no
// options takes the text verbatim. Shared by resolveOne and ResolveWithDrafting: an answer
// never uses an option the platform did not offer.
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
