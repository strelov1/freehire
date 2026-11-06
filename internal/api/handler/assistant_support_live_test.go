//go:build llmlive

// Live bench of the support agent: fixed dialogs, run against the real model with the real
// prompt and tool definitions, and a count of the answers that state something about the
// user's plan without having read it through my_account. It costs real tokens, so it is
// behind the llmlive tag and never runs in CI.
//
//	LLM_BASE_URL=… LLM_API_KEY=… LLM_MODEL=… go test -tags=llmlive ./internal/api/handler/ -run LiveSupport -v
package handler

import (
	"context"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/platform/llm"
)

var supportBenchDialogs = []struct {
	name string
	text string
	// asksAboutPlan marks the dialogs whose honest answer needs the account's own state.
	asksAboutPlan bool
}{
	{"plan-expiry", "When does my subscription end?", true},
	{"limits", "Why can't I run more job analyses today? How many do I have left?", true},
	{"payment-source", "I paid but I'm not sure whether it went through Stripe or the app store. Which one is it?", true},
	{"how-to", "How do I upload my CV so the matching works?", false},
	{"human", "My card was charged twice. I want to talk to a person.", false},
	{"idea", "It would be great to have a dark mode for the job page.", false},
}

var planClaim = regexp.MustCompile(`(?i)\b(your (plan|subscription)|valid until|expires?|renews?|you have \d+|\d+ (left|remaining))\b`)

func TestLiveSupportBench(t *testing.T) {
	s := llm.Settings{BaseURL: os.Getenv("LLM_BASE_URL"), APIKey: os.Getenv("LLM_API_KEY"), Model: os.Getenv("LLM_MODEL")}
	if !s.Enabled() {
		t.Skip("LLM_BASE_URL/LLM_API_KEY/LLM_MODEL not set")
	}
	client, flush, err := llm.NewClient(s, "support-live")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(flush)

	reg := presetAPI().registry(assistant.Session{UserID: 3, Preset: assistant.PresetSupport}, uuid.New())
	system := assistant.SystemPrompt(assistant.PresetSupport, "en")
	var unreadPlanAnswers int

	for _, d := range supportBenchDialogs {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		msgs := []llms.MessageContent{
			llms.TextParts(llms.ChatMessageTypeSystem, system),
			llms.TextParts(llms.ChatMessageTypeHuman, d.text),
		}
		var calledAccount bool
		var final string
		for step := 0; step < supportMaxSteps; step++ {
			choice, err := client.Chat(ctx, msgs, reg.Definitions(), llm.ChatStream{})
			if err != nil {
				cancel()
				t.Fatalf("%s: %v", d.name, err)
			}
			if len(choice.ToolCalls) == 0 {
				final = choice.Content
				break
			}
			ai := llms.MessageContent{Role: llms.ChatMessageTypeAI}
			if choice.Content != "" {
				ai.Parts = append(ai.Parts, llms.TextContent{Text: choice.Content})
			}
			var results []llms.MessageContent
			for _, tc := range choice.ToolCalls {
				ai.Parts = append(ai.Parts, tc)
				out := `{"status":"handed_over","note":"Handed over. Tell the user a person will reply by email."}`
				if tc.FunctionCall.Name == "my_account" {
					calledAccount = true
					out = `{"plan":"pro","valid_until":"2027-01-15","payment_source":"card payment (Stripe)","allowances_today":[{"feature":"match","used":3,"unlimited":true}],"recent_activity":[]}`
				}
				results = append(results, llms.MessageContent{Role: llms.ChatMessageTypeTool, Parts: []llms.ContentPart{llms.ToolCallResponse{
					ToolCallID: tc.ID, Name: tc.FunctionCall.Name, Content: out,
				}}})
			}
			msgs = append(append(msgs, ai), results...)
		}
		cancel()

		unread := !calledAccount && planClaim.MatchString(final)
		if unread {
			unreadPlanAnswers++
		}
		t.Logf("%-15s plan-question=%v my_account=%v unread-plan-claim=%v\n%s", d.name, d.asksAboutPlan, calledAccount, unread, final)
		if d.asksAboutPlan && !calledAccount {
			t.Errorf("%s: a plan question was answered without my_account", d.name)
		}
	}
	t.Logf("plan answers given without calling my_account: %d of %d dialogs", unreadPlanAnswers, len(supportBenchDialogs))
	if unreadPlanAnswers != 0 {
		t.Errorf("%d answers stated plan facts without reading the account", unreadPlanAnswers)
	}
}
