## Context

Three curated dictionaries describe a job, and marketing is thin in all three:

| Layer | Package | Marketing coverage today |
|---|---|---|
| category (one value) | `internal/classify` | 13 aliases, all → `marketing` |
| roles (a list) | `internal/roletag` | 4 named roles + `head_of_marketing`/`head_of_growth` |
| skills (a list) | `internal/skilltag` | `seo`, `analytics`, `crm`, `hubspot`, `marketo`, `canva`, `community-management` |

Prod `/api/v1/insights` counts ~18.1k open marketing vacancies with a resolved
grade. `roletag.go` already carries the comment `// Marketing (granular names the
coarse "marketing" category flattens)` — the seam for this change was left open
deliberately.

Constraints that shape the work:

- `marketing` and `sales` are `vocab.NonTechCategories` members, so anything landing
  there derives `is_tech=false` and is skipped by LLM enrichment and embeddings.
- Deletion is **not** governed by the category. `classify.ConfirmedNonTech`
  (`nontech.go:202`) reads the separate `nonTechTitleTerms` list, which this change
  does not touch — so no alias added here can make a posting removable.
- `skilltag` gates a set of `ambiguousWords` (`dictionaries.go:646`) that only tag
  when the same text carries a strong, concrete technology token. **`seo` is one of
  them.**
- `jobderive.go:164` builds the skills facet from the **description only**; the
  title is not parsed for skills on the ingest path.

## Goals / Non-Goals

**Goals:**

- Make each marketing discipline separately filterable through the `role` facet.
- Resolve the tooling these postings name, so a discipline filter can be combined
  with a tool filter.
- Partition the `GTM` and `GEO` homonyms so neither meaning contaminates the other.
- Leave every existing matcher mechanism, category value and requirement untouched.

**Non-Goals:**

- Splitting `marketing` into sub-categories. It would add members to
  `vocab.CategoryValues`, force a placement decision in the tech/non-tech partition
  test, and touch four deletion paths — cost with no filtering benefit that the
  `role` facet does not already provide.
- Enriching marketing postings with the LLM. They stay off that budget.
- Re-deriving prod data automatically. Backfill is a task, run deliberately.

## Decisions

### 1. Granularity lives in `roletag`, not in new categories

