package survey

import (
	"slices"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

func s(v string) *string { return &v }
func i(v int) *int       { return &v }

func TestValidateAcceptsAFullyStatedRecord(t *testing.T) {
	a := Responses{
		JobSearchStage:        s("searching"),
		BiggestChallenges:     []string{"english"},
		CurrentIncomeAmount:   i(5000),
		CurrentIncomeCurrency: s("USD"),
		CurrentIncomePeriod:   s("month"),
	}
	a.Sanitize()
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestValidateAcceptsAnEmptyRecord(t *testing.T) {
	// Answering nothing is a normal state, not an error: every step is skippable, and the
	// wizard PUTs whatever the user gave — which may be nothing at all.
	var a Responses
	a.Sanitize()
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate() on an empty record = %v, want nil", err)
	}
}

func TestValidateRejectsAnUnknownStage(t *testing.T) {
	a := Responses{JobSearchStage: s("thinking_about_it")}
	a.Sanitize()
	err := a.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error naming the field")
	}
	if !strings.Contains(err.Error(), "job_search_stage") {
		t.Errorf("error %q does not name the offending field", err)
	}
}

func TestValidateRejectsAnUnknownChallenge(t *testing.T) {
	a := Responses{BiggestChallenges: []string{"imposter_syndrome"}}
	a.Sanitize()
	if err := a.Validate(); err == nil {
		t.Fatal("Validate() = nil, want an error")
	}
}

func TestValidateRejectsAnEnumValueThatIsOnlyDifferentlyCased(t *testing.T) {
	// The vocabularies are canonical and values are stored as-is, mirroring how
	// screeninganswers treats desired_salary_period. Accepting "Searching" here would
	// persist a value no later membership check could match.
	a := Responses{JobSearchStage: s("Searching")}
	a.Sanitize()
	if err := a.Validate(); err == nil {
		t.Fatal("Validate() accepted a differently-cased stage, want rejection")
	}
}

func TestNoteIsAcceptedOnlyAlongsideOther(t *testing.T) {
	a := Responses{BiggestChallenges: []string{vocab.JobChallengeOther}, BiggestChallengeNote: s("visa paperwork")}
	a.Sanitize()
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate() on other+note = %v, want nil", err)
	}
}

func TestNoteAlongsideACodedChallengeIsRejected(t *testing.T) {
	// A note beside a coded answer could contradict it, and nothing downstream would know
	// which to believe.
	a := Responses{BiggestChallenges: []string{"english"}, BiggestChallengeNote: s("actually it is the interviews")}
	a.Sanitize()
	err := a.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want a rejection of note-beside-coded-challenge")
	}
	if !strings.Contains(err.Error(), "biggest_challenge_note") {
		t.Errorf("error %q does not name the offending field", err)
	}
}

func TestNoteWithoutAnyChallengeIsRejected(t *testing.T) {
	a := Responses{BiggestChallengeNote: s("something")}
	a.Sanitize()
	if err := a.Validate(); err == nil {
		t.Fatal("Validate() = nil, want a rejection of a note with no challenge")
	}
}

