package assistant

import (
	"testing"

	"github.com/tmc/langchaingo/llms"
)

func TestUserConfirmedRequiresTheCurrentUnambiguousAnswer(t *testing.T) {
	claim := "Merge these two achievements: taught students; trained students."
	ask, _ := EncodeAssistant("", []llms.ToolCall{{ID: "ask", FunctionCall: &llms.FunctionCall{
		Name: "request_confirmation", Arguments: `{"claim":"` + claim + `","question":"Merge these?"}`,
	}}})
	for _, tc := range []struct {
		answer string
		want   bool
	}{
		{claim, true}, {"yes", true}, {"Да", true},
		{"where does my CV fit most", false}, {"no", false}, {"not " + claim, false},
	} {
		t.Run(tc.answer, func(t *testing.T) {
			answer, _ := EncodeUser(tc.answer)
			if got := UserConfirmed([]Message{ask, answer}, claim); got != tc.want {
				t.Errorf("UserConfirmed = %v, want %v", got, tc.want)
			}
		})
	}
	answer, _ := EncodeUser("yes")
	other, _ := EncodeAssistant("", []llms.ToolCall{{FunctionCall: &llms.FunctionCall{
		Name: "request_confirmation", Arguments: `{"claim":"Another claim entirely"}`,
	}}})
	if UserConfirmed([]Message{ask, other, answer}, claim) {
		t.Fatal("one yes must not approve multiple pending claims")
	}
	later, _ := EncodeAssistant("Before merging, do you want to add missing metrics?", nil)
	if UserConfirmed([]Message{ask, later, answer}, claim) {
		t.Fatal("a yes to a later question must not approve the merge")
	}
	approved, _ := EncodeUser(claim)
	changedTopic, _ := EncodeUser("find jobs instead")
	if UserConfirmed([]Message{ask, approved, changedTopic}, claim) {
		t.Fatal("an old approval must not authorize a later turn")
	}
}

func TestUserSaidAcceptsYesOnlyForOneMatchingConfirmation(t *testing.T) {
	claim := "I am available for shifts and weekends."
	ask, _ := EncodeAssistant("", []llms.ToolCall{{FunctionCall: &llms.FunctionCall{
		Name: "request_confirmation", Arguments: `{"claim":"` + claim + `"}`,
	}}})
	yes, _ := EncodeUser("yes")
	if !UserSaid([]Message{ask, yes}, claim) {
		t.Fatal("a clear answer to one confirmation should establish the candidate's claim")
	}
	next, _ := EncodeUser("now tailor my CV")
	if !UserSaid([]Message{ask, yes, next}, claim) {
		t.Fatal("a confirmed claim must stay said after a later turn")
	}
	if UserSaid([]Message{yes}, claim) {
		t.Fatal("yes alone must not establish an arbitrary claim")
	}
}
