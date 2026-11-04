package mcpapp

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// A session is told only about the workflows it can run; OAuth-only actions
// must never be suggested to a client whose credential cannot call them.
func TestAccountInstructions_FollowTheServedTools(t *testing.T) {
	keyed := accountInstructions([]assistant.Tool{{Name: "search_jobs"}, {Name: "subscribe"}})
	for _, unavailable := range []string{"Auto-apply", "suggest_dictionary_term", "report_problem"} {
		if strings.Contains(keyed, unavailable) {
			t.Errorf("key instructions mention unavailable action %q: %q", unavailable, keyed)
		}
	}
	if !strings.Contains(keyed, "Alerts use configured channels") {
		t.Errorf("alert destination missing: %q", keyed)
	}

	granted := accountInstructions([]assistant.Tool{
		{Name: "auto_apply_start"}, {Name: "subscribe"},
		{Name: "suggest_dictionary_term"}, {Name: "report_problem"},
	})
	for _, required := range []string{
		"saved answers may be sent to employers", "Alerts use configured channels",
		"suggest_dictionary_term", "zero search hits alone is not a gap",
		"report_problem", "omit secrets and personal documents", "Do not report report failures",
	} {
		if !strings.Contains(granted, required) {
			t.Errorf("instructions omit %q: %q", required, granted)
		}
	}
	if n := utf8.RuneCountInString(granted); n > 512 {
		t.Errorf("account instructions are %d characters, want at most512", n)
	}
}
