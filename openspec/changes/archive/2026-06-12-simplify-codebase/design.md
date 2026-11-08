## Context

These tasks come from a completed, adversarially-verified simplification audit.
Each finding was double-checked by a skeptic that read the real code and rejected
anything that collapsed a deliberate seam or wasn't actually simpler. This doc
records only the decisions that had a live alternative — the rest are mechanical.

The codebase is early/MVP and intentionally fluid, but well-disciplined: every
surviving finding is `low` severity. The bar is the project's own principle —
"no overengineering AND no MVP shortcuts" — so each fix removes a real drag
without inventing infrastructure.

## Goals / Non-Goals

Goals:

- Remove dead code, verbatim duplication, and one hot-path `SELECT *`.
- Finish one half-built generic abstraction so it stops lying about being generic.
- Keep every external wire contract byte-for-byte identical.

Non-Goals:

- No behavior, API, or schema changes; no migrations; no new config or deps.
- No Go→TS codegen pipeline (explicitly rejected below).
- No speculative new abstractions; the `ui/input.svelte` primitive only mirrors
  the existing `button`/`badge` pattern already in the repo.

## Decisions

**smartRecruiters fan-out: `[]*Job` nil-sentinel, not an `ExternalID != ""` sentinel.**
The audit's first proposal reconstructed the success bit from `j.ExternalID != ""`.
Rejected: that couples an invisible invariant to an unrelated field and is more
fragile than the explicit `found []bool` it replaces. `detail()` already returns
`(Job, bool)`; the idiomatic Go collapse is one `[]*Job` slice where `nil` means
"skipped", written per-index (so the bounded worker pool stays concurrency-safe),
then compacted to `[]Job`.

**Generic And/Or facet: finish the generic path, don't special-case skills.**
`FacetSection` gates the toggle on the generic `def.hasAndOr` but hard-wires
`store.setSkillsAnd` / `store.value.skillsAnd`. The backend (`search.go
buildSearchFilter`) already reads `<param>_mode=and` for *any* faceted param, so
the seam is half-built on both ends. Decision: move match-all into `FacetState`
(`matchAll`, mirroring the existing per-param `exclude`), keyed by param — not the
cheaper "gate on `def.param === 'skills'` and delete `hasAndOr`", which would
regress toward a special-case against an already-generic backend. The wire output
for the skills facet stays byte-identical (`skills_mode=and`).

**Enum vocab drift (Go ↔ web `facets.ts`): a cheap guard, not codegen.**
A Go→TS generator was rejected as premature infra for a repo with zero codegen and
a vocab that has changed ~once. It also can't emit `facets.ts` cleanly: each
faceted list diverges by design (reorder, dropped values, label overrides; some
facets have no Go vocab and some Go vocabs aren't faceted). Decision: add a
source-of-truth comment in `facets.ts`, and — only for the ~6 lists that genuinely
mirror a Go closed vocab — a small Go fixture test that fails if a faceted
`*Values` list drifts from a checked-in expectation. The `humanize()` fallback
already prevents a missed value from rendering blank, so this is drift-flagging,
not a correctness fix.

**List-response envelope: a free function, not a `Handler` method.**
`listResponse(c, data any, total int64, limit, offset int)` lives next to
`pageParams` (also a free func). All three `total` sources are already `int64`,
so no cast and no `any` for total. Single-item `{data}` handlers stay untouched.

## Risks / Trade-offs

- [`make sqlc` regenerations (#2, #5) drift from hand edits] → Regenerate via the
  project's `make sqlc`, commit the generated file, and gate on `go build` +
  `go test` before marking the task done. The sqlc memory note covers the no-Docker
  fallback (`sqlc@v1.31.1`).
- [Web has no unit-test runner] → Frontend tasks (#7–#10) are verified by
  `svelte-check` + lint, not a new per-feature runner. The controlled-input URL-sync
  pattern is a known gotcha — keep the synchronous-in-handler write.
- [Generic facet refactor touches serialization] → `filtersToParams` /
  `filtersFromParams` change; the guard is the unchanged backend test
  (`skills_mode=and`) plus an end-to-end glance.

## Migration Plan

None. Pure refactor; each task is independently revertable. No deploy ordering.

## Open Questions

- #11 (vocab guard): comment-only, or comment + fixture test? **Resolved: comment-only.**
  A Go fixture test would assert the Go `*Values` slices against a checked-in copy
  in the test — guarding Go-against-itself, not the actual Go↔web `facets.ts` drift
  (a Go test can't read the TS list). That is ceremony, not a guard, so it was
  dropped per this section's own escape hatch. The honest cheap guard is a
  source-of-truth comment in `facets.ts` plus the existing `humanize()` fallback.
