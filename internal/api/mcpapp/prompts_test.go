package mcpapp

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

func promptText(t *testing.T, session *mcp.ClientSession, name string, args map[string]string) (string, error) {
	t.Helper()
	res, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: name, Arguments: args})
	if err != nil {
		return "", err
	}
	if len(res.Messages) != 1 || res.Messages[0].Role != "user" {
		t.Fatalf("%s: messages = %+v, want one user message", name, res.Messages)
	}
	text, ok := res.Messages[0].Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("%s: content is %T, want text", name, res.Messages[0].Content)
	}
	return text.Text, nil
}

func TestEveryPromptIsListedWithItsArguments(t *testing.T) {
	listed, err := connect(t, readerThatFinds(t)).ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatalf("prompts/list: %v", err)
	}
	want := map[string]bool{promptJobsForStack: true, promptCheckShortlist: true, promptSalaryCheck: true}
	if len(listed.Prompts) != len(want) {
		t.Fatalf("listed %d prompts, want %d", len(listed.Prompts), len(want))
	}
	for _, p := range listed.Prompts {
		if !want[p.Name] {
			t.Errorf("unexpected prompt %q", p.Name)
		}
		if p.Title == "" || p.Description == "" || len(p.Arguments) == 0 {
			t.Errorf("%s has no title, description or arguments; a client has nothing to show", p.Name)
		}
	}
}

func TestAPromptFillsInWhatItWasGivenAndLeavesOutWhatItWasNot(t *testing.T) {
	session := connect(t, readerThatFinds(t))

	full, err := promptText(t, session, promptJobsForStack, map[string]string{
		"stack": "Go and Kubernetes", "location": "remote from LATAM", "seniority": "senior",
	})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	for _, part := range []string{"senior", "Go and Kubernetes", "remote from LATAM"} {
		if !strings.Contains(full, part) {
			t.Errorf("prompt %q does not carry %q", full, part)
		}
	}

	bare, err := promptText(t, session, promptJobsForStack, map[string]string{"stack": "Rust"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if strings.Contains(bare, "  ") || strings.Contains(bare, " in .") || strings.Contains(bare, "{") {
		t.Errorf("an omitted optional argument left a trace: %q", bare)
	}
}

func TestAPromptWithoutItsRequiredArgumentIsRefused(t *testing.T) {
	session := connect(t, readerThatFinds(t))
	for _, name := range []string{promptJobsForStack, promptCheckShortlist, promptSalaryCheck} {
		if _, err := promptText(t, session, name, map[string]string{}); err == nil {
			t.Errorf("%s rendered with its required argument missing", name)
		}
	}
}

func TestTheInstructionsIntroduceEveryTool(t *testing.T) {
	// The instructions are where cross-tool guidance belongs — tool descriptions may not
	// carry it. A tool they never mention is one the model has to discover by accident.
	// search and fetch are left out on purpose: they exist for the deep-research connectors,
	// which call them by name without being told.
	for _, name := range []string{toolSearchJobs, toolGetJob, toolSearchCompanies, toolGetCompany,
		toolCheckJobsOpen, toolSimilarJobs, toolSalaryStats, toolMarketStats, toolFilterValues} {
		if !strings.Contains(serverInstructions, name) {
			t.Errorf("the instructions never mention %s", name)
		}
	}
}

func complete(t *testing.T, session *mcp.ClientSession, prompt, arg, value string) []string {
	t.Helper()
	res, err := session.Complete(context.Background(), &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: "ref/prompt", Name: prompt},
		Argument: mcp.CompleteParamsArgument{Name: arg, Value: value},
	})
	if err != nil {
		t.Fatalf("completion/complete %s.%s=%q: %v", prompt, arg, value, err)
	}
	return res.Completion.Values
}

// A prompt argument with a closed vocabulary offers its values as the person types, so
// they pick a word the tools understand instead of one dropped into ignored_params.
func TestAPromptArgumentWithAVocabularySuggestsItsValues(t *testing.T) {
	session := connect(t, readerThatFinds(t))

	if got := complete(t, session, promptSalaryCheck, "seniority", "S"); !slices.Equal(got, []string{"senior", "staff"}) {
		t.Errorf("seniority from %q = %v, want [senior staff] — prefix match, case folded", "S", got)
	}
	if got := complete(t, session, promptJobsForStack, "seniority", ""); !slices.Equal(got, vocab.SeniorityValues) {
		t.Errorf("seniority from nothing = %v, want the whole vocabulary in its own order", got)
	}
	if got := complete(t, session, promptSalaryCheck, "role", "back"); !slices.Contains(got, "backend") {
		t.Errorf("role from %q = %v, want it to offer backend", "back", got)
	}
}

// An argument with no vocabulary — free text, or a list of slugs — offers nothing rather
// than a guess, and an unknown prompt is an empty answer rather than a failure the client
// shows the person.
func TestAFreeTextArgumentSuggestsNothing(t *testing.T) {
	session := connect(t, readerThatFinds(t))
	for _, c := range []struct{ prompt, arg string }{
		{promptJobsForStack, "stack"},
		{promptCheckShortlist, "postings"},
		{"no_such_prompt", "seniority"},
	} {
		if got := complete(t, session, c.prompt, c.arg, "s"); len(got) != 0 {
			t.Errorf("%s.%s = %v, want no suggestions", c.prompt, c.arg, got)
		}
	}
}
