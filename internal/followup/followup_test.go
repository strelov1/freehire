package followup

import (
	"strings"
	"testing"
)

func base() Input {
	return Input{
		Role:       "Senior Backend Engineer",
		Company:    "Appinio",
		DaysSilent: 24,
		Stage:      "applied",
	}
}

func TestDraftNamesTheRoleAndCompany(t *testing.T) {
	d := Draft(base())

	if !strings.Contains(d.Subject, "Senior Backend Engineer") {
		t.Errorf("subject = %q, want the role in it", d.Subject)
	}
	if !strings.Contains(d.Body, "Appinio") {
		t.Errorf("body = %q, want the company in it", d.Body)
	}
}

func TestDraftIsDeterministic(t *testing.T) {
	a, b := Draft(base()), Draft(base())

	if a != b {
		t.Errorf("two drafts of the same input differ:\n%+v\n%+v", a, b)
	}
}

func TestDraftStatesTheElapsedTime(t *testing.T) {
	for _, tc := range []struct {
		days int
		want string
	}{
		{21, "three weeks"},
		{24, "three weeks"},
		{60, "two months"},
		{7, "a week"},
	} {
		in := base()
		in.DaysSilent = tc.days
		if got := Draft(in).Body; !strings.Contains(got, tc.want) {
			t.Errorf("at %d days the body lacks %q:\n%s", tc.days, tc.want, got)
		}
	}
}

func TestDraftStatesTheStrengthWhenThereIsOne(t *testing.T) {
	in := base()
	in.Strength = "Scaled a Go service to 2M requests a day"

	if got := Draft(in).Body; !strings.Contains(got, "Scaled a Go service to 2M requests a day") {
		t.Errorf("body lacks the strength:\n%s", got)
	}
}

func TestDraftOmitsTheStrengthLineEntirelyWhenThereIsNone(t *testing.T) {
	with, without := base(), base()
	with.Strength = "Scaled a Go service to 2M requests a day"

	a, b := Draft(with).Body, Draft(without).Body

	if a == b {
		t.Fatal("the strength made no difference to the body")
	}
	// No placeholder, no dangling connective, no double blank line where the line was.
	for _, bad := range []string{"  ", "\n\n\n", "In particular,\n", "particular, ."} {
		if strings.Contains(b, bad) {
			t.Errorf("body without a strength contains %q — the line left a hole:\n%s", bad, b)
		}
	}
}

func TestDraftGreetsByNameWhenKnown(t *testing.T) {
	in := base()
	in.RecipientName = "Mara"

	if got := Draft(in).Body; !strings.HasPrefix(got, "Hi Mara,") {
		t.Errorf("body starts %q, want a greeting by name", firstLine(got))
	}
}

func TestDraftFallsBackToANeutralGreeting(t *testing.T) {
	got := Draft(base()).Body

	if strings.Contains(strings.ToLower(firstLine(got)), "hi ,") || strings.Contains(got, "Hi ,") {
		t.Errorf("greeting collapsed with no name: %q", firstLine(got))
	}
	if !strings.HasPrefix(got, "Hello,") {
		t.Errorf("body starts %q, want a neutral greeting", firstLine(got))
	}
}

// TestDraftToneRules pins the product, not the plumbing: a chase that apologises for existing or
// says "just checking in" is the one the candidate would have written themselves and did not send.
func TestDraftToneRules(t *testing.T) {
	in := base()
	in.Strength = "Led the migration of 12 services to Kubernetes"
	body := strings.ToLower(Draft(in).Body)

	for _, banned := range []string{
		"just checking in", "sorry to bother", "i apologise", "i apologize",
		"i know you're busy", "any update", "passionate", "hoping to hear",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("body contains the filler %q:\n%s", banned, body)
		}
	}
	if !strings.Contains(body, "?") {
		t.Error("body asks no question — a chase without an ask is a nudge nobody can answer")
	}
}

// TestDraftAsksTheQuestionThatFitsTheStage: after an interview, "is the role still open" reads as
// if the candidate does not know they are already in the process. The ask has to match where they
// actually are.
func TestDraftAsksTheQuestionThatFitsTheStage(t *testing.T) {
	applied := base()
	later := base()
	later.Stage = "interview"

	a, l := Draft(applied).Body, Draft(later).Body

	if !strings.Contains(a, "still open") {
		t.Errorf("applied-stage ask = %q, want it to ask whether the role is open", a)
	}
	if strings.Contains(l, "still open") {
		t.Errorf("interview-stage ask still asks whether the role is open:\n%s", l)
	}
	if !strings.Contains(strings.ToLower(l), "stand") && !strings.Contains(strings.ToLower(l), "decision") {
		t.Errorf("interview-stage ask = %q, want it to ask where the application stands", l)
	}
}

func TestDraftStaysShortEnoughToReadOnAPhone(t *testing.T) {
	in := base()
	in.Strength = "Led the migration of 12 services to Kubernetes, cutting deploy time by 40%"

	if words := len(strings.Fields(Draft(in).Body)); words > 120 {
		t.Errorf("body is %d words, want at most 120", words)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
