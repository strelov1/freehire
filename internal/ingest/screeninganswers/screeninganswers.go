// Package screeninganswers holds the candidate's own answers to the six screening
// questions that repeat across ATS application forms and no CV can supply: which
// countries they are authorized to work in, whether they need visa sponsorship, their
// desired salary, their notice period, whether they are willing to relocate, and whether
// they are 18 or older — plus one standing consent, to an employer's privacy / personal-data
// processing terms, which the same forms ask for on every application.
//
// Not internal/identity/userprofile (search/targeting preferences, a different lifecycle), not
// internal/candidate/experience (an accumulating evidence bank for CV content), not
// internal/candidate/resumeextract (CV-derived — none of these six facts can be derived from a CV).
// See internal/ingest/screeninganswers/AGENTS.md for the full boundary reasoning.
package screeninganswers

import (
	"fmt"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/vocab"
)

// Answers is the candidate's stated screening answers. Every field is independently
// optional: a nil pointer (or a nil/empty slice for AuthorizedCountries) means the
// candidate has not stated that fact, never a guessed default.
type Answers struct {
	AuthorizedCountries   []string `json:"authorized_countries,omitempty"`
	VisaSponsorshipNeeded *bool    `json:"visa_sponsorship_needed,omitempty"`
	DesiredSalaryAmount   *int     `json:"desired_salary_amount,omitempty"`
	DesiredSalaryCurrency *string  `json:"desired_salary_currency,omitempty"`
	DesiredSalaryPeriod   *string  `json:"desired_salary_period,omitempty"`
	NoticePeriodDays      *int     `json:"notice_period_days,omitempty"`
	WillingToRelocate     *bool    `json:"willing_to_relocate,omitempty"`
	Age18OrOlder          *bool    `json:"age_18_or_older,omitempty"`
	// PrivacyConsent is the candidate's standing agreement to the privacy / personal-data
	// processing terms an employer's form makes a condition of applying. Not a fact about
	// the candidate like the fields above but a permission they grant: nil until they turn
	// it on, and false is a refusal the resolver honours by asking them each time.
	PrivacyConsent *bool `json:"privacy_consent,omitempty"`
}

// Sanitize normalizes the record in place: a country code the dictionary recognizes
// becomes freehire's canonical lowercase alpha-2 form; one it does not is left as-is
// (never dropped here) so Validate can still reject it — Sanitize normalizes, it does not
// gate-keep. The salary currency is uppercased the same way.
func (a *Answers) Sanitize() {
	for i, c := range a.AuthorizedCountries {
		if code := location.NormalizeCountry(c); code != "" {
			a.AuthorizedCountries[i] = code
		}
	}
	if a.DesiredSalaryCurrency != nil {
		upper := strings.ToUpper(strings.TrimSpace(*a.DesiredSalaryCurrency))
		a.DesiredSalaryCurrency = &upper
	}
}

// Validate reports the first reason this record cannot be persisted, naming the invalid
// value so a caller — the manual-edit endpoint or the assistant tool — can report it back
// actionably. Runs after Sanitize, so a currency that was only differently-cased has
// already been normalized.
func (a Answers) Validate() error {
	for _, c := range a.AuthorizedCountries {
		if location.NormalizeCountry(c) == "" {
			return fmt.Errorf("authorized_countries: %q is not a recognized country code", c)
		}
	}
	if a.DesiredSalaryCurrency != nil && !vocab.IsCurrencyCode(*a.DesiredSalaryCurrency) {
		return fmt.Errorf("desired_salary_currency: %q is not a three-letter ISO 4217 code", *a.DesiredSalaryCurrency)
	}
	if a.DesiredSalaryPeriod != nil && !slices.Contains(vocab.SalaryPeriodValues, *a.DesiredSalaryPeriod) {
		return fmt.Errorf("desired_salary_period: %q is not one of %v", *a.DesiredSalaryPeriod, vocab.SalaryPeriodValues)
	}
	if a.DesiredSalaryAmount != nil && *a.DesiredSalaryAmount <= 0 {
		return fmt.Errorf("desired_salary_amount: %d must be positive", *a.DesiredSalaryAmount)
	}
	if a.NoticePeriodDays != nil && *a.NoticePeriodDays < 0 {
		return fmt.Errorf("notice_period_days: %d must not be negative", *a.NoticePeriodDays)
	}
	return nil
}

// Field names one screening answer by its wire key — the vocabulary a caller uses to ask
// for an answer to be withdrawn.
type Field string

