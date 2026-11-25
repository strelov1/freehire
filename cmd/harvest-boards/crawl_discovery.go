package main

import "context"

// crawlPatterns are the Common Crawl CDX patterns for providers whose prober has no discover of
// its own. Each matched URL is turned into a board id by atsboard's recogniser, so a hit names a
// board the same way the catalogue does. See commonCrawlCandidates for the pattern shapes.
var crawlPatterns = map[string][]string{
	// No jobs.eu.lever.co: leverProber only asks api.lever.co, so an EU board would probe as empty.
	"lever":           {"jobs.lever.co/*"},
	"smartrecruiters": {"jobs.smartrecruiters.com/*", "careers.smartrecruiters.com/*"},
	"workable":        {"apply.workable.com/*"},
	"recruitee":       {"*.recruitee.com"},
	"teamtailor":      {"*.teamtailor.com"},
	"bamboohr":        {"*.bamboohr.com"},
	"breezy":          {"*.breezy.hr"},
	"pinpoint":        {"*.pinpointhq.com"},
	"personio":        {"*.jobs.personio.de", "*.jobs.personio.com"},
	"jazzhr":          {"*.applytojob.com"},
	"jobvite":         {"jobs.jobvite.com/*"},
	"icims":           {"*.icims.com"},
	"freshteam":       {"*.freshteam.com"},
	"keka":            {"*.keka.com"},
	"zohorecruit":     {"*.zohorecruit.com"},
	"darwinbox":       {"*.darwinbox.in"},
	"gem":             {"jobs.gem.com/*"},
}

// crawlDiscovery adds the crawlPatterns sweep to a prober that cannot discover by itself. Only
// for a seedless run: the wrapper hides the prober's optional seed-mapping halves.
func crawlDiscovery(provider string, p prober) prober {
	if _, ok := p.(discoverer); ok {
		return p
	}
	if _, ok := crawlPatterns[provider]; !ok {
		return p
	}
	return crawlDiscoverer{prober: p, provider: provider}
}

type crawlDiscoverer struct {
	prober
	provider string
}

func (d crawlDiscoverer) discover(ctx context.Context, c httpClient) ([]string, error) {
	var out []string
	for _, pattern := range crawlPatterns[d.provider] {
		boards, err := commonCrawlCandidates(ctx, c, pattern, recognizedBoard(d.provider))
		if err != nil {
			return nil, err
		}
		out = append(out, boards...)
	}
	return out, nil
}
