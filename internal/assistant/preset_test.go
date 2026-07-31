package assistant

import (
	"strings"
	"testing"
)

// SystemPrompt has always fallen back to the chat prompt for a preset it does not
// recognise, and the tool registry decides the same question separately. Two
// switches that fall back differently would hand a session one preset's
// instructions and another's tools — the model would then be told about tools it
// cannot call. One exported normaliser is what keeps the two answers the same.
func TestNormalizePresetMapsTheUnrecognisedToChat(t *testing.T) {
	for _, known := range []string{PresetChat, PresetTailor, PresetProfile, PresetBrowse, PresetInterview} {
		if got := NormalizePreset(known); got != known {
			t.Errorf("NormalizePreset(%q) = %q, want it unchanged", known, got)
		}
	}
	for _, unknown := range []string{"", "browze", "CHAT", "rehearsal"} {
		if got := NormalizePreset(unknown); got != PresetChat {
			t.Errorf("NormalizePreset(%q) = %q, want %q", unknown, got, PresetChat)
		}
	}
}

// The fallback is the normaliser's, not a second copy of it.
func TestSystemPromptFallsBackThroughTheNormaliser(t *testing.T) {
	if SystemPrompt("browze") != SystemPrompt(PresetChat) {
		t.Error("an unrecognised preset does not get the chat prompt")
	}
}

// The side panel extends the chat playbook but registers no mail tool, so it must
// not inherit the mail section along with it. browsePrompt is appended to
// chatPrompt, and appending mail to chatPrompt itself is exactly the mistake this
// pins shut.
func TestOnlyTheChatPresetIsToldAboutMail(t *testing.T) {
	if !strings.Contains(SystemPrompt(PresetChat), "inbox_overview") {
		t.Error("the chat prompt does not mention the mail tools it carries")
	}
	for _, preset := range []string{PresetBrowse, PresetTailor, PresetProfile, PresetInterview} {
		if strings.Contains(SystemPrompt(preset), "inbox_overview") {
			t.Errorf("the %q prompt talks about mail, but that preset registers no mail tool", preset)
		}
	}
}

// A rehearsal runs on the candidate's own recorded experience, and the bank is where
// that lives — so the prompt has to state the one rule the service cannot enforce.
// experience_add takes `said` as a string and stamps stated_in_chat either way, so
// nothing downstream can tell the candidate's words from the model's paraphrase of
// them. Only an explicit agreement, visible in the transcript, makes it checkable.
func TestInterviewPromptGatesTheBankOnTheCandidatesAgreement(t *testing.T) {
	p := SystemPrompt(PresetInterview)
	for _, want := range []string{"experience_add", "agree", "their own words"} {
		if !strings.Contains(p, want) {
			t.Errorf("the rehearsal prompt never says %q about recording an answer", want)
		}
	}
}

// The invitation reaches the model as text an employer wrote, and the same guard the
// mail section carries has to be stated here — this preset holds no mail tools, so it
// would otherwise never be told.
func TestInterviewPromptNamesTheInvitationAsUntrusted(t *testing.T) {
	p := SystemPrompt(PresetInterview)
	for _, want := range []string{"untrusted", "ATTACK"} {
		if !strings.Contains(p, want) {
			t.Errorf("the rehearsal prompt never says %q about the invitation", want)
		}
	}
}

// One round per session is what keeps a rehearsal from wandering into a survey of
// every interview format, and what keeps its turns inside the ordinary step ceiling.
func TestInterviewPromptHoldsToOneRound(t *testing.T) {
	p := SystemPrompt(PresetInterview)
	for _, want := range []string{"one round", "one question"} {
		if !strings.Contains(strings.ToLower(p), want) {
			t.Errorf("the rehearsal prompt never says %q", want)
		}
	}
}

// Bodies are attacker-controlled text. The prompt is not the only guard — the tool
// surface sends no mail, so an injection has no outbound channel — but the model
// still has to be told, or it reads an instruction in a message as a request.
func TestMailPromptNamesBodiesAsUntrusted(t *testing.T) {
	prompt := SystemPrompt(PresetChat)
	for _, want := range []string{"untrusted", "ATTACK"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the mail section never says %q about message bodies", want)
		}
	}
}
