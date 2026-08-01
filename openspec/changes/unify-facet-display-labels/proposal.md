## Why

`web/src/lib/labels.ts` opens by declaring itself the single source of facet display
labels — "Keeping ONE map prevents the drift that previously left stale region codes and
inconsistent casing in two places." Three surfaces render those labels, and only one reads
that map exclusively. The `/insights` SEO pages carry a second, 35-entry `CATEGORY_LABELS`
of their own, and the job-detail page reaches the shared map through a *different* fallback
function than the filter panel does. Both forks have already produced visible disagreement
on pages Google indexes.

The drift is not a tidiness complaint — the same facet code renders under different names
depending on which page the reader is on:

| code | filter panel | job detail page | /insights (indexed) |
|---|---|---|---|
| `ai_engineering` | AI Engineer | AI Engineer | AI Engineering |
| `fullstack` | Fullstack | Fullstack | Full-Stack |
| `network_engineering` | Network Engineering | Network engineering | Network Engineering |
| `not_supported` (relocation) | None | Not supported | — |

The root cause is structural rather than clerical. `CATEGORY_LABELS` deliberately lists only
the codes whose label differs from a title-cased fallback, but two surfaces supply *different*
fallbacks: `facets.ts` title-cases every word, `enrichment.ts` capitalises only the first. So
every multi-word category the map omits renders two ways by construction, and the `/insights`
fork exists precisely because neither fallback was trustworthy enough to rely on.

## What Changes

- `CATEGORY_LABELS` becomes **exhaustive** over the generated `CATEGORY_VALUES`, so the
  fallback cannot fire for that closed vocabulary and the two fallback styles stop mattering
  for categories. `satisfies Record<Category, string>` binds the map to the generated
  vocabulary in both directions, so an unlabelled new category and a stale leftover label
  each fail `pnpm run check` — which `main`'s required `web` CI job runs.
- The `/insights` pages lose their private `CATEGORY_LABELS` and `SENIORITY_LABELS`; they read
  the shared maps. Their category-page titles, H1s and auto-intro sentences keep rendering the
  same strings they render today.
- `RELOCATION_LABELS` moves into `labels.ts` and is read by both the filter panel and the
  job-detail facet row, replacing two divergent inline maps.
- One title-cased fallback (`titleCase`) is owned by `labels.ts` and imported by `facets.ts`,
  instead of being declared in both. `enrichment.ts`'s sentence-cased fallback stays and is
  renamed `sentenceCase` — its current name and doc comment claim title case and demonstrate
  sentence case, which is how the divergence stayed invisible.
- Wording decisions settled by the product owner and applied once, in one place:
  `ai_engineering` → **AI Engineering**, `fullstack` → **Full-Stack**,
  relocation `not_supported` → **Not supported**.

Visible consequences, all of them convergences onto a string the product already shows
somewhere:

- The filter panel and the job page start saying "AI Engineering" and "Full-Stack".
- The filter panel's relocation pill starts saying "Not supported" instead of "None".
- Seven multi-word categories on the job page gain their missing capital
  (`network_engineering`, `engineering_design`, `business_analysis`,
  `solutions_engineering`, `developer_relations`, `technical_writing`,
  `customer_success`), matching the panel and the indexed pages.
- `/open` starts saying "C-level" and "On-site" in its seniority and work-mode
  distributions, matching every other surface.

`/insights` page titles, H1s and intro sentences are byte-identical apart from the two
settled category wordings.

No **BREAKING** API change: these are display strings in the SPA. The filter *values* sent to
the API are the underlying codes and are untouched.

## Capabilities

### New Capabilities

- `facet-display-labels`: how the SPA turns a closed-vocabulary facet code into the text a
  reader sees, and the rule that one code renders as one string on every surface — the filter
  panel, the job-detail facet rows, and the indexed `/insights` pages.

### Modified Capabilities

None. No existing spec states a requirement about how a facet code is spelled on screen;
`market-insights` governs which `/insights` pages get published and `deterministic-facets`
governs how the codes themselves are derived, and neither is affected.

## Impact

- `web/src/lib/labels.ts` — gains the exhaustive category map, `RELOCATION_LABELS`,
  `titleCase`, `categoryLabel`.
- `web/src/lib/insights.ts` — loses two maps and its local `categoryLabel` body.
- `web/src/lib/facets.ts` — loses its local `humanize` and `categoryLabel`, and its inline
  relocation overrides.
- `web/src/lib/enrichment.ts` — loses its inline `RELOCATION`; `humanize` renamed.
- `web/src/routes/open/+page.svelte` — its seniority and work-mode buckets read the shared
  maps instead of a private fallback; skills keep the local one, being an open vocabulary.
- Import sites: `web/src/lib/filterSections.ts`, `web/src/lib/components/ProfileForm.svelte`,
  and the three `web/src/routes/insights/*/[category]/+page.server.ts` loaders.
- Tests: `web/src/lib/insights.test.ts` (the `categoryLabel` cases move, a
  `seniorityLabel` case is added), plus a new `web/src/lib/labels.test.ts` pinning the
  settled wordings and the fallback.
- No backend, database, API or Meilisearch change. No migration.
