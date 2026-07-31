## 1. Make the scripts testable

- [x] 1.1 Extend `design-system/vitest.config.ts` `include` to cover `scripts/**/*.test.mjs`,
      so the verification scripts are held by tests the way the primitives are. Confirm the
      existing `src/**/*.test.ts` suite still runs and passes unchanged.

## 2. The ratchet

- [x] 2.1 RED: `scripts/ratchet.test.mjs` — a measured value equal to its baseline passes; a
      value above fails; a value below fails and reports the baseline stale; a run without the
      update flag never writes the baseline file; a run with it writes exactly the measured
      value.
- [x] 2.2 GREEN: `scripts/ratchet.mjs` — read baseline JSON, compare `===`, report per-key
      movement with the direction named, `--update` rewrites. Exit code owned by the caller.
- [x] 2.3 REFACTOR. Tests stay green. (The per-group `simplify` calls are consolidated
      into one pass over the whole diff at 7.5 — six passes over overlapping diffs is waste.)

## 3. Adoption census

- [x] 3.1 RED: `scripts/check-adoption.test.mjs` — the import parser over fixture sources.
      Single-line and multi-line specifier lists, `X as Y` aliases, a file importing two
      primitives counting once for each, a file importing the same primitive twice counting
      once, and a `.ts` file counted alongside `.svelte`.
- [x] 3.2 RED: the door rule — a `.svelte` or `.ts` file importing `freehire-design-system`
      by name fails unconditionally; a CSS `@import` of the package is not read at all.
- [x] 3.3 GREEN: `scripts/check-adoption.mjs` — walk `../web/src`, count files per exported
      primitive, drive `ratchet.mjs`, enforce the door, and print the unused primitives by
      name on every run. Header states it is a repo-boundary check, not a package check.
- [x] 3.4 Run it, commit `scripts/adoption-baseline.json` with the counts **the script
      reports**, not the figures in the proposal.
- [x] 3.5 Wire `pnpm check:adoption` in `design-system/package.json`.
- [x] 3.6 REFACTOR. Tests stay green.

## 4. Token check — the web radius

- [x] 4.1 RED: `scripts/check-token-coverage.test.mjs` — extract the detectors so they can be
      asserted directly. `COLOUR` and `ARBITRARY` keep their current behaviour, including
      arbitrary *variants* (`[&_tr]:border-b`) passing and TypeScript indexing not matching.
- [x] 4.2 RED: the new `PALETTE` detector — matches `text-amber-600`, `bg-emerald-500`,
      `border-red-300`; does not match a semantic token utility (`bg-card`, `text-muted-
      foreground`), an arbitrary value, or a non-colour scale (`p-4`, `gap-2`, `z-50`).
- [x] 4.3 GREEN: teach `check-token-coverage.mjs` the two radii — `design-system/src` hard
      fail on `COLOUR`/`ARBITRARY` as today, `../web/src` ratcheted on all three. One
      definition per detector, shared by both passes.
- [x] 4.4 Run it, commit `scripts/web-token-baseline.json` with the counts the script reports.
- [x] 4.5 Verify the package radius still fails on a planted hex in a primitive, and that the
      `avatar.svelte` exception still applies and is still asserted.
- [x] 4.6 REFACTOR. Tests stay green.

## 5. `dist` sync

- [x] 5.1 Add `"check:dist": "pnpm build && git diff --exit-code -- dist"` to
      `design-system/package.json`.
- [x] 5.2 Verify both directions by hand: passes on a clean tree; fails after editing a value
      in `tokens/*.tokens.json` without committing the rebuild. Restore the token afterwards.

## 6. Tests for the primitives the app depends on

- [x] 6.1 Read `button.svelte`, `input.svelte`, `badge.svelte` first — the scenarios must
      pin the contract each actually offers, not one assumed from its name.
- [x] 6.2 RED→GREEN (verified by mutation: each contract broken in turn, the intended test went red)
      : `src/button.test.ts` — every variant and size resolves to distinct
      classes, `destructive` among them; `disabled` reaches the element; a caller's `class`
      beats a colliding base class through `cn`.
- [x] 6.3 RED→GREEN: `src/input.test.ts` — `type` and the native attributes pass through;
      the invalid state reaches `aria-invalid`; a caller's `class` wins.
- [x] 6.4 RED→GREEN: `src/badge.test.ts` — every variant resolves distinctly, `destructive`
      among them; a caller's `class` wins.
- [x] 6.5 REFACTOR across the three. Tests stay green.

## 7. CI and docs

- [x] 7.1 `.github/workflows/ci.yml` — add `pnpm check:adoption` and `pnpm check:dist` to the
      `design-system` job. No new job, no second install.
- [x] 7.2 `design-system/docs/verification.md` — one row per guarantee: what it asserts, which
      files it reads, and the command. Include the guarantees that already existed.
- [x] 7.3 `design-system/AGENTS.md` — state the two radii and why they differ, the exact-match
      ratchet and its `--update`, the one door and the CSS exemption, and that `dist` is
      rebuilt and diffed. Keep it to what a future agent would get wrong without it.
- [x] 7.5 `simplify` over the whole diff; re-run everything. Tests stay green.
- [x] 7.4 Run the full `design-system` job locally end to end: `pnpm build && pnpm check &&
      pnpm test && pnpm validate:docs && pnpm check:tokens && pnpm check:adoption && pnpm
      check:dist && pnpm build-storybook`. Every one green.

## 8. Finish

- [ ] 8.1 `requesting-code-review` on the whole diff; act on Critical and Important via
      `receiving-code-review`.
- [ ] 8.2 `finishing-a-development-branch` — integrate the branch.
- [ ] 8.3 `/opsx:archive` then `/opsx:sync`.
- [ ] 8.4 Internal-only change — no changelog entry. Confirm that call rather than assuming it.
