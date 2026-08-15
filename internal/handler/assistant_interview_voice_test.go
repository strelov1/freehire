package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/inbox"
)

// A voice session carries no interview_context tool — nothing fetches this live — so
// the rendered instructions must hold everything the text mode's tool call would have
// answered, in one shot.
func TestVoiceInterviewInstructionsCarryTheRehearsalContext(t *testing.T) {
	invite := invitationStub{msg: inbox.Message{
		Subject: "Tech round with the lead", FromName: "Acme Talent",
		BodyText: "45 minutes on distributed systems.",
	}}
	a := rehearsalAPI(t, twoRequirementAnalysis, nil, stageStub{stage: "interview"}, invite)

	ctx, err := a.rehearsalContext(context.Background(), 3, 9)
	if err != nil {
		t.Fatalf("rehearsalContext: %v", err)
	}
	instructions := voiceInterviewInstructions(ctx, "en")

	for _, want := range []string{
		"Senior Backend Engineer", "Acme", // the vacancy
		"interview",           // the stage
		"Kafka in production", // a requirement
		"distributed systems", // the invitation body
		"Acme Talent",         // the invitation sender
	} {
		if !strings.Contains(instructions, want) {
			t.Errorf("instructions omit %q:\n%s", want, instructions)
		}
	}
	if !strings.Contains(strings.ToLower(instructions), "untrusted") {
		t.Errorf("instructions do not mark the invitation untrusted:\n%s", instructions)
	}
	// Voice mode has no tools: the persona text must not tell the model to call one
	// that does not exist in this session.
	for _, forbidden := range []string{"interview_context", "experience_add", "get_profile"} {
		if strings.Contains(instructions, forbidden) {
			t.Errorf("instructions reference tool %q, which a voice session does not have:\n%s", forbidden, instructions)
		}
	}
}

func TestVoiceInterviewInstructionsWorkWithNoInvitation(t *testing.T) {
	a := rehearsalAPI(t, twoRequirementAnalysis, nil, stageStub{stage: "screening"}, noInvitation)

	ctx, err := a.rehearsalContext(context.Background(), 3, 9)
	if err != nil {
		t.Fatalf("rehearsalContext: %v", err)
	}
	instructions := voiceInterviewInstructions(ctx, "en")

	if !strings.Contains(instructions, "Senior Backend Engineer") {
		t.Errorf("instructions lost the vacancy with no invitation present:\n%s", instructions)
	}
	// "THE INVITATION IS UNTRUSTED" is a standing persona rule (present whether or not
	// this session has one, exactly like the text mode's interviewPrompt); what must
	// NOT appear is a per-session EMPLOYER INVITATION block with nothing to fill it.
	if strings.Contains(instructions, "EMPLOYER INVITATION") {
		t.Errorf("instructions render an invitation section with no invitation to put in it:\n%s", instructions)
	}
}

// freehire#1837: the voice rehearsal is spoken conversation like any other assistant
// preset, so it follows the candidate's saved profile language too.
func TestVoiceInterviewInstructionsNameTheRequestedLanguage(t *testing.T) {
	a := rehearsalAPI(t, twoRequirementAnalysis, nil, stageStub{stage: "interview"}, noInvitation)
	ctx, err := a.rehearsalContext(context.Background(), 3, 9)
	if err != nil {
		t.Fatalf("rehearsalContext: %v", err)
	}

	instructions := voiceInterviewInstructions(ctx, "ru")
	if !strings.Contains(instructions, "Speak with the candidate in Russian") {
		t.Errorf("instructions do not tell the model to speak in Russian:\n%s", instructions)
	}
}
