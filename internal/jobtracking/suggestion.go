package jobtracking

import (
	"time"

	"github.com/strelov1/freehire/internal/mailclassify"
)

// SuggestionEmail is the slice of a linked message the suggestion rule reads: when it arrived
// and what it was classified as. Deliberately not the whole mail row — this rule has no business
// with bodies, senders or read state, and taking only these three keeps it testable without a
// database and callable by the assistant, which reads the service directly.
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
// It exists because the automatic rules are deliberately narrow and were therefore silent. Mail
// advances a stage only strictly forward, only from a deterministic link, and never into a
// settled outcome — so an application can accumulate seven messages, including a plain
// rejection, and sit at `interview` with nothing anywhere saying why. This turns that silence
// into a question the candidate answers with one press.
//
// Only the newest classified message is considered: an older disagreement has been overtaken by
// whatever came after it.
//
// lastStageSet is when the candidate last set the stage themselves (zero for never). A decision
// later than the message silences the offer, whichever stage they picked — that is the whole
// dismissal mechanism, and it is the ledger rather than a flag of our own because the ledger
// already records exactly this and cannot drift from itself.
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

	// An unset stage on a recorded application reads as `applied` everywhere else — the board
	// files it under Applied, CountByStage counts it there — so it must read as `applied` here
	// too. Otherwise the commonest application there is (applied to, never touched again) gets
	// offered a move to the stage it already occupies the moment its acknowledgement arrives.
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
