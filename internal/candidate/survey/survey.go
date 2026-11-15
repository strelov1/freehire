// Package survey holds the candidate's self-reported segmentation answers: how far along
// their job search is, the things in its way, and what they earn today.
//
// These describe the candidate to us and nobody else, so they are deliberately not in
// internal/identity/userprofile (that table IS the search filter) nor in
// internal/ingest/screeninganswers (what an EMPLOYER sees on an application form).
//
// Current income uses the same amount/currency/period triple screeninganswers uses for
// desired salary, so the two compare without conversion. Desired salary stays there; a
// second copy would give two numbers with equal claim.
package survey

import (
	"fmt"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// Responses is one candidate's survey record. Every field is independently optional: nil
// means not stated, never a default or a zero standing in for silence.
type Responses struct {
	JobSearchStage *string `json:"job_search_stage,omitempty"`
	// BiggestChallenges are members of vocab.JobChallengeValues, in the order picked. Nil
	// means unstated; Sanitize folds an empty list into nil so the two never differ.
	BiggestChallenges []string `json:"biggest_challenges,omitempty"`
	// BiggestChallengeNote is free text, admitted only while vocab.JobChallengeOther is
	// among the challenges; beside coded answers alone it could contradict them.
	BiggestChallengeNote  *string `json:"biggest_challenge_note,omitempty"`
	CurrentIncomeAmount   *int    `json:"current_income_amount,omitempty"`
	CurrentIncomeCurrency *string `json:"current_income_currency,omitempty"`
	CurrentIncomePeriod   *string `json:"current_income_period,omitempty"`
}

// Sanitize normalizes the record in place; an invalid value is left intact so Validate
// can name it.
//
// The two enum fields are deliberately NOT case-folded: values are stored as-is
// (like screeninganswers' desired_salary_period), and folding would accept a spelling
// no later membership check matches.
func (a *Responses) Sanitize() {
	if a.CurrentIncomeCurrency != nil {
		upper := strings.ToUpper(strings.TrimSpace(*a.CurrentIncomeCurrency))
		a.CurrentIncomeCurrency = &upper
	}
	if len(a.BiggestChallenges) == 0 {
		a.BiggestChallenges = nil
	} else {
		// Keep the first of a repeated pick, preserving the order picked.
		seen := make(map[string]bool, len(a.BiggestChallenges))
		unique := make([]string, 0, len(a.BiggestChallenges))
		for _, c := range a.BiggestChallenges {
			if !seen[c] {
				seen[c] = true
				unique = append(unique, c)
			}
		}
		a.BiggestChallenges = unique
	}
	if a.BiggestChallengeNote != nil {
		// A whitespace note is empty, so "other" with no text still validates.
		trimmed := strings.TrimSpace(*a.BiggestChallengeNote)
		if trimmed == "" {
			a.BiggestChallengeNote = nil
		} else {
			a.BiggestChallengeNote = &trimmed
		}
	}
}

// Validate reports the first reason this record cannot be persisted, naming the field.
// Run it after Sanitize. An entirely empty record is valid: every wizard step is skippable.
func (a Responses) Validate() error {
	if a.JobSearchStage != nil && !slices.Contains(vocab.JobSearchStageValues, *a.JobSearchStage) {
		return fmt.Errorf("job_search_stage: %q is not one of %v", *a.JobSearchStage, vocab.JobSearchStageValues)
	}
	for _, c := range a.BiggestChallenges {
		if !slices.Contains(vocab.JobChallengeValues, c) {
			return fmt.Errorf("biggest_challenges: %q is not one of %v", c, vocab.JobChallengeValues)
		}
	}
	if len(a.BiggestChallenges) > 1 && slices.Contains(a.BiggestChallenges, vocab.JobChallengeJustStarting) {
		return fmt.Errorf("biggest_challenges: %q cannot be picked with another answer", vocab.JobChallengeJustStarting)
	}
	if a.BiggestChallengeNote != nil && !slices.Contains(a.BiggestChallenges, vocab.JobChallengeOther) {
		return fmt.Errorf("biggest_challenge_note: only accepted while biggest_challenges includes %q", vocab.JobChallengeOther)
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

// Merge overlays update onto existing: a non-nil field replaces the stored value, an
// omitted one keeps it. A stated field cannot return to unstated; candidates restate, they
// do not withdraw (same as screeninganswers.Merge).
func Merge(existing, update Responses) Responses {
	merged := existing
	if update.JobSearchStage != nil {
		merged.JobSearchStage = update.JobSearchStage
	}
	if update.BiggestChallenges != nil {
		// The note belongs to "other"; keeping it past a list without "other" would leave a
		// record Validate rejects and no call could repair.
		if !slices.Contains(update.BiggestChallenges, vocab.JobChallengeOther) {
			merged.BiggestChallengeNote = nil
		}
		merged.BiggestChallenges = update.BiggestChallenges
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
