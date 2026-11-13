package mcpapp

import (
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func promptNames(t *testing.T, session *mcp.ClientSession) []string {
	t.Helper()
	listed, err := session.ListPrompts(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range listed.Prompts {
		if p.Title == "" || p.Description == "" {
			t.Errorf("%s has no title or description; a client has nothing to show", p.Name)
		}
		names = append(names, p.Name)
	}
	slices.Sort(names)
	return names
}

func TestAccountPrompts_AreListedAndRenderTheirToolOrder(t *testing.T) {
	granted := namedTools("get_profile", "update_profile", "search_jobs", "subscribe", "subscription_updates", "list_subscriptions",
		"my_jobs", "check_jobs_open", "auto_apply_start", "auto_apply_get", "auto_apply_review")
	want := []string{"apply_to_saved_jobs", "jobs_for_my_profile", "set_up_my_profile", "whats_new_in_my_alerts"}

	account := connectAccountCatalogue(t, granted, nil)
	got := promptNames(t, account)
	for _, name := range want {
		if !slices.Contains(got, name) {
			t.Fatalf("account prompts = %v, want %v among them", got, want)
		}
	}
	text, err := promptText(t, account, "apply_to_saved_jobs", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "auto_apply_start") || !strings.Contains(text, "sends nothing") ||
		strings.Index(text, "my_jobs") > strings.Index(text, "auto_apply_start") {
		t.Errorf("apply prompt does not order the tools or promise review: %q", text)
	}

	// The canonical endpoint offers them to a signed-in person next to the catalogue prompts.
	mixed := connectServer(t, newMixedServer(granted, 42, testOrigin, readerThatFinds(t), false))
	if got := promptNames(t, mixed); !slices.Contains(got, "jobs_for_my_profile") || !slices.Contains(got, promptJobsForStack) {
		t.Errorf("signed-in canonical prompts = %v", got)
	}
	// An anonymous connection is offered only the catalogue ones.
	anon := connectServer(t, newMixedServer(granted, 0, testOrigin, readerThatFinds(t), false))
	if got := promptNames(t, anon); slices.Contains(got, "jobs_for_my_profile") {
		t.Errorf("anonymous prompts offer a personal workflow: %v", got)
	}
}

// With the setup tools served, setting up starts from what is missing — a CV first — and
// saves only what the person confirms.
func TestAccountPrompts_SetUpWalksTheGuidedSetup(t *testing.T) {
	granted := connectAccountCatalogue(t,
		namedTools("get_profile", "update_profile", "get_setup_status", "add_cv"), nil)
	text, err := promptText(t, granted, "set_up_my_profile", nil)
	if err != nil {
		t.Fatal(err)
	}
	order := []string{"get_setup_status", "add_cv", "update_profile"}
	for i, name := range order {
		if !strings.Contains(text, name) {
			t.Fatalf("set-up prompt does not name %s: %q", name, text)
		}
		if i > 0 && strings.Index(text, order[i-1]) > strings.Index(text, name) {
			t.Errorf("set-up prompt names %s before %s: %q", name, order[i-1], text)
		}
	}
	if !strings.Contains(text, "confirm") {
		t.Errorf("set-up prompt does not ask for confirmation: %q", text)
	}
	// add_cv takes the person's own CV only; an agent never composes one from a chat.
	if !strings.Contains(text, "verbatim") || strings.Contains(text, "work history") {
		t.Errorf("set-up prompt invites a composed CV: %q", text)
	}
}

func TestAccountPrompts_FollowTheServedTools(t *testing.T) {
	keyed := connectAccountCatalogue(t, namedTools("get_profile", "search_jobs"), nil)
	got := promptNames(t, keyed)
	for name, offered := range map[string]bool{"jobs_for_my_profile": true, "set_up_my_profile": true, "apply_to_saved_jobs": false, "whats_new_in_my_alerts": false} {
		if slices.Contains(got, name) != offered {
			t.Fatalf("a key is offered %v, want %s offered = %v", got, name, offered)
		}
	}
	text, err := promptText(t, keyed, "set_up_my_profile", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "update_profile") || !strings.Contains(text, "/my/profile") {
		t.Errorf("a session without update_profile must be sent to the profile page: %q", text)
	}
}
