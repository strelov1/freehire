package mcpapp

import (
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// accountPrompts are the templates a signed-in person picks in their client (Claude lists
// them under `/`) to find out what freehire does beyond a search. Unlike the catalogue
// prompts, each names the tools and their order: the sequence is the point, and a prompt is
// text the person chose to send, not a tool description the directory policy governs.
//
// A prompt is offered only when every tool it needs is served, so an API key is not handed
// the auto-apply workflow it cannot run.
func accountPrompts(tools []assistant.Tool) []promptSpec {
	served := func(name string) bool {
		return slices.ContainsFunc(tools, func(t assistant.Tool) bool { return t.Name == name })
	}
	fixed := func(name, title, description, text string) promptSpec {
		return promptSpec{
			prompt: &mcp.Prompt{Name: name, Title: title, Description: description},
			render: func(map[string]string) string { return text },
		}
	}
	var out []promptSpec
	if served("get_profile") {
		out = append(out, fixed("jobs_for_my_profile", "Find jobs for my profile",
			"Open jobs that fit your saved profile.",
			"Call get_profile, then search_jobs with the roles, skills and location preferences it holds. "+
				"Show the ten best matches with company, location, salary where stated and the link. "+
				"If I have no saved profile yet, say so and offer to set one up first."))
		setup := "Call get_profile and tell me what is missing: roles, skills, location and work mode, salary. " +
			"Ask me about the gaps, then save my answers"
		if served("update_profile") {
			setup += " with update_profile once I agree."
		} else {
			setup += " by sending me to /my/profile to fill them in."
		}
		out = append(out, fixed("set_up_my_profile", "Set up my profile",
			"Fill in what your job-search profile is missing.", setup))
	}
	if served("subscription_updates") {
		out = append(out, fixed("whats_new_in_my_alerts", "What's new in my alerts",
			"New postings your job alerts matched.",
			"Call list_subscriptions, then subscription_updates for the last 7 days. For each alert list "+
				"the new postings with company, location and link, and say which alerts found nothing. "+
				"If I have no alerts, offer to create one from my profile with subscribe."))
	}
	if served("auto_apply_start") {
		out = append(out, fixed("apply_to_saved_jobs", "Apply to my saved jobs",
			"Prepare applications for the jobs you saved, for your review.",
			"Call my_jobs with filter saved, then check_jobs_open on them, and list the ones still open. "+
				"Ask which I want to apply to. For each I pick, call auto_apply_start: it prepares the "+
				"application and sends nothing. Show me each prepared application with auto_apply_get, "+
				"and submit only the ones I approve, with auto_apply_review."))
	}
	return out
}
