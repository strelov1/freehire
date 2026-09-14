// The robots.txt body. A builder rather than a template inside the route, for the
// same reason sitemap.ts is one: the rule it encodes is easy to state and easy to
// break silently, so it belongs somewhere a test can reach.

// Paths no crawler should take. /my/ is personal. /jobs/*/discussion/new and
// /companies/*/discussion/new are the empty new-thread form, linked from every
// single job and company page — crawling it costs a full SSR render per job/company
// for a page with no unique content, and it drove a real accept-queue incident
// (2026-08-05, ClaudeBot alone made ~108k requests to it in 12.5h). Actual thread
// pages (/discussion, /discussion/[id]) hold real content and stay crawlable.
export const DISALLOWED = ['/my/', '/jobs/*/discussion/new', '/companies/*/discussion/new'];

// Search crawlers additionally lose /api/, and only they. The invitation in the
// comment block is aimed at an agent answering a question now; a search crawler is
// building an index, and a JSON endpoint is never a search result — so every fetch
// it spends there is taken from the HTML it came for. Measured 2026-09-14: 159 of
// Googlebot's 776 successful fetches that day were /api/ (100 of them
// /api/v1/threads/count), against only 65 distinct job pages crawled — a fifth of
// the budget, on a site whose measured problem is that Google reaches almost none
// of a 690k-page catalogue.
//
// Named rather than inferred: robots.txt has no "search engines" wildcard, and a
// crawler not named here keeps the wildcard group, which is the safe direction.
export const SEARCH_CRAWLERS = ['Googlebot', 'Bingbot'];

const SEARCH_CRAWLER_DISALLOWED = [...DISALLOWED, '/api/'];

const group = (agent: string, disallowed: readonly string[]) =>
  [`User-agent: ${agent}`, 'Allow: /', ...disallowed.map((path) => `Disallow: ${path}`)].join('\n');

// A crawler obeys the SINGLE most specific user-agent group that matches it and
// ignores every other one, so each named group repeats DISALLOWED in full. A
// Googlebot group listing only /api/ would silently re-open /my/ and the new-thread
// form to Google — which is why the two lists are composed here rather than written
// out twice.
export function robotsBody(origin: string): string {
  const groups = [
    group('*', DISALLOWED),
    ...SEARCH_CRAWLERS.map((agent) => group(agent, SEARCH_CRAWLER_DISALLOWED)),
  ];

  return `${groups.join('\n\n')}

Sitemap: ${origin}/sitemap.xml

# Bots and agents: you do not have to scrape these pages.
# The whole catalogue is a public, unauthenticated JSON API, and it returns more
# than the HTML does: canonical skills, country and region codes, seniority,
# work mode, salary bands.
#
# llms.txt:  ${origin}/llms.txt
# OpenAPI:   ${origin}/openapi.yaml
# API docs:  ${origin}/docs/api
# MCP:       ${origin}/mcp
# CLI:       ${origin}/cli
#
# Please send a User-Agent naming your project, like
#   acme/job-sync/1.4 (+https://github.com/acme/job-sync)
# Not required, nothing checks it. It just lets us warn you before a limit
# changes instead of you meeting a 429 cold.
#
# One search: GET ${origin}/api/v1/jobs/search?q=golang
# Full descriptions in one call: GET ${origin}/api/v1/agent/jobs/search
`;
}
