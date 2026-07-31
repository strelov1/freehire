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
	for _, known := range []string{PresetChat, PresetTailor, PresetProfile, PresetBrowse} {
		if got := NormalizePreset(known); got != known {
			t.Errorf("NormalizePreset(%q) = %q, want it unchanged", known, got)
		}
	}
	for _, unknown := range []string{"", "browze", "CHAT", "interview"} {
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
	for _, preset := range []string{PresetBrowse, PresetTailor, PresetProfile} {
		if strings.Contains(SystemPrompt(preset), "inbox_overview") {
			t.Errorf("the %q prompt talks about mail, but that preset registers no mail tool", preset)
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
