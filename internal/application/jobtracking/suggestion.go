package jobtracking

import (
	"time"

	"github.com/strelov1/freehire/internal/application/mailclassify"
)

// SuggestionEmail is the slice of a linked message the suggestion rule reads: when it arrived
// and what it was classified as. Not the whole mail row, so the rule is testable without a
// database and callable by the assistant.
type SuggestionEmail struct {
	ID         int64
	Signal     string
	ReceivedAt time.Time
}

// StageSuggestion is an offer to move an application to the stage its newest classified message
// implies. It names the message so the reader can check the claim rather than take it.
type StageSuggestion struct {
	Stage   string `json:"stage"`
	Signal  string `json:"signal"`
	EmailID int64  `json:"email_id"`
}

// SuggestStage reports the stage change the application's mail implies but has not made, or nil
// when there is nothing to offer.
//
// The automatic rules are narrow: mail advances a stage only strictly forward, only from a
// deterministic link, and never into a settled outcome, so an application can hold a rejection
// and still sit at `interview`. This turns that into a question the candidate answers.
//
// Only the newest classified message is considered; older ones are overtaken.
//
// lastStageSet is when the candidate last set the stage themselves (zero for never). A decision
// later than the message silences the offer, whichever stage they picked. That is the whole
// dismissal mechanism, read from the ledger so it cannot drift from a flag.
func SuggestStage(currentStage string, emails []SuggestionEmail, lastStageSet time.Time) *StageSuggestion {
	var newest *SuggestionEmail
	for i, e := range emails {
		if e.Signal == "" {
			continue // unclassified: every `external` message, by design
		}
		if newest == nil || e.ReceivedAt.After(newest.ReceivedAt) {
			newest = &emails[i]
		}
	}
	if newest == nil {
		return nil
	}

	// An unset stage reads as `applied` everywhere else (board, CountByStage); otherwise an
	// untouched application is offered a move to the stage it already occupies.
	if currentStage == "" {
		currentStage = "applied"
	}

	implied, _ := mailclassify.StageFor(mailclassify.StatusSignal(newest.Signal))
	if implied == "" || implied == currentStage {
		return nil
	}
	if !lastStageSet.IsZero() && lastStageSet.After(newest.ReceivedAt) {
		return nil
	}

	return &StageSuggestion{Stage: implied, Signal: newest.Signal, EmailID: newest.ID}
}
