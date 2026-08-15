package handler

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/assistant"
)

// assistantMailTools is the group this change adds. Naming it once keeps the two
// directions of the scoping test honest about the same list.
var assistantMailTools = []string{
	"inbox_overview", "inbox_search", "inbox_triage",
	"inbox_resolve_suggestion", "inbox_link", "inbox_unlink", "inbox_record_application",
}

func TestChatPresetOffersTheMailTools(t *testing.T) {
	reg := presetAPI().registry(assistant.Session{UserID: 3, Preset: assistant.PresetChat}, uuid.New())

	for _, want := range assistantMailTools {
		if !slices.Contains(reg.Names(), want) {
			t.Errorf("chat preset is missing the %q tool; registered: %v", want, reg.Names())
		}
	}
}

// A tailoring session is working one CV against one vacancy, an experience
// interview is collecting what the candidate has done, and a side panel is talking
// about the page on screen. None of them has a reason to read the mailbox, and
// every registered tool spends the model's context on every turn whether or not it
// is called.
func TestOnlyTheChatPresetOffersTheMailTools(t *testing.T) {
	cvID, jobID := uuid.MustParse("66666666-6666-4666-8666-666666666666"), int64(9)

	for _, sess := range []assistant.Session{
		{UserID: 3, Preset: assistant.PresetTailor, CVID: &cvID, JobID: &jobID},
		{UserID: 3, Preset: assistant.PresetProfile},
		{UserID: 3, Preset: assistant.PresetBrowse},
		{UserID: 3, Preset: assistant.PresetInterview, JobID: &jobID},
	} {
		reg := presetAPI().registry(sess, uuid.New())
		for _, name := range reg.Names() {
			if strings.HasPrefix(name, "inbox_") {
				t.Errorf("preset %q offers %q; mail belongs to the general chat session only", sess.Preset, name)
			}
		}
	}
}

// A session recording a preset we do not know must get ONE preset's prompt and
// that same preset's tools. Before assistant.NormalizePreset the prompt fell back
// to chat while the registry compared against the chat constant, so the two could
// answer differently — the model would then be instructed at length about tools it
// had not been given.
func TestAnUnknownPresetGetsTheChatToolSet(t *testing.T) {
	reg := presetAPI().registry(assistant.Session{UserID: 3, Preset: "browze"}, uuid.New())

	for _, want := range append([]string{"search_jobs"}, assistantMailTools...) {
		if !slices.Contains(reg.Names(), want) {
			t.Errorf("an unrecognised preset is missing %q, but runs under the chat prompt that names it", want)
		}
	}
}

// toolNameInPrompt matches a backticked identifier shaped like one of our tool
// names: lowercase words joined by underscores. Prompts backtick other things
// (argument names, filter values), so a match is only a candidate — the assertion
// below is one-directional on purpose.
var toolNameInPrompt = regexp.MustCompile("`([a-z]+(?:_[a-z]+)+)`")

// Every tool a preset's prompt names must be registered for that preset.
//
// Nothing guarded this before. The prompts compose — the side panel's is the chat
// playbook plus an override — so a section appended to the shared prompt reaches
// presets that never registered its tools, and the model spends rounds on calls
// that can only come back "unknown tool". The reverse is deliberately NOT asserted:
// a tool may go unnamed by the prompt, because its own description is what the
// model reads to decide whether to call it.
func TestPromptOnlyNamesToolsThePresetHas(t *testing.T) {
	cvID, jobID := uuid.MustParse("66666666-6666-4666-8666-666666666666"), int64(9)

	for _, sess := range []assistant.Session{
		{UserID: 3, Preset: assistant.PresetChat},
		{UserID: 3, Preset: assistant.PresetProfile},
		{UserID: 3, Preset: assistant.PresetBrowse},
		{UserID: 3, Preset: assistant.PresetTailor, CVID: &cvID, JobID: &jobID},
		{UserID: 3, Preset: assistant.PresetInterview, JobID: &jobID},
	} {
		registered := presetAPI().registry(sess, uuid.New()).Names()
		// Only names that are tools SOMEWHERE can be judged; a backticked word that
		// is no preset's tool is an argument or a filter value, not a broken promise.
		everywhere := allRegisteredToolNames(t)

		for _, m := range toolNameInPrompt.FindAllStringSubmatch(assistant.SystemPrompt(sess.Preset, "en"), -1) {
			name := m[1]
			if slices.Contains(everywhere, name) && !slices.Contains(registered, name) {
				t.Errorf("the %q prompt tells the model to call %q, which that preset does not register",
					sess.Preset, name)
			}
		}
	}
}

// allRegisteredToolNames is every tool name any preset offers.
func allRegisteredToolNames(t *testing.T) []string {
	t.Helper()
	cvID, jobID := uuid.MustParse("66666666-6666-4666-8666-666666666666"), int64(9)
	var all []string
	for _, sess := range []assistant.Session{
		{UserID: 3, Preset: assistant.PresetChat},
		{UserID: 3, Preset: assistant.PresetBrowse},
		{UserID: 3, Preset: assistant.PresetTailor, CVID: &cvID, JobID: &jobID},
		// Every preset that registers a tool of its own belongs here, or the guard above
		// cannot see that tool at all: a name no session offers is not in `everywhere`,
		// so a prompt naming it passes unchallenged.
		{UserID: 3, Preset: assistant.PresetInterview, JobID: &jobID},
	} {
		all = append(all, presetAPI().registry(sess, uuid.New()).Names()...)
	}
	return all
}