func TestSanitizeDropsAWhitespaceOnlyNote(t *testing.T) {
	// A note of spaces is an empty note. Dropping it in Sanitize is what lets a user who
	// picked "other" and typed nothing still pass validation.
	a := Responses{BiggestChallenges: []string{vocab.JobChallengeOther}, BiggestChallengeNote: s("   ")}
	a.Sanitize()
	if a.BiggestChallengeNote != nil {
		t.Fatalf("BiggestChallengeNote = %q, want nil after Sanitize", *a.BiggestChallengeNote)
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestSanitizeUppercasesTheCurrency(t *testing.T) {
	a := Responses{CurrentIncomeCurrency: s(" usd ")}
	a.Sanitize()
	if a.CurrentIncomeCurrency == nil || *a.CurrentIncomeCurrency != "USD" {
		t.Fatalf("CurrentIncomeCurrency = %v, want USD", a.CurrentIncomeCurrency)
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestValidateRejectsAMalformedCurrency(t *testing.T) {
	a := Responses{CurrentIncomeCurrency: s("DOLLARS")}
	a.Sanitize()
	if err := a.Validate(); err == nil {
		t.Fatal("Validate() = nil, want an error")
	}
}

func TestValidateRejectsAnUnknownPeriod(t *testing.T) {
	a := Responses{CurrentIncomePeriod: s("fortnight")}
	a.Sanitize()
	if err := a.Validate(); err == nil {
		t.Fatal("Validate() = nil, want an error")
	}
}

func TestValidateRejectsANonPositiveIncome(t *testing.T) {
	// Zero is not "unstated" — unstated is a nil pointer. A stated zero is a mistake.
	for _, amount := range []int{0, -1} {
		a := Responses{CurrentIncomeAmount: i(amount)}
		a.Sanitize()
		if err := a.Validate(); err == nil {
			t.Errorf("Validate() on amount %d = nil, want an error", amount)
		}
	}
}

func TestMergeKeepsFieldsTheUpdateOmits(t *testing.T) {
	existing := Responses{JobSearchStage: s("searching"), BiggestChallenges: []string{"english"}}
	merged := Merge(existing, Responses{BiggestChallenges: []string{"technical_interviews"}})

	if merged.JobSearchStage == nil || *merged.JobSearchStage != "searching" {
		t.Errorf("JobSearchStage = %v, want the stored value to survive an update that omits it", merged.JobSearchStage)
	}
	if !slices.Equal(merged.BiggestChallenges, []string{"technical_interviews"}) {
		t.Errorf("BiggestChallenges = %v, want the update's value", merged.BiggestChallenges)
	}
}

func TestMergeReplacingTheChallengeDropsAStaleNote(t *testing.T) {
	// The note belongs to the "other" answer. Moving off "other" without clearing it would
	// leave a note attached to a coded challenge — exactly the state Validate rejects, and
	// it would be unreachable through the API since there is no clear operation.
	existing := Responses{BiggestChallenges: []string{vocab.JobChallengeOther}, BiggestChallengeNote: s("visa paperwork")}
	merged := Merge(existing, Responses{BiggestChallenges: []string{"english"}})

	if merged.BiggestChallengeNote != nil {
		t.Fatalf("BiggestChallengeNote = %q, want it dropped when the challenge moves off %q", *merged.BiggestChallengeNote, vocab.JobChallengeOther)
	}
	if err := merged.Validate(); err != nil {
		t.Fatalf("merged record does not validate: %v", err)
	}
}

func TestValidateAcceptsSeveralChallenges(t *testing.T) {
	a := Responses{BiggestChallenges: []string{"visa_sponsorship", "english", "job_fit"}}
	a.Sanitize()
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestSanitizeDropsRepeatedChallengesKeepingOrder(t *testing.T) {
	a := Responses{BiggestChallenges: []string{"english", "job_fit", "english"}}
	a.Sanitize()
	if !slices.Equal(a.BiggestChallenges, []string{"english", "job_fit"}) {
		t.Fatalf("BiggestChallenges = %v, want duplicates dropped in first-seen order", a.BiggestChallenges)
	}
}

func TestSanitizeTreatsAnEmptyListAsUnstated(t *testing.T) {
	// An empty list carries no answer; storing it would read as "answered nothing" next to
	// a record that never answered at all, and the two must be the same fact.
	a := Responses{BiggestChallenges: []string{}}
	a.Sanitize()
	if a.BiggestChallenges != nil {
		t.Fatalf("BiggestChallenges = %v, want nil", a.BiggestChallenges)
	}
}

func TestJustStartingStandsAlone(t *testing.T) {
	// "Nothing is hard yet" contradicts naming something that is.
	a := Responses{BiggestChallenges: []string{vocab.JobChallengeJustStarting, "english"}}
	a.Sanitize()
	err := a.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want just_starting beside another answer rejected")
	}
	if !strings.Contains(err.Error(), "biggest_challenges") {
		t.Errorf("error %q does not name the offending field", err)
	}
}

func TestNoteIsAcceptedWhenOtherIsAmongSeveral(t *testing.T) {
	a := Responses{BiggestChallenges: []string{"english", vocab.JobChallengeOther}, BiggestChallengeNote: s("visa paperwork")}
	a.Sanitize()
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestMergeKeepsTheNoteWhileOtherStaysPicked(t *testing.T) {
	existing := Responses{BiggestChallenges: []string{vocab.JobChallengeOther}, BiggestChallengeNote: s("visa paperwork")}
	merged := Merge(existing, Responses{BiggestChallenges: []string{"english", vocab.JobChallengeOther}})
	if merged.BiggestChallengeNote == nil {
		t.Fatal("BiggestChallengeNote = nil, want it kept while other is still among the answers")
	}
}
