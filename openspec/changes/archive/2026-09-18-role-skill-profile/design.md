## Context

`insights_role_stats` (migration 0022) already holds role demand keyed by
(category, seniority, country), rebuilt nightly by `cmd/rollup-stats` as an atomic
delete-and-reinsert over `jobs`. `insights_skill_stats` holds skill demand beside it, and
its own migration comment records the rule that shaped it: *category and country are not
crossed in one row* — rows are `(skill,'','')`, `(skill,category,'')` or
`(skill,'',country)`. Seniority was never a dimension of skill demand at all.

The public surface is equally far along. `GET /api/v1/insights/roles` serves the ranked
roles; `web/src/routes/insights/roles/[category]/` lists one category's seniorities;
`loadInsightsGate` is the single coverage gate five call sites share; `sitemap-insights.xml`
publishes exactly what clears that gate. The per-category page ranks seniorities and then
has nowhere to send a click.

On the candidate side, `userprofile.skills` is the canonical skill set (the experience bank
folds banked atoms into it via `experience.Store.syncProfileSkills`), and
`internal/candidate/jobmatch.Compute` is an existing pure function that takes a job's
skills and a profile's skills and returns exact/adjacent/total, resolving neighbours
through `internal/dict/skilladjacency`.

So the missing piece is one dimension on one rollup, and one leaf page.

## Goals / Non-Goals

**Goals:**

- Answer "what does *this* role want" from the catalogue, per (category, seniority).
- Answer "how far am I from it" for a signed-in visitor, without a model call.
- Give the existing `/insights/roles/[category]` page a destination, and the insights
  sitemap more indexable leaves.
- Add no new binary, no new external dependency, no Meilisearch work and no reindex.

**Non-Goals:**

- Psychometric, sociometric or cognitive matching. It cannot be validated without hiring
  outcomes, which this product does not observe; a confidence number nothing can check is
  a proxy metric that lies.
- Non-IT roles. The category vocabulary and the `is_tech` gate are built for IT, and
  widening them is a separate, larger change.
- An employer-side "pre-populate this role's requirements" tool. Same data, different
  audience, different surface; it can be built on this rollup later without changing it.
- Requirement-grade precision. See the first risk.

## Decisions

### The role key is (category, seniority), not a normalized title

Alternatives: a normalized posting title (`cmd/build-suggestions` already mines those with
open-counts and a floor of 25), or (category, seniority, top skill).

(category, seniority) wins because the pair is **already a live entity** — a table, an
endpoint, a gate, a page and a sitemap shard all key on it — so every consumer joins for
free. It is drawn from two closed vocabularies (27 tech categories × 8 seniorities), so
the key space is bounded and enumerable, and every published cell is guaranteed a large
sample.

The cost is real and accepted: `Senior Backend` fuses Go, Java and PHP, so the
distribution describes the role's *market*, not one stack. A title key would answer the
stack question but splits one role across `Senior Go Engineer` / `Golang Developer` /
`Backend Engineer (Go)`, smearing exactly the statistic the page exists to show. Adding a
stack axis later is additive to this table's key, not a rewrite of it.

### Count from Postgres, in `cmd/rollup-stats`, not from Meilisearch facets

Alternative: `search.FacetCounts` filtered per role, folded into `cmd/rollup-facets` — one
facet query per role, ~216 of them.

Postgres wins on three counts. First, **provenance**: the sibling role rollup is already a
`GROUP BY category, seniority` over `jobs` in this very worker, so the new aggregate joins
its transaction and inherits its atomicity, its `@min_sample` parameter and its tests. A
Meili-sourced table beside a Postgres-sourced one would give two role rollups two different
definitions of "open". Second, **cost is already measured**: the new aggregate is
`FROM jobs, unnest(skills)`, the exact shape of the existing
`RebuildInsightsRoleStatsByCountry`'s `FROM jobs, unnest(countries)`, running nightly
today. `skills` is a small `text[]`, not the TOASTed `description`, so this is not the
de-TOAST trap. Third, **no new unit**: extending a deployed worker sidesteps the
build-list-is-not-`ls cmd/` trap that left `close-chronic-boards` undeployed.

### No geography axis on the skill rollup

27 × 8 × skills is already the widest key in the insights family. Crossing it with country
multiplies it by the country cardinality for a slice nobody has requested, and the sibling
table's own comment already settled this trade for skill demand. The endpoint therefore
serves a country-scoped `open_count` beside a country-agnostic distribution, and `meta`
says so rather than letting a caller assume the whole answer moved.

### Extend `GET /api/v1/insights/roles`; do not add a route

Naming a single role is a narrowing of the existing ranked read, not a different question.
A second endpoint would duplicate the category/country/sort vocabulary and give
`UnknownParams` a second place to drift. `seniority` must be added to this endpoint's
read-parameter vocabulary in the same commit — an unread param here would be reported in
`meta.ignored_params` while silently widening the answer to every seniority, which is the
exact failure mode `country=it` demonstrated.

