package assistant

import (
	"strings"
	"testing"
)

func TestEachPresetHasItsOwnPrompt(t *testing.T) {
	chat := SystemPrompt(PresetChat)
	tailor := SystemPrompt(PresetTailor)

	if chat == "" || tailor == "" {
		t.Fatal("both presets need a system prompt; an unprompted agent has no job to do")
	}
	if chat == tailor {
		t.Error("the two presets share a prompt; the preset is what makes them different")
	}
}

// The candidate has already told the product their roles, skills and geography on
// the profile page. A prompt that does not say to read it produces an agent that
// opens every conversation with a questionnaire.
func TestChatPromptReadsTheProfileBeforeInterrogating(t *testing.T) {
	p := SystemPrompt(PresetChat)

	if !strings.Contains(p, "get_profile") {
		t.Error("the chat prompt never mentions get_profile, so the agent will ask for what the profile already answers")
	}
}

func TestChatPromptCarriesTheSearchPlaybook(t *testing.T) {
	p := SystemPrompt(PresetChat)

	// Read the vocabulary before filtering — otherwise the model invents facet
	// values and gets a confidently unfiltered result set.
	if !strings.Contains(p, "facets") {
		t.Error("the chat prompt does not tell the agent to read the facet vocabulary first")
	}
	// Vacancies must be presented as canonical links, which the chat unfurls into
	// job cards.
	if !strings.Contains(p, "/jobs/") {
		t.Error("the chat prompt does not tell the agent to link vacancies by public_slug")
	}
}

func TestTailorPromptCarriesTheHonestyRule(t *testing.T) {
	p := SystemPrompt(PresetTailor)

	for _, want := range []string{"missing_have", "missing_gap"} {
		if !strings.Contains(p, want) {
			t.Errorf("the tailoring prompt does not mention %q; the reframe-vs-ask split is the whole safeguard", want)
		}
	}
	if !strings.Contains(strings.ToLower(p), "ask") {
		t.Error("the tailoring prompt must require asking the candidate before writing an unevidenced claim")
	}
}

func TestAnUnknownPresetFallsBackToTheChatPrompt(t *testing.T) {
	if SystemPrompt("wizard") != SystemPrompt(PresetChat) {
		t.Error("an unknown preset must still get a prompt; a session with none would answer unguided")
	}
}
