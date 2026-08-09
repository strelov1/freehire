## Why

The `ai_archetype` facet (added in `refine-ai-role-classification`, PR #1673)
is filterable through the API today but has no presence in the web UI — a
user cannot discover or select it. `internal/search`'s facet-count endpoint
already returns live counts for it with zero further backend work, so the gap
is purely on the frontend: the facet needs a place in the filter registry, a
label for itself and its six values, and a spot in the filter modal.

## What Changes

- Expose the six `ai_archetype` values as a generated enum
  (`vocab.AIArchetypeValues` → `AI_ARCHETYPE_VALUES` in the web contracts),
  the same generation path `CATEGORY_VALUES` already uses — so the frontend's
  valid-value list can never drift from the backend's.
- Add `AI_ARCHETYPE_LABELS` to `web/src/lib/labels.ts`, hand-maintained
  against the generated values (the same pattern `CATEGORY_LABELS` and
  `DOMAIN_LABELS` already follow) — a small, fixed 6-value vocabulary does
  not warrant `internal/roletag`'s heavier generated-`Catalog()` machinery,
  which exists for its ~200-entry dynamically curated dictionary.
- Register a new "AI Specialization" facet in `web/src/lib/facets.ts`
  (`control: 'select'`, static options built from the values/labels above,
  live counts merged in automatically — the same mechanism `category` and
  `domains` already use), positioned immediately after `category` in the
  facet list and the filter modal's rail.
- Ship with the facet live even though it will show zero results everywhere
  until the backend's pending prod reindex populates `ai_archetype` on
  existing jobs — the code path is independent of that data and correct
  either way, and holding the UI back would gate an unrelated ops step.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `ai-role-archetype`: the six archetype slugs are additionally exposed as a
  generated web-contracts enum (`AI_ARCHETYPE_VALUES`), not only as an
  internal Go rule set.

`filter-modal` and `facet-display-labels` are deliberately NOT listed here:
registering `ai_archetype` in `facets.ts` and labelling it in `labels.ts`
are new *instances* of those capabilities' existing generic requirements
("every facet control shows a live match count", "a facet code renders as
one string on every surface" via `labels.ts`) — no new requirement or
mechanism is introduced, so there is nothing for a delta spec to state that
those capabilities don't already cover.

## Impact

- `internal/vocab`: new `AIArchetypeValues []string` (6 literal slugs,
  cross-checked against `internal/aiarchetype`'s rule table by a test).
- `cmd/gen-contracts/main.go`: one new `emitVocab(...)` line; regenerates
  `web/src/lib/generated/contracts.ts`.
- `web/src/lib/labels.ts`: new `AI_ARCHETYPE_LABELS` map.
- `web/src/lib/facets.ts`: new static options array + one `FACETS` entry.
- No change to `internal/search` (already wired in PR #1673) and no schema
  migration.
