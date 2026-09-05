// Package screeninganswers holds the candidate's own answers to the six screening
// questions that repeat across ATS application forms and no CV can supply: which
// countries they are authorized to work in, whether they need visa sponsorship, their
// desired salary, their notice period, whether they are willing to relocate, and whether
// they are 18 or older.
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

// Merge overlays update onto existing: a field update states (a non-nil pointer, or a
// non-empty AuthorizedCountries) replaces the stored value; a field update leaves unset
// keeps whatever existing already held. There is no way to explicitly clear a
// previously-stated field back to unstated through Merge — every field this domain holds
// is corrective in practice (a candidate restates a changed salary, they do not withdraw
// one), so that omission trades a rare, low-value operation for a simpler contract.
func Merge(existing, update Answers) Answers {
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
	return merged
}
