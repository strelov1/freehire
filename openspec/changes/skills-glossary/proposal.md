## Why

The canonical skill vocabulary has two sides — the slug (`ci-cd`, `dbt`, `1c`) and the
display label (`CI/CD`, `dbt`, `1C`, `internal/dict/skilltag/labels.go`). Neither tells a
reader what the thing *is*. A candidate meets 863 canonical tokens across the filter
panel, job cards and the CV-match delta with no way to learn "dbt is a SQL transformation
tool for data warehouses" without leaving the site.

Three surfaces pay for that today: filter discovery (people skip facets they do not
recognise), the skill-gap delta (a bare gap is not an actionable one), and search — a
curated glossary of real IT skills is a linkable, indexable asset, the same play
`/roles` already runs.

Closes [#1983](https://github.com/strelov1/freehire/issues/1983).

## What Changes

- **A curated description dictionary** next to the labels that own the same vocabulary:
  `internal/dict/skilltag/descriptions.go`, one or two plain-language sentences per
  canonical, English only. A static reviewed artifact — the vocabulary never calls a
  model at request time, and this does not change that.
- **A generator, run once per wave, not at runtime**: `cmd/gen-skill-descriptions` reads
  the canonical set plus each skill's own aliases, asks the LLM for a definition, and
  prints a Go map for a human to review. Nothing it emits ships unreviewed.
- **Delivery to the SPA over the existing codegen seam**, but in its own module:
  `cmd/gen-contracts` writes `web/src/lib/generated/skillDescriptions.ts` rather than
  adding ~110 KB to `contracts.ts`, which every visitor loads. The SPA imports it
  dynamically, so a reader who never opens a definition never downloads one.
- **Reveal on the chip**: the existing `design-system` tooltip gains a tap-to-open path
  (it is hover/focus-only today) and skill chips wrap in it. The chip's own link still
  goes to the `/jobs` filter; the tooltip carries the definition and a link on.
- **A glossary surface**: `/skills` (the index) and `/skills/<slug>` (definition, the
  spellings the parser accepts, neighbouring skills, and the live postings for that
  facet), server-rendered, listed in `sitemap-skills.xml`.
- **Coverage is a test, and it ratchets.** Descriptions land in waves ordered by how
  often a skill appears in the catalogue; a floor constant records how many are written
  and only ever rises. When it reaches the whole vocabulary the floor is replaced by the
  absolute rule the labels already carry: a canonical with no description fails the
  build.
- Fixes a bug in passing: `web/src/lib/components/JobView.svelte` renders the raw slug
  on a job's skill chips, so a posting reads "ci-cd" where the filter panel reads
  "CI/CD".

Not in scope: rewriting the skill vocabulary, i18n of the descriptions, and any
request-time model call.

## Capabilities

### New Capabilities

- `skill-glossary`: the curated description for each canonical skill — where the text
  lives, how coverage is enforced, how it reaches the SPA, and the two surfaces that
  reveal it (the chip tooltip and the `/skills` pages).

### Modified Capabilities

<!-- None. `facet-display-labels` explicitly excludes skill tags from its shared-map
     requirement, and this change adds a second field beside the labels rather than
     changing how any facet code is labelled. -->

## Impact

- `internal/dict/skilltag` — new `descriptions.go` and its tests; a new accessor
  exposing each canonical's aliases, which the glossary page renders and the generator
  prompts with.
- `cmd/gen-skill-descriptions` — new run-once worker (needs `LLM_BASE_URL` /
  `LLM_API_KEY` / `LLM_MODEL`, and `DATABASE_URL` to order the vocabulary by catalogue
  frequency).
- `cmd/gen-contracts` — emits one more generated file.
- `web/` — new `/skills` and `/skills/<slug>` routes, `sitemap-skills.xml` and its entry
  in the sitemap index, the skill chip on `JobView`/`JobRow`, and the generated
  descriptions module.
- `design-system/` — `tooltip.svelte` gains touch activation; a change visible to every
  existing tooltip consumer.
- No database migration, no API change, no new endpoint.
