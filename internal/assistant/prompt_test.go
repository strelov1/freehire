package assistant

import (
	"strings"
	"testing"
)

func TestEachPresetHasItsOwnPrompt(t *testing.T) {
	chat := SystemPrompt(PresetChat)
	tailor := SystemPrompt(PresetTailor)
	browse := SystemPrompt(PresetBrowse)

	if chat == "" || tailor == "" || browse == "" {
		t.Fatal("every preset needs a system prompt; an unprompted agent has no job to do")
	}
	if chat == tailor || chat == browse || tailor == browse {
		t.Error("two presets share a prompt; the preset is what makes them different")
	}
}

// The panel's agent is the only one with eyes. A prompt that does not say so
// produces an agent that asks the candidate to paste the vacancy it could have
// read itself.
func TestBrowsePromptTellsTheAgentToReadThePage(t *testing.T) {
	p := SystemPrompt(PresetBrowse)

	if !strings.Contains(p, "read_current_page") {
		t.Error("the browse prompt never names read_current_page, so the agent will not know it can see the page")
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
}

func TestChatPromptShowsVacanciesOnlyThroughTheTool(t *testing.T) {
	p := SystemPrompt(PresetChat)

	if !strings.Contains(p, "present_jobs") {
		t.Error("the chat prompt does not tell the agent to show vacancies through present_jobs")
	}
	// A link in prose no longer renders as a card, so an instruction to write one
	// would produce exactly the bare link this change exists to remove.
	if strings.Contains(p, "/jobs/") {
		t.Errorf("the chat prompt still asks for a job URL in prose; a vacancy reaches the user only through present_jobs\n%s", p)
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

// The candidate opened the panel ON something. The chat playbook this preset
// extends opens with `get_profile` and a "what are you looking for?" — correct on
// the website, wrong in a side panel, where the answer is on the screen behind it.
// The extension must say so explicitly, because it is appended AFTER that
// instruction and the model follows what it read.
func TestBrowsePromptOpensOnThePageNotTheProfile(t *testing.T) {
	p := SystemPrompt(PresetBrowse)

	if !strings.Contains(p, "FIRST thing you do") {
		t.Error("the browse prompt does not override how the conversation opens, so the agent will run the chat playbook's opening and ask what the candidate is looking for")
	}
	// It has to name the instruction it is overriding, or the two read as advice
	// about different moments rather than one replacing the other.
	browseOnly := strings.TrimPrefix(p, chatPrompt)
	if !strings.Contains(browseOnly, "get_profile") {
		t.Error("the browse prompt never mentions get_profile, so nothing tells the agent that the opening it just read does not apply here")
	}
}

// An unattended run is the same method at a different rhythm. What the prompt has to state
// is exactly what a run would otherwise get wrong: keep going, do not ask mid-pass, and
// account for every requirement at the end — including the ones nothing could close.
func TestTailorPromptDescribesTheUnattendedRun(t *testing.T) {
	p := SystemPrompt(PresetTailor)

	for _, want := range []string{"tailor_report", "closed_bank", "closed_candidate", "open", "not_reached"} {
		if !strings.Contains(p, want) {
			t.Errorf("the tailoring prompt never mentions %q; a run cannot report an outcome it was never told exists", want)
		}
	}
	if !strings.Contains(p, "evidence_id") {
		t.Error("the run section must restate that a bullet needs evidence — the wall holds when nobody is watching")
	}
	// One question at the end, not a list: the rest of the remainder is visible beside the CV.
	if !strings.Contains(strings.ToLower(p), "one question") && !strings.Contains(p, "FIRST open one") {
		t.Error("the prompt must ask for ONE closing question; a numbered list of gaps gets one answer")
	}
}

// Two recorded sessions opened with a long restatement of the fit analysis the candidate had
// open beside the chat, then spent every remaining round searching the bank one requirement at
// a time — and edited nothing. The prompt now says where the evidence already is, and to spend
// rounds on edits rather than on a summary.
func TestTailorPromptSpendsRoundsOnEditsNotSummaries(t *testing.T) {
	p := SystemPrompt(PresetTailor)

	if !strings.Contains(p, "cv_context") || !strings.Contains(p, "evidence") {
		t.Error("the prompt must point the agent at the evidence cv_context already carries")
	}
	lower := strings.ToLower(p)
	if !strings.Contains(lower, "do not restate") && !strings.Contains(lower, "not repeat") {
		t.Error("the prompt must forbid restating the fit analysis — the candidate has it open beside the chat")
	}
	if !strings.Contains(lower, "as you go") && !strings.Contains(lower, "one requirement at a time") {
		t.Error("the prompt must ask for edits as each requirement is closed, not after all the research")
	}
}