### The share divides by skill-bearing postings, not by the role's open count

Surfaced by the measurement, not by the design: 11% of the eligible postings (392,020 →
348,060) carry no tagged skill at all. Dividing by the whole open count would fold our own
tagging gap into every published share, and since that gap differs per role it would make
two roles' shares incomparable — the one comparison the page exists to support. The
denominator is therefore the role's skill-bearing postings, served as `sample_size` so the
figure is inspectable rather than implied. `open_count` and `sample_size` stay distinct
fields for the same reason: they count different things, and one field standing for both
is how a number starts arguing with the code.

### Coverage reuses `jobmatch.Compute` unchanged

`jobmatch` takes `(jobSkills, profileSkills)` and is I/O-free; a role's ranked skills are
just another skill list. Reusing it means adjacency behaviour cannot drift between "how
well do I match this job" and "how well do I match this role", which is precisely the
comparison a user will make between the two pages. Layering holds: `candidate` is layer 4
and `api/handler` is layer 8, so the handler composes `market-insights` data with
`userprofile` and `jobmatch` without any block importing sideways.

### Coverage rides on the same response, but is never shared-cached

A second authenticated call would double the round trips for one screen and force the page
to reconcile two orderings of the same skill list. The aggregate half is happily
`s-maxage`-able and the sibling insights pages set exactly that; the moment a coverage
section is present the response must be `private`. This is a footgun worth naming in the
handler, because the surrounding pages all set a shared-cache header by default.

## Risks / Trade-offs

- **"Mentioned" is not "required".** `jobs.skills` tags a skill named anywhere in the
  description — a nice-to-have list, a stack blurb, a benefits paragraph. Every published
  share is an upper bound. → The spec makes the wording normative: surfaces say "mentioned
  in", never "required by". `reqextract`'s `priority: required` lines are the honest
  upgrade path, deliberately deferred until its real coverage is measured — a
  precision-weighted number computed over the minority of postings whose markup parses
  would describe employers who write tidy HTML, not the market.
- **The sample floor may empty the list rather than rank it.** The social digest's floor of
  10 selected exactly one posting in a day and had to drop to 3. → Ship the floor as a
  worker knob, read it with the strict `worker.EnvInt` reader (so a typo fails the run
  rather than silently taking a default), and set its value from the first real run's
  numbers instead of guessing now.
- **`seniority` coverage is 39%, and this risk FIRED.** Measured on production 2026-09-18:
  of 1,012,085 open `is_tech` postings, 998,688 (98.7%) carry a `category` but only 394,610
  (39.0%) carry a `seniority`; 392,020 carry both and 348,060 of those carry at least one
  tagged skill. The missing 61% are not a random sample — they are exactly the postings
  whose title names no level, so the rollup describes postings that STATE a seniority, not
  the role's market. → Three mitigations, all cheap, none of which is "measure it later":
  (a) the spec now makes the sample size a served field and forbids any surface from
  claiming to describe the role's market; (b) the leaf page shows the category-only
  distribution beside the role's — that slice has 98.7% coverage and **already exists** in
  `insights_skill_stats`, so one number backstops the other for zero new work; (c) 392k
  postings across 216 cells is still a large sample where it matters, and the floor
  suppresses the cells where it does not. Not a blocker: an honest statistic over a
  labelled subset is worth more than no statistic, provided it never pretends to be the
  other thing.
- **A wide key with a nightly full rebuild.** 216 roles × their skills is more rows than
  either sibling. → The floor bounds it; the delete-and-reinsert stays inside the existing
  transaction so a reader never sees a partial rebuild; and the aggregate's shape is one
  already running against this table nightly.
- **Traffic is a slow bet.** Googlebot fetched 65 job pages in a day against ~690k in the
  sitemap, and sampled postings answer "URL is unknown to Google". New leaves will not be
  discovered quickly. → Judge this by the coverage-gap feature and by Bing/company-page-style
  long-lived pages, not by a fast impression lift; measure with URL inspection, not
  impressions.

## Migration Plan

1. Migration adding `insights_role_skill_stats`. A plain `CREATE TABLE`, so it does not
   need a lock window on `jobs`.
2. Deploy the worker change; let one nightly `cmd/rollup-stats` run fill the table.
3. Read the run's numbers: rows produced, roles that cleared the floor, and the seniority
   coverage share. Set the floor from these.
4. Deploy the endpoint. It is additive — without `seniority` the response is byte-identical
   to today's.
5. Deploy the page and the sitemap entries last, once step 3's numbers justify publishing.

Rollback at any step is to stop serving the page; the table and the rollup are additive and
their presence costs the existing insights surfaces nothing.

## Open Questions

- The two numeric knobs (skill sample floor, and any minimum role size required to publish
  a page) are deliberately unset here. They are resolved by step 3's measurement, not by
  this document.
