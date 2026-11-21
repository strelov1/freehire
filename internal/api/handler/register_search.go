package handler

import (
	"github.com/strelov1/freehire/internal/api/atsapply"
	"github.com/strelov1/freehire/internal/platform/db"
)

// searchBackends is cfg.Search under each port it serves. Every field stays a nil interface
// when search is unconfigured: a nil *search.Client in an interface is non-nil and defeats
// the handlers' nil checks.
type searchBackends struct {
	jobs      searcher
	facets    facetCounter
	companies companySearcher
	// Both sitemaps page an index, so they are wired here.
	sitemapJobs, sitemapCompanies sitemapLister
	sitemapFreshJobs              freshSitemapLister
}

func newSearchBackends(cfg Config) searchBackends {
	if cfg.Search == nil {
		return searchBackends{}
	}
	return searchBackends{
		jobs:             cfg.Search,
		facets:           cfg.Search,
		companies:        cfg.Search,
		sitemapJobs:      cfg.Search,
		sitemapCompanies: companySitemapIndex{c: cfg.Search},
		sitemapFreshJobs: cfg.Search,
	}
}

// newAgentSurfaces wires the two agent-facing read surfaces over the same search backend
// and store, so an agent's question and a browser's reach the same catalogue: OJCP renders
// a foreign schema for agents that branch on error codes, and the ChatGPT app's MCP server
// renders ours for a model reading prose.
func newAgentSurfaces(cfg Config, queries *db.Queries, sb searchBackends) (*ojcpHandlers, *mcpappHandlers) {
	ojcpH := newOJCPHandlers(sb.jobs, queries, cfg.FrontendOrigin, atsapply.SubmittableProviders())
	ojcpH.signer = cfg.OJCPSigner
	ojcpH.agentViews = cfg.AgentViews
	mcpappH := newMCPAppHandlers(
		sb.jobs,
		sb.companies,
		queries,
		cfg.FrontendOrigin,
	)
	mcpappH.facets = sb.facets
	mcpappH.cache = cfg.Cache
	mcpappH.agentViews = cfg.AgentViews
	return ojcpH, mcpappH
}

// newSuggestHandlers wires the completion dictionary. Left nil when search is unconfigured: a
// nil *suggest.Service in the interface would pass the handler's "not configured" check and
// then dereference nil.
func newSuggestHandlers(cfg Config) *suggestHandlers {
	h := &suggestHandlers{}
	if cfg.Suggest != nil {
		h.suggest = cfg.Suggest
	}
	return h
}
