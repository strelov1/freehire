## Context

`internal/search`'s `ai_archetype` facet (PR #1673) is a filterable Meilisearch
attribute and already flows through `GET /api/v1/jobs/facets` with live
counts — nothing server-side needs to change. `web/src/lib/facets.ts` is a
single data-driven registry (`FACETS: FacetDef[]`) that every downstream
piece (URL sync, the filter modal, the store, live-count merging) iterates
generically by `param`; adding a facet is adding one entry plus its
value/label source, not touching multiple components.

Two existing small, fixed-vocabulary facets are the direct precedent:
`category` (`CATEGORY_VALUES` generated from Go, `CATEGORY_LABELS`
hand-maintained in `labels.ts`) and `domains` (same shape). `role` is a
different shape — a ~200-entry dynamically curated dictionary
(`roletag.Catalog()`), generated end-to-end because hand-maintaining that
many labels twice would rot. `ai_archetype` has six fixed, rarely-changing
values, so it fits the `category`/`domains` shape, not `role`'s.

## Goals / Non-Goals

**Goals:**
- Make `ai_archetype` selectable in the filter modal and sidebar, with live
  counts, using the existing generic facet machinery.
- Keep the frontend's valid-value list generated from the Go source of
  truth, so it cannot silently drift from what the backend actually derives.

**Non-Goals:**
- No backend behavior change (PR #1673 already wired the API/index side).
- No `roletag.Catalog()`-style generated label map — six values don't
  warrant it; see Decisions.
- Not blocking on the prod reindex that populates `ai_archetype` on existing
  jobs (a separate, already-tracked ops step) — the UI code is correct
  regardless of when that runs, and holding it back would gate unrelated work
  the user has explicitly chosen to trade off.

## Decisions

### 1. Values generated from Go, labels hand-maintained in `labels.ts`

Add `vocab.AIArchetypeValues []string` (the six literal slugs), consumed by
one new `emitVocab(...)` line in `cmd/gen-contracts/main.go` — the same path
`CATEGORY_VALUES`/`SENIORITY_VALUES` already use, so `make gen-contracts`
regenerates `AI_ARCHETYPE_VALUES` into `web/src/lib/generated/contracts.ts`.
A test cross-checks `vocab.AIArchetypeValues` against `internal/aiarchetype`'s
own rule-table archetype slugs (mirroring how `internal/roletag`'s test suite
cross-checks against `vocab.CategoryValues`), so the two can't drift apart.

`AI_ARCHETYPE_LABELS` is added to `web/src/lib/labels.ts` by hand, in the
same block/pattern as `CATEGORY_LABELS`, matched 1:1 against the generated
values:

```ts
export const AI_ARCHETYPE_LABELS: Record<string, string> = {
  rag_app_builder: 'RAG Application Builder',
  agent_builder: 'Agent Builder',
  cloud_ml_platform_engineer: 'Cloud/ML Platform Engineer',
  ml_trainer_researcher: 'ML Trainer/Researcher',
  fullstack_ai_engineer: 'Full-Stack AI Engineer',
  devops_infra_engineer: 'DevOps/Infra Engineer',
};
```

Alternative considered: mirror `roletag.Catalog()` (a Go-owned slug→label
map, generated end-to-end like `ROLE_LABELS`). Rejected — that machinery
exists because `roletag` curates ~200 dynamically-growing named roles, where
hand-maintaining labels twice would rot. `ai_archetype`'s six values are
fixed by the rule table in `internal/aiarchetype/aiarchetype.go` and change
about as often as `category`'s do; `category`'s lighter
generated-values-plus-hand-labels shape is the correct-weight precedent, not
`role`'s.

### 2. Registration in `facets.ts`: static `select`, not dynamic

```ts
const AI_ARCHETYPE: FacetOption[] = options(AI_ARCHETYPE_VALUES, AI_ARCHETYPE_LABELS);
...
{ param: 'ai_archetype', label: 'AI Specialization', control: 'select', options: AI_ARCHETYPE, excludable: true, placeholder: 'Search AI specializations' },
```

placed immediately after the `category` entry (`facets.ts:527`). `dynamic:
true` (the `role`/`skills`/`countries` shape, options built from the live
distribution) is deliberately not used — with only six values there's
nothing to search or paginate, and a static list keeps the panel's option
order stable regardless of which values currently have zero counts (useful
before the reindex backfills historic jobs, and still correct after).

### 3. Ship ahead of the prod reindex

The facet will show 0 for every option until the backend's reindex (a
separate, already-tracked ops task, not part of this change) runs on
historic jobs. The user explicitly chose to ship the UI now rather than gate
it on that ops step — the code path is identical either way, and newly
ingested/reindexed AI jobs will populate counts incrementally regardless of
when the full reindex happens.

## Risks / Trade-offs

- **[Risk]** Empty-looking filter until the reindex runs — a user opening the
  filter modal sees "AI Specialization" with every option at 0. →
  **Mitigation**: accepted trade-off (see Decision 3); the reindex is already
  tracked as the backend change's task group 6 and is independent ops work.
- **[Risk]** `AI_ARCHETYPE_LABELS` hand-maintained in TS could drift from the
  Go rule table's slugs if a future change adds/renames an archetype without
  updating the frontend label map. → **Mitigation**: `AI_ARCHETYPE_VALUES` is
  generated (not hand-typed) and `options()` (the same helper `CATEGORY`
  already uses) falls back to a title-cased raw slug for any value missing
  from the label map — matching `categoryLabel`'s own fallback pattern in
  `labels.ts:135` — so a drift degrades gracefully (an ugly-but-correct label)
  rather than breaking.