const (
	FieldAuthorizedCountries   Field = "authorized_countries"
	FieldVisaSponsorshipNeeded Field = "visa_sponsorship_needed"
	FieldDesiredSalaryAmount   Field = "desired_salary_amount"
	FieldDesiredSalaryCurrency Field = "desired_salary_currency"
	FieldDesiredSalaryPeriod   Field = "desired_salary_period"
	FieldNoticePeriodDays      Field = "notice_period_days"
	FieldWillingToRelocate     Field = "willing_to_relocate"
	FieldAge18OrOlder          Field = "age_18_or_older"
	FieldPrivacyConsent        Field = "privacy_consent"
)

// Fields lists every answer, in the order Answers declares them.
func Fields() []Field {
	return []Field{
		FieldAuthorizedCountries, FieldVisaSponsorshipNeeded, FieldDesiredSalaryAmount,
		FieldDesiredSalaryCurrency, FieldDesiredSalaryPeriod, FieldNoticePeriodDays,
		FieldWillingToRelocate, FieldAge18OrOlder, FieldPrivacyConsent,
	}
}

// ParseField resolves a wire key to its Field, refusing a name this record does not hold.
func ParseField(name string) (Field, error) {
	for _, f := range Fields() {
		if string(f) == name {
			return f, nil
		}
	}
	return "", fmt.Errorf("clear: %q is not a screening answer", name)
}

// states reports whether a stores a value for f.
func (a Answers) states(f Field) bool {
	switch f {
	case FieldAuthorizedCountries:
		return len(a.AuthorizedCountries) > 0
	case FieldVisaSponsorshipNeeded:
		return a.VisaSponsorshipNeeded != nil
	case FieldDesiredSalaryAmount:
		return a.DesiredSalaryAmount != nil
	case FieldDesiredSalaryCurrency:
		return a.DesiredSalaryCurrency != nil
	case FieldDesiredSalaryPeriod:
		return a.DesiredSalaryPeriod != nil
	case FieldNoticePeriodDays:
		return a.NoticePeriodDays != nil
	case FieldWillingToRelocate:
		return a.WillingToRelocate != nil
	case FieldAge18OrOlder:
		return a.Age18OrOlder != nil
	case FieldPrivacyConsent:
		return a.PrivacyConsent != nil
	}
	return false
}

// unstate returns f to unstated.
func (a *Answers) unstate(f Field) {
	switch f {
	case FieldAuthorizedCountries:
		a.AuthorizedCountries = nil
	case FieldVisaSponsorshipNeeded:
		a.VisaSponsorshipNeeded = nil
	case FieldDesiredSalaryAmount:
		a.DesiredSalaryAmount = nil
	case FieldDesiredSalaryCurrency:
		a.DesiredSalaryCurrency = nil
	case FieldDesiredSalaryPeriod:
		a.DesiredSalaryPeriod = nil
	case FieldNoticePeriodDays:
		a.NoticePeriodDays = nil
	case FieldWillingToRelocate:
		a.WillingToRelocate = nil
	case FieldAge18OrOlder:
		a.Age18OrOlder = nil
	case FieldPrivacyConsent:
		a.PrivacyConsent = nil
	}
}

// Merge overlays update onto existing: a field update states (a non-nil pointer, or a
// non-empty AuthorizedCountries) replaces the stored value; a field update leaves unset
// keeps whatever existing already held. Leaving a field out can therefore never withdraw
// it, so withdrawing is its own instruction: each field in cleared returns to unstated.
//
// It has to exist because these answers are not a form that sits still: auto-apply sends
// them to employers in the candidate's name. A candidate who moved away from a country, or
// stopped wanting to name a salary up front, must be able to take the answer back rather
// than only replace it — until 2026-10-30 they could not, and the old answer kept going out.
func Merge(existing, update Answers, cleared ...Field) Answers {
	merged := existing
	if len(update.AuthorizedCountries) > 0 {
		merged.AuthorizedCountries = update.AuthorizedCountries
	}
	if update.VisaSponsorshipNeeded != nil {
		merged.VisaSponsorshipNeeded = update.VisaSponsorshipNeeded
	}
	if update.DesiredSalaryAmount != nil {
		merged.DesiredSalaryAmount = update.DesiredSalaryAmount
	}
	if update.DesiredSalaryCurrency != nil {
		merged.DesiredSalaryCurrency = update.DesiredSalaryCurrency
	}
	if update.DesiredSalaryPeriod != nil {
		merged.DesiredSalaryPeriod = update.DesiredSalaryPeriod
	}
	if update.NoticePeriodDays != nil {
		merged.NoticePeriodDays = update.NoticePeriodDays
	}
	if update.WillingToRelocate != nil {
		merged.WillingToRelocate = update.WillingToRelocate
	}
	if update.Age18OrOlder != nil {
		merged.Age18OrOlder = update.Age18OrOlder
	}
	if update.PrivacyConsent != nil {
		merged.PrivacyConsent = update.PrivacyConsent
	}
	for _, f := range cleared {
		merged.unstate(f)
	}
	return merged
}