Alternative considered: `seo`, `smm`, `growth` as `CategoryValues`. Rejected —
a category is single-valued, and these disciplines overlap in real titles ("SEO &
Content Marketing Manager"), which a list-valued facet models honestly and a
single-valued one does not. `roletag` is already list-valued, already emits named
roles independent of the seniority×category grid, and already exports labels to
the web contracts through `cmd/gen-contracts`.

### 2. `GTM` belongs to go-to-market; the tag manager must be spelled out

The first draft of this design read the abbreviation as Google Tag Manager and put
it in `sharedAcronyms`. That is backwards for this corpus: in a posting, `GTM` is
overwhelmingly go-to-market ("own our GTM strategy", "shape the GTM motion", "GTM
Engineer"), while the tag manager is written out or named as a container. Reading
the bare token as the product would mislabel the marketing population wholesale.

- `roletag`: `gtm engineer`, `go-to-market engineer`, `go to market engineer` →
  `gtm_engineer`. The bare token is never a role alias.
- `classify`: the same phrases → `sales`, placed before the bare `sales` entry —
  the ordering discipline the file already uses for `solutions_engineering`.
- `skilltag`: `go-to-market` canonical from `go-to-market`, `go to market`, `gtm
  strategy`, `gtm motion`, `gtm plan`, `gtm execution`. `google-tag-manager` keeps
  its spelled-out phrase (already in the dictionary) and gains `gtm container`.
  There is no `GTM` acronym entry in any case form.

### 3. `GEO` resolves only from disambiguated phrases

`geo` collides with the whole of `internal/location` and with geospatial
engineering. So:

- `roletag` aliases: `generative engine optimization`, `answer engine optimization`,
  `generative search optimization`, `geo specialist`, `geo manager`, `aeo specialist`,
  `aeo manager` → one slug, `geo_specialist`. The industry uses GEO, AEO and GSO for
  the same job, so they collapse to one role rather than fragmenting the facet.
- `skilltag` phrase aliases: the spelled-out forms → `generative-engine-optimization`.
  No bare `geo`, no bare `aeo`.
- The title `Geo Data Analyst` matches no alias and is untouched.

### 4. Marketing title aliases must be phrases

`growth`, `content`, `brand` and `performance` are forbidden as standalone aliases:
`growth` alone would claim `Growth Engineer` (an existing `roletag` role, currently
technical), flipping it to `marketing`, `is_tech=false`, and off the enrichment and
embedding budgets. Only phrases that name the marketing role are added
(`growth marketing`, `content marketing`, `brand manager`, `performance marketing`).
A regression test asserts `Growth Engineer` keeps its pre-change classification.

### 5. New tool canonicals double as corroborators for `seo`

`seo` is an `ambiguousWords` member and only tags when the text carries a strong
concrete token. Semrush, Ahrefs, Screaming Frog and Google Search Console are exactly
that kind of token — unambiguous product names, the marketing equivalent of
`kubernetes`. Adding them as **strong** (ungated) aliases therefore also recovers
`seo` on postings that name their toolchain but no engineering technology. This is a
deliberate secondary effect, not an accident.

`sem` is **not** added in any form — it is a Portuguese and Spanish preposition
("sem experiência") and this catalogue carries that population in bulk. `ppc` is a
phrase alias only (`ppc campaigns`, `ppc specialist`, `paid search`, …).

### 6. Discipline phrases tag but do not corroborate

The disciplines cannot simply join the phrase list. Every phrase match lands in
`strong`, and one strong match rescues every `weak` one — so `content marketing`
would vouch for the gated `ai`, and "AI-powered content marketing" (marketing
boilerplate, not a requirement) would tag the whole population with `ai`. An
existing test, `TestParse_AmbiguousCorroboration`, caught exactly this.

`Parse` therefore gains a third bucket. `nonCorroboratingPhrases` names the
discipline canonicals; a match on one goes into `standalone`, which is unioned into
the result **after** the corroboration step. So a discipline tags on its own —
it is a certain enough signal — but can never rescue a gated word. Named products
stay in `strong` and keep corroborating, which is what recovers `seo` (decision 5).

Alternatives rejected: routing disciplines through the existing `weak` bucket would
have made them vanish on postings that name nothing else, which is precisely the
posting they describe best; dropping the disciplines entirely would have removed
half the filtering value this change exists to add.

### 7. Tool names that are ordinary words are excluded, not gated

Segment, Buffer and Later are real products in this space and are absent from the
vocabulary. Unlike `amplitude` — a physics noun that is nonetheless rare in job
prose, so `ambiguousWords` handles it — these three occur constantly in exactly the
postings the block serves: "the customer segment", "a content buffer", "apply
later". Gating would not help, since a marketing posting reliably carries other
strong marketing tokens that would corroborate them.

## Risks / Trade-offs

- **A marketing alias claims a technical title** → every alias is a phrase, never a
  bare discipline noun; regression tests pin `Growth Engineer`, `Content Platform
  Engineer` and `Geo Data Analyst` to their pre-change classification.
- **`GTM` uppercase appears in non-marketing prose** (e.g. a German posting's
  "GmbH … GTM") → the acronym pass is whole-word and uppercase-only; the same
  exposure the existing `ML` entry carries, and `google-tag-manager` on a stray
  posting is a low-cost false positive.
- **Ad-platform names collide with the vendor** — "Google Ads" must not also emit a
  generic Google canonical → phrase aliases are matched before the word pass, the
  same mechanism that keeps `google cloud` off `google-analytics`.
- **Facet fragmentation** — three names for one job (GEO/AEO/GSO) could become three
  slugs → they collapse to one canonical slug by decision 3.
- **The change does not reach existing jobs by itself** → backfill + reindex is an
  explicit task, and the reindex must not be stacked with `reindex-companies`.

## Migration Plan

1. Merge. New jobs get the new roles, skills and categories on the next ingest.
2. `go run ./cmd/backfill-derive` — re-derives `skills` and `category` on existing
   rows. Roles need no backfill: they are computed at index time.
3. `make reindex` — populates `roles` on existing documents and picks up the
   re-derived skills. **Never** concurrently with `reindex-companies`.
4. Rollback is a revert: the dictionaries are pure data, nothing is persisted that
   a re-run of steps 2–3 does not overwrite.

## Open Questions

None blocking. The exact per-cluster alias lists are settled during implementation
against the requirement that every alias be a phrase.
