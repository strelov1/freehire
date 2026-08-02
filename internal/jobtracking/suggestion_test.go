package jobtracking_test

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/jobtracking"
)

var (
	day0 = time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	day1 = day0.AddDate(0, 0, 1)
	day2 = day0.AddDate(0, 0, 2)
)

func mail(id int64, signal string, at time.Time) jobtracking.SuggestionEmail {
	return jobtracking.SuggestionEmail{ID: id, Signal: signal, ReceivedAt: at}
}

// The case the whole thing exists for: a rejection arrives, the stage says `interview`, and
// nothing moves — by design, because settling an application is the candidate's call. Before
// this, the rule was invisible and the candidate was left to notice the discrepancy themselves.
func TestARejectionOnAnInterviewingApplicationIsOffered(t *testing.T) {
	got := jobtracking.SuggestStage("interview", []jobtracking.SuggestionEmail{
		mail(8814, "rejection", day1),
	}, time.Time{})

	if got == nil {
		t.Fatal("SuggestStage = nil, want a suggestion of `rejected`")
	}
	if got.Stage != "rejected" || got.Signal != "rejection" || got.EmailID != 8814 {
		t.Errorf("suggestion = %+v, want {rejected rejection 8814}", *got)
	}
}

func TestASignalAgreeingWithTheStageOffersNothing(t *testing.T) {
	got := jobtracking.SuggestStage("interview", []jobtracking.SuggestionEmail{
		mail(1, "interview_invitation", day1),
	}, time.Time{})
	if got != nil {
		t.Errorf("SuggestStage = %+v, want nil — the stage already says what the mail says", *got)
	}
}

// Every `external` message is unclassified by design: that tier costs us nothing precisely
// because we never run the classifier over it. It must not produce a suggestion out of thin air.
func TestUnclassifiedMailOffersNothing(t *testing.T) {
	got := jobtracking.SuggestStage("applied", []jobtracking.SuggestionEmail{
		mail(1, "", day1),
	}, time.Time{})
	if got != nil {
		t.Errorf("SuggestStage = %+v, want nil for unclassified mail", *got)
	}
}

// `info_request` and friends are to-dos, not movement — they imply no stage at all.
func TestASignalThatImpliesNoStageOffersNothing(t *testing.T) {
	for _, signal := range []string{"info_request", "incomplete_application", "other"} {
		got := jobtracking.SuggestStage("applied", []jobtracking.SuggestionEmail{
			mail(1, signal, day1),
		}, time.Time{})
		if got != nil {
			t.Errorf("SuggestStage(%s) = %+v, want nil", signal, *got)
		}
	}
}

// The ledger is what silences the offer — no dismissal flag anywhere. Any stage set after the
// message means the candidate has already answered the question, including by choosing a stage
// other than the one suggested.
func TestAStageSetAfterTheMessageSilencesTheOffer(t *testing.T) {
	got := jobtracking.SuggestStage("withdrawn", []jobtracking.SuggestionEmail{
		mail(1, "rejection", day1),
	}, day2)
	if got != nil {
		t.Errorf("SuggestStage = %+v, want nil — the candidate answered this on day 2", *got)
	}
}

func TestAStageSetBeforeTheMessageLeavesTheOfferStanding(t *testing.T) {
	got := jobtracking.SuggestStage("interview", []jobtracking.SuggestionEmail{
		mail(1, "rejection", day2),
	}, day1)
	if got == nil {
		t.Fatal("SuggestStage = nil, want the offer — the message is newer than the decision")
	}
}

// Only the newest classified message is asked. An older one that disagrees has already been
// overtaken by events.
func TestOnlyTheNewestClassifiedMessageIsOffered(t *testing.T) {
	got := jobtracking.SuggestStage("applied", []jobtracking.SuggestionEmail{
		mail(1, "rejection", day0),
		mail(2, "interview_invitation", day2),
		mail(3, "", day1),
	}, time.Time{})

	if got == nil {
		t.Fatal("SuggestStage = nil, want the newest classified message's suggestion")
	}
	if got.EmailID != 2 || got.Stage != "interview" {
		t.Errorf("suggestion = %+v, want the day-2 interview invitation", *got)
	}
}

// An application with applied_at set and no explicit stage IS `applied` everywhere else — the
// board files it under Applied, CountByStage counts it there. Offering to "move" it to the stage
// it already occupies is a prompt with nothing behind it, on the commonest kind of application
// there is: one applied to and never touched again.
func TestAnUnsetStageIsAlreadyAppliedAndOffersNothing(t *testing.T) {
	got := jobtracking.SuggestStage("", []jobtracking.SuggestionEmail{
		mail(1, "acknowledgement", day1),
	}, time.Time{})
	if got != nil {
		t.Errorf("SuggestStage = %+v, want nil — an unset stage already reads as applied", *got)
	}
}

// ...but a real move off it is still worth offering.
func TestAnUnsetStageStillTakesAForwardSuggestion(t *testing.T) {
	got := jobtracking.SuggestStage("", []jobtracking.SuggestionEmail{
		mail(1, "interview_invitation", day1),
	}, time.Time{})
	if got == nil || got.Stage != "interview" {
		t.Fatalf("SuggestStage = %v, want a suggestion of `interview`", got)
	}
}

func TestNoMailOffersNothing(t *testing.T) {
	if got := jobtracking.SuggestStage("applied", nil, time.Time{}); got != nil {
		t.Errorf("SuggestStage = %+v, want nil", *got)
	}
}
