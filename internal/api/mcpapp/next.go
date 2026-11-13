package mcpapp

import (
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

// nextHints are the follow-up sentences a search_jobs and a get_job result carry in their
// `next` field. Agents pick tools from the person's request and follow hints in results, so
// this is where a person who only searches hears that freehire does more.
//
// One short sentence each, and the same text for every call of a session: the result is
// replayed into the model's context every turn, so it is stated once per response, never
// per job, and never grows with the result.
type nextHints struct{ search, job string }

// signedInNext names only the follow-ups among the tools this session is actually served:
// an API key has no auto-apply, so its hint does not offer it.
func signedInNext(tools []assistant.Tool) nextHints {
	served := func(name string) bool {
		return slices.ContainsFunc(tools, func(t assistant.Tool) bool { return t.Name == name })
	}
	pick := func(items ...[2]string) string {
		var out []string
		for _, item := range items {
			if served(item[0]) {
				out = append(out, item[1])
			}
		}
		if len(out) == 0 {
			return ""
		}
		return "Next: " + strings.Join(out, "; ") + "."
	}
	return nextHints{
		search: pick(
			[2]string{"get_profile", "get_profile first if not read yet"},
			[2]string{"save_job", "save_job to keep a posting"},
			[2]string{"track_job", "track_job to follow its stage"},
			[2]string{"subscribe", "subscribe for alerts on this search"},
		),
		job: pick(
			[2]string{"save_job", "save_job to keep it"},
			[2]string{"track_job", "track_job to follow its stage"},
			[2]string{"auto_apply_start", "auto_apply_start to prepare an application for review"},
			[2]string{"subscribe", "subscribe for alerts on similar searches"},
		),
	}
}
