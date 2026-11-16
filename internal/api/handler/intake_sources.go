package handler

import (
	"context"
	"log"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// intakeSources keeps arbitrary ATS links on the SSRF-guarded client. LinkedIn's
// adapter only reads fixed public hosts, so it may use its configured fetch tier.
func intakeSources(registry map[string]sources.Source) map[string]sources.Source {
	linkedin := map[string]sources.Source{"linkedin": registry["linkedin"]}
	if err := sources.ApplyScraplingEgress(linkedin); err != nil {
		// Keep other intake sources available, but never silently bypass a broken
		// LinkedIn fetch configuration. Its links remain eligible for triage.
		log.Printf("intake: LinkedIn fetch configuration unavailable: %v", err)
		registry["linkedin"] = sources.NewLinkedIn(unavailableLinkedInFetch{err: err})
		return registry
	}
	registry["linkedin"] = linkedin["linkedin"]
	return registry
}

type unavailableLinkedInFetch struct{ err error }

func (f unavailableLinkedInFetch) GetHTMLResolved(context.Context, string) (*html.Node, string, error) {
	return nil, "", f.err
}
