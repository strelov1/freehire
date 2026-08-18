import type { RequestHandler } from './$types';

// A real robots file (not the SPA shell): allow crawling the public pages, keep
// personal pages out, and point at the sitemap.
//
// The wildcard already admits every AI crawler (GPTBot, ClaudeBot, PerplexityBot,
// …), so no per-bot Allow block is listed — a redundant one would only invite
// drift from this rule. llms.txt has no robots directive of its own, so it is
// advertised as a comment, the convention crawlers look for.
//
// The API is advertised the same way, and for a self-interested reason: AI
// crawlers are the majority of this site's traffic, and every page they render
// is one unauthenticated JSON call that returns MORE than the HTML does (the
// facets a page only renders are fields in the response). A crawler that takes
// the hint costs us an SSR render instead of thousands and gets better data, so
// the pointer is cheap even at the low odds any given bot reads comments.
// Comments, not directives: robots.txt has no field for "prefer this instead",
// and inventing one would only be ignored by parsers that validate strictly.
//
// /jobs/*/discussion/new and /companies/*/discussion/new are the empty
// new-thread form, linked from every single job and company page — crawling it
// costs a full SSR render per job/company for a page with no unique content, and
// it drove a real accept-queue incident (2026-08-05, ClaudeBot alone made
// ~108k requests to it in 12.5h). Actual thread pages (/discussion,
// /discussion/[id]) hold real content and stay crawlable.
export const GET: RequestHandler = ({ url }) => {
  const body = `User-agent: *
Allow: /
Disallow: /my/
Disallow: /jobs/*/discussion/new
Disallow: /companies/*/discussion/new

Sitemap: ${url.origin}/sitemap.xml

# Bots and agents: you do not have to scrape these pages.
# The whole catalogue is a public, unauthenticated JSON API, and it returns more
# than the HTML does: canonical skills, country and region codes, seniority,
# work mode, salary bands.
#
# llms.txt:  ${url.origin}/llms.txt
# OpenAPI:   ${url.origin}/openapi.yaml
# API docs:  ${url.origin}/docs/api
# MCP:       ${url.origin}/mcp
# CLI:       ${url.origin}/cli
#
# One search: GET ${url.origin}/api/v1/jobs/search?q=golang
# Full descriptions in one call: GET ${url.origin}/api/v1/agent/jobs/search
`;
  return new Response(body, {
    headers: {
      'content-type': 'text/plain; charset=utf-8',
      'cache-control': 'public, max-age=86400',
    },
  });
};
