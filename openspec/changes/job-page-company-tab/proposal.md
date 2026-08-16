## Why

A job page tells you everything about the role and almost nothing about who you would
work for. The catalogue already holds curated company facts and a full company summary
(`companies.company_info`, populated by the YC and hirebase importers), but today they
are reachable only by leaving the posting for `/companies/<slug>`. Somebody deciding
whether a role is worth an application has to abandon the page to answer "who are these
people?" — and often does not come back.

## What Changes

- The job detail page gains a two-tab strip over its content column: **Description**
  (the current content, default) and **Company**.
- The Company tab loads the company on first activation and shows the same facts card
  and About summary the company page already renders, plus a link through to the
  company's full page.
- The strip appears only on jobs that carry a `company_slug`. Jobs without one render
  exactly as they do today.
- The company content is deliberately **not** server-rendered. Inlining a company's
  summary into the HTML of every one of its postings would put hundreds of near-identical
  pages in competition with `/companies/<slug>`, which is the page that should rank for
  that company. Lazy loading keeps the copy out of the crawlable HTML while the existing
  server-rendered link to `/companies/<slug>` continues to carry the internal-link value.

No breaking changes.

## Capabilities

### New Capabilities

None. This adds a second surface for a capability that already exists.

### Modified Capabilities

- `company-info-display`: currently specifies company-info rendering on the company
  detail page only. A requirement is added for the job detail page surface — a lazily
  loaded Company tab, its absent/empty/failed states, and the rule that this surface is
  never server-rendered.

## Impact

- `web/src/lib/components/JobView.svelte` — the content column gains the tab strip.
- `web/src/lib/components/JobCompanyPanel.svelte` — new; owns the fetch, the panel
  states, and reuses `CompanyFacts` and `CompanyAbout`.
- No backend change. The panel calls the existing `GET /api/v1/companies/:slug` through
  `api.getCompany`.
- No change to canonical URLs, `robots`, sitemaps, or the job route's `+page.svelte`.
- Verification is by `pnpm --dir web test` over the lifted derivations, then
  `svelte-check` and headless-Chrome inspection for the markup and wiring. `web`'s vitest
  runs in `environment: 'node'` with no Svelte compilation, so a component itself cannot
  be rendered in a test — which is why the feature's only real logic was lifted into a
  plain module where it can be.
