## Context

See proposal.md — Why. Four independent CI jobs on PR #1737 after the rebase. Specs are skipped (`skip_specs: true`).

## Goals / Non-Goals

**Goals:**
- Green `web` typecheck, `design-system` adoption, `backend` integration (the two named tests), and extension `build` typecheck on this branch.
- Keep the existing heal/reset *contracts* (tests stay; production code moves).
- Leave changes local until the user asks to push.

**Non-Goals:**
- Broader oxlint cleanup, new coverage, or Talent Network behaviour.
- Weakening or deleting the two failing integration tests.
- A general TypeScript `noUncheckedIndexedAccess` policy change beyond the files CI named.

## Decisions

1. **Profile counts: `null`, not `{}`.**
   - `counts` is typed `FacetCounts | null`. Failed `facetCounts` should leave the same empty UI as “not loaded”.
   - Alternative considered: fabricate a zero `FacetCounts` — rejected; consumers already treat `null`.

2. **Adoption: record the improvement.**
   - Extra Button/Input imports are intentional (contacts editor / profile). Use the script `--update` path and commit `adoption-baseline.json`.
   - Do not loosen the ratchet to `>=`.

3. **Do not change the two Go test assertions — split heal merge from seed-apply merge.**
   - `mergeSeedHeader` is seed-first on purpose (`TestMergeSeedHeaderFillsGapsOnly`, `applySeedContent`): a non-empty seed field replaces. Reset-from-résumé must keep that.
   - `healRecordHeader` currently reuses that helper, so GET on a tailored CV with `FullName: "Keep Me"` becomes `"From Blob"`. Heal must be keep-first: copy résumé identity only into *empty* fields on the existing document. Add a second helper (or invert arguments with a clear name); do not flip `mergeSeedHeader`.
   - Reset summary `"stale"` is a different bug: the fixture marks structure superseded (`resume_uploaded_at` after `resume_structured_uploaded_at`). Seed must treat that blob as unusable for *semantics* (summary/skills/projects) and use provisional contacts only. If `Structured()` / the seeder now returns the stale blob as current, restore the stale-window gate rather than accepting `"stale"` in the test.

4. **Extension typecheck: narrow possibly-undefined; don’t disable the rule.**
   - 47 errors are `T | undefined` on destructuring / `[0]` after CI’s check (already red on `main` post-#1733; still required here because this PR touches `design-system/`).
   - Prefer a small local helper or an early `if (!x) throw` in tests, and the same narrowing in `form.ts` / `combobox.ts` / `freehire.ts` production paths. Avoid `!` on every line.

5. **No push.**
   - Apply + commit locally when asked; push only on a later explicit request.

## Risks / Trade-offs

- **[Risk] One merge function serves heal and seed-apply with opposite precedence** → Mitigation: grep callers; split if they disagree rather than flipping the shared helper blindly.
- **[Risk] Stale-window fix regresses “pending blob still seeds body” (`TestBankedSeederPendingBlobSeedsSummarySkillsProjects`)** → Mitigation: run the whole `cv_*` integration set, not only the two failures.
- **[Risk] Extension fixes look unrelated on this PR** → Accepted; the `build` check is on the rollup. Keep the diff mechanical.
- **[Risk] Local green, remote still red until push** → Accepted.

## Migration Plan

Not a deploy. After apply: local verify the four commands, commit when requested, push only on ask, then re-check PR #1737.
