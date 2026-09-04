// Package survey holds the candidate's self-reported segmentation answers: how far along
// their job search is, the single biggest thing in its way, and what they earn today.
//
// These are the questions the onboarding wizard could not ask while every one of its steps
// was a search facet. They describe the candidate to US, and to nobody else — deliberately
// not internal/identity/userprofile (that table IS the search filter, and its CHECK
// constraints mean a row cannot exist for someone who skipped the skills step), and
// deliberately not internal/ingest/screeninganswers (those six facts are what an EMPLOYER
// sees on an application form, and none of these three ever is).
//
// Current income keeps the same amount/currency/period triple screeninganswers uses for
// desired salary, so the two figures compare without conversion. The desired salary itself
// stays over there: it is one fact, and a second copy of it here would give the product two
// numbers with equal claim to being the answer.
package survey

import (
	"fmt"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// Responses is one candidate's survey record. Every field is independently optional: a nil
// pointer means the candidate has not stated that fact, never a guessed default and never
// a zero standing in for silence.
type Responses struct {
	JobSearchStage   *string `json:"job_search_stage,omitempty"`
	BiggestChallenge *string `json:"biggest_challenge,omitempty"`
	// BiggestChallengeNote is free text, and only vocab.JobChallengeOther admits it. A note
	// beside a coded challenge could contradict it, and nothing downstream would know which
	// to believe.
	BiggestChallengeNote  *string `json:"biggest_challenge_note,omitempty"`
	CurrentIncomeAmount   *int    `json:"current_income_amount,omitempty"`
	CurrentIncomeCurrency *string `json:"current_income_currency,omitempty"`
	CurrentIncomePeriod   *string `json:"current_income_period,omitempty"`
}

// Sanitize normalizes the record in place. It normalizes; it does not gate-keep — an
// invalid value is left intact so Validate can still name it.
//
// The two enum fields are deliberately NOT case-folded: the vocabularies are canonical and
// values are stored as-is (the same choice screeninganswers made for desired_salary_period),
// so folding here would quietly accept a spelling the wizard never sends and no later
// membership check would match.
func (a *Responses) Sanitize() {
	if a.CurrentIncomeCurrency != nil {
		upper := strings.ToUpper(strings.TrimSpace(*a.CurrentIncomeCurrency))
		a.CurrentIncomeCurrency = &upper
	}
	if a.BiggestChallengeNote != nil {
		// A note of whitespace is an empty note, which is what lets someone who picked
		// "other" and typed nothing still pass validation.
		trimmed := strings.TrimSpace(*a.BiggestChallengeNote)
		if trimmed == "" {
			a.BiggestChallengeNote = nil
		} else {
			a.BiggestChallengeNote = &trimmed
		}
	}
}

// Validate reports the first reason this record cannot be persisted, naming the offending
// field so the endpoint can report it back actionably. Runs after Sanitize, so a currency
// that was only differently-cased has already been normalized.
//
// An entirely empty record is valid: every wizard step is skippable, so "answered nothing"
// is a normal state rather than a failure.
func (a Responses) Validate() error {
	if a.JobSearchStage != nil && !slices.Contains(vocab.JobSearchStageValues, *a.JobSearchStage) {
		return fmt.Errorf("job_search_stage: %q is not one of %v", *a.JobSearchStage, vocab.JobSearchStageValues)
	}
	if a.BiggestChallenge != nil && !slices.Contains(vocab.JobChallengeValues, *a.BiggestChallenge) {
		return fmt.Errorf("biggest_challenge: %q is not one of %v", *a.BiggestChallenge, vocab.JobChallengeValues)
	}
	if a.BiggestChallengeNote != nil && (a.BiggestChallenge == nil || *a.BiggestChallenge != vocab.JobChallengeOther) {
		return fmt.Errorf("biggest_challenge_note: only accepted alongside biggest_challenge %q", vocab.JobChallengeOther)
	}
	if a.CurrentIncomeCurrency != nil && !vocab.IsCurrencyCode(*a.CurrentIncomeCurrency) {
		return fmt.Errorf("current_income_currency: %q is not a three-letter ISO 4217 code", *a.CurrentIncomeCurrency)
	}
	if a.CurrentIncomePeriod != nil && !slices.Contains(vocab.SalaryPeriodValues, *a.CurrentIncomePeriod) {
		return fmt.Errorf("current_income_period: %q is not one of %v", *a.CurrentIncomePeriod, vocab.SalaryPeriodValues)
	}
	if a.CurrentIncomeAmount != nil && *a.CurrentIncomeAmount <= 0 {
		return fmt.Errorf("current_income_amount: %d must be positive", *a.CurrentIncomeAmount)
	}
	return nil
}

// Merge overlays update onto existing: a stated field (non-nil pointer) replaces the stored
// value, an omitted one keeps it. There is no way to return a stated field to unstated —
// every field here is corrective in practice (a candidate restates a stage they have moved
// past, they do not withdraw one) and no wizard surface produces a withdrawal, so that
// omission trades a rare, low-value operation for a contract with no presence-detection in
// it. Same reasoning, and same conclusion, as screeninganswers.Merge.
func Merge(existing, update Responses) Responses {
	merged := existing
	if update.JobSearchStage != nil {
		merged.JobSearchStage = update.JobSearchStage
	}
	if update.BiggestChallenge != nil {
		// The note belongs to the "other" answer. Carrying it across a move to a coded
		// challenge would leave a record Validate rejects and no API call could repair,
		// since nothing here can clear a field on its own.
		if *update.BiggestChallenge != vocab.JobChallengeOther {
			merged.BiggestChallengeNote = nil
		}
		merged.BiggestChallenge = update.BiggestChallenge
	}
	if update.BiggestChallengeNote != nil {
		merged.BiggestChallengeNote = update.BiggestChallengeNote
	}
	if update.CurrentIncomeAmount != nil {
		merged.CurrentIncomeAmount = update.CurrentIncomeAmount
	}
	if update.CurrentIncomeCurrency != nil {
		merged.CurrentIncomeCurrency = update.CurrentIncomeCurrency
	}
	if update.CurrentIncomePeriod != nil {
		merged.CurrentIncomePeriod = update.CurrentIncomePeriod
	}
	return merged
}
