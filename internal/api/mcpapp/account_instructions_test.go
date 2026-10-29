package mcpapp

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// A session is told only about the workflows it can run: an API key is served no
// auto-apply tool, so its instructions must not walk the person through one.
func TestAccountInstructions_FollowTheServedTools(t *testing.T) {
	keyed := accountInstructions([]assistant.Tool{{Name: "search_jobs"}, {Name: "subscribe"}})
	if strings.Contains(keyed, "auto_apply") {
		t.Errorf("instructions without auto-apply tools mention auto-apply: %q", keyed)
	}
	if !strings.Contains(keyed, "list_subscriptions") {
		t.Errorf("instructions with the alert tools omit the alert workflow: %q", keyed)
	}

	granted := accountInstructions([]assistant.Tool{{Name: "auto_apply_start"}, {Name: "subscribe"}})
	if !strings.Contains(granted, "auto_apply_retry") || !strings.Contains(granted, "subscription_updates") {
		t.Errorf("full instructions lost a workflow: %q", granted)
	}
}
