// The robots.txt body. A builder rather than a template inside the route, for the
// same reason sitemap.ts is one: the rule it encodes is easy to state and easy to
// break silently, so it belongs somewhere a test can reach.

// Paths no crawler should take. /my/ is personal. /jobs/*/discussion and
// /companies/*/discussion are the discussion surface, linked from every single job
// and company page — crawling it costs a full SSR render per job/company, and the
// new-thread form under it drove a real accept-queue incident (2026-08-05,
// ClaudeBot alone made ~108k requests to it in 12.5h).
//
// The whole subtree, not just /new. This rule used to stop at the empty form on the
// reasoning that thread pages hold real content — true of what they are for, and not
// yet true of what they serve. Measured 2026-09-22: 16 of 16 sampled company
// discussion pages were empty, including google, microsoft, amazon and openai, each
// rendering the same 440-word shell with no thread on it. That is ~321k identical
// pages, and Googlebot spent 257 of its ~1,300 daily fetches walking them
// alphabetically while reaching almost none of the catalogue it came for.
//
// Two costs, not one. The budget is the obvious half; the other is that a template
// repeated across a 321k-page address space is exactly the shape a crawler reads as
// scaled content, on a site whose measured problem is already that Google crawls its
// pages and declines to index them.
//
// REVISIT WHEN THREADS EXIST. This is a measurement, not a verdict on the feature —
// re-sample before assuming it still holds, and narrow the rule back to /new once a
// meaningful share of these pages carry a thread.
//
// /signin is the same shape and larger. Every sign-in entry point in the app goes
// through `signinUrl()`, which carries the page it was clicked from in `returnTo`
// — so the address space of /signin is the size of the catalogue, and a crawler
// walking the job list finds a fresh /signin URL on every page. Measured
// 2026-09-20: 127,799 fetches across 41,180 distinct /signin URLs, 10% of
// everything the host served that day, 114,864 of them from one crawler.
//
// It also ROUTES AROUND the rule above it: many of those `returnTo` values point
// at /jobs/*/discussion/new, which is disallowed and which a crawler therefore
// cannot fetch directly — but the sign-in link to it is not, and was not.
//
// And the page it lands on is where the four OAuth buttons live, which is how
// 11,032 bot-initiated authorization redirects reached Google, LinkedIn, Apple
// and GitHub in that same day. That one is not our CPU; it is four identity
// providers seeing this application start ~2,700 sign-ins a day that no person
// asked for. The page carries `noindex, nofollow` for the same reason — see
// routes/signin/+page.svelte, which is the half of this that reaches a crawler
// that does not read robots.txt at all.
export const DISALLOWED = [
  '/my/',
  '/signin',
  '/jobs/*/discussion',
  '/companies/*/discussion',
];

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
