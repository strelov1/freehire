package atsapply

import "strings"

// privacyConsentKey is the answers-map key the candidate's standing privacy consent arrives
// under (internal/ingest/screeninganswers.Answers.AutofillFields, which projects only a
// GRANTED consent, as "yes").
const privacyConsentKey = "privacy_consent"

// privacyConsentIDs are the platform ids that name the consent field outright. Workable's
// labelless `gdpr` boolean was on 23,835 captured forms on 2026-10-25, required on every one,
// and carries no label for privacyTerms to read.
var privacyConsentIDs = map[string]bool{"gdpr": true}

// privacyTerms name an employer's privacy / personal-data terms, in the languages the
// catalogue's forms are written in. Substring stems, like sensitiveTerms.
//
// French "confidentialité" keeps its accent on purpose: unaccented, it would also match the
// English "confidentiality", which on these forms is a non-compete or NDA question — a
// statement about the candidate's contracts, not a consent.
var privacyTerms = []string{
	"privacy", "gdpr", "rgpd", "dsgvo", "lgpd",
	"personal data", "personal information", "data protection", "processing of my data",
	"datenschutz", "personenbezogen",
	"données personnelles", "confidentialité",
	"persoonsgegevens",
	"datos personales", "privacidad",
	"dados pessoais", "privacidade",
	"персональн",
}

// notPrivacyConsentTerms are what a privacy-worded field is ABOUT when it is not the
// condition of applying. Each is measured on the captured forms of 2026-10-25, where they
// share the page — and often the label — with the real consent:
//   - keeping the candidate's data for, or contacting them about, FUTURE openings (Lever's
//     "Yes, <company> can contact me about future job opportunities ... Privacy policy",
//     on ~10k forms) — a talent pool the candidate joins only by choice;
//   - SMS, text and marketing messages, which cite the privacy policy they are sent under;
//   - a background or criminal-record check, which cites the screening company's own
//     privacy address;
//   - a demographic or diversity disclosure, which is the candidate's alone to give.
var notPrivacyConsentTerms = []string{
	"futur", "zukünft", "toekomst", "talent pool", "talent community", "contact me",
	"sms", "text message", "messag", "marketing", "newsletter", "promotion",
	"background", "criminal",
	"gender", "race", "ethnic", "veteran", "disab", "demographic", "sexual orientation",
	"religi", "genetic", "diversi", "etni", "deficiên", "discapacidad", "behinderung",
}

// isPrivacyConsentField reports whether f asks the candidate to accept the employer's
// privacy / personal-data processing terms as a condition of applying.
//
// Only a REQUIRED checkbox or single-choice select qualifies. Required, because the
// candidate's permission covers the condition of applying and nothing more: an optional
// box beside it is an extra the form offers, and leaving it unticked is a fine outcome.
// Checkbox or select, because the answer is the act of agreeing, and a text field naming
// the privacy policy is asking for something else. The exclusions apply to an id match too:
// a field named `gdpr` whose label asks about marketing is a marketing opt-in.
func isPrivacyConsentField(f MergedField) bool {
	if !f.Required || (f.Kind != "checkbox_group" && f.Kind != "select") {
		return false
	}
	if containsAnyTerm(f.Label, notPrivacyConsentTerms) {
		return false
	}
	return privacyConsentIDs[strings.ToLower(f.ID)] || containsAnyTerm(f.Label, privacyTerms)
}

// affirmativePrefixes open an option that agrees. Matched against the START of the option's
// label, so "I agree to the terms" counts and "Yes, but ..." still reads as yes.
var affirmativePrefixes = []string{
	"yes", "i agree", "agree", "i acknowledge", "acknowledge", "i accept", "accept",
	"i consent", "consent", "i confirm", "confirm", "i have read", "i understand",
	"understood", "ok",
	"sim", "sí", "si", "oui", "ja", "да",
	"concordo", "aceito", "acepto", "de acuerdo", "j'accepte", "j’accepte", "d'accord",
	"akkoord", "ik ga akkoord", "einverstanden", "ich stimme", "согласен", "согласна",
}

// negativeMarkers turn an otherwise agreeing option into a refusal ("I do not agree").
var negativeMarkers = []string{
	"not ", "n't", "don’t", " no ", "não", "nao ", "non ", "nicht", "niet", "нет", "не ",
}

// affirmativeOption is the value that agrees to a consent field. A checkbox with no declared
// options (Workable's boolean) takes "yes", which the fill path ticks; a lone checkbox takes
// its one option, since checking it is the only thing it can do — unless that option says it
// refuses. Otherwise exactly one option must read as agreeing — two candidates, or none (a
// select that declared no options included), and the field parks for the candidate rather
// than having one picked for them.
func affirmativeOption(f MergedField) (string, bool) {
	if f.Kind == "checkbox_group" {
		switch len(f.Options) {
		case 0:
			return "yes", true
		case 1:
			return f.Options[0].Value, !isNegative(f.Options[0].Label)
		}
	}
	var found string
	matches := 0
	for _, opt := range f.Options {
		if isAffirmative(opt.Label) {
			found = opt.Value
			matches++
		}
	}
	return found, matches == 1
}

// isAffirmative reports whether an option's label agrees: it opens with an agreeing word or
// phrase that ends there (so "ok" does not match "okta"), and carries no negation.
func isAffirmative(label string) bool {
	if isNegative(label) {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(label))
	for _, p := range affirmativePrefixes {
		rest, ok := strings.CutPrefix(lower, p)
		if ok && (rest == "" || strings.ContainsRune(" ,.!", []rune(rest)[0])) {
			return true
		}
	}
	return false
}

// isNegative reports whether an option's label carries a negation ("I do not agree", "No").
func isNegative(label string) bool {
	padded := " " + strings.ToLower(strings.TrimSpace(label)) + " "
	for _, neg := range negativeMarkers {
		if strings.Contains(padded, neg) {
			return true
		}
	}
	return false
}

// resolvePrivacyConsent answers a consent field from the candidate's standing consent, when
// they have granted one. Reports false — leaving the field to the ordinary sources, the
// answer bank included — for any field that is not a consent, for a candidate who has not
// granted it, and for a field whose agreeing option cannot be told apart.
func resolvePrivacyConsent(f MergedField, answers map[string]string) (ResolvedField, bool) {
	if answers[privacyConsentKey] != "yes" || !isPrivacyConsentField(f) {
		return ResolvedField{}, false
	}
	value, ok := affirmativeOption(f)
	if !ok {
		return ResolvedField{}, false
	}
	return ResolvedField{ID: f.ID, Kind: f.Kind, Multi: f.Multi, Value: value}, true
}
