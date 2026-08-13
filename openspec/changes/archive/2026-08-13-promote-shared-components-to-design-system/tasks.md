## 1. SectionLabel, ProviderIcon, NumberedGrid, SettingRow (no domain coupling, move as-is)

- [x] 1.1 Add `section-label.svelte`, `provider-icon.svelte`, `numbered-grid.svelte`,
      `setting-row.svelte` to `design-system/src`, adapted from their `web/src/lib/components`
      originals (and `web/src/lib/components/cv/SettingRow.svelte` for the last one), export
      each from `design-system/src/index.ts`.
- [x] 1.2 Add `*.stories.ts` for each under `Primitives/<Name>`, with a story per variant seen
      at its current call sites (all provider icons for `ProviderIcon`; a populated and an
      overflowing `NumberedGrid`; a plain and a `grow` `SettingRow`).
- [x] 1.3 Verify all four render correctly with the Storybook toolbar set to `dark`; fix any
      literal color that doesn't adapt.
- [x] 1.4 Update every `web/src` call site of these four to import from `$lib/ui`; delete
      `web/src/lib/components/{SectionLabel,ProviderIcon,NumberedGrid}.svelte` and
      `web/src/lib/components/cv/SettingRow.svelte`.
- [x] 1.5 `pnpm check:adoption` (no `--update`) in `design-system/` and confirm it reports
      exactly the expected new counts for these four primitives.

## 2. TabRow (second tabs primitive, distinct from Tabs)

- [x] 2.1 Add `tab-strip.svelte` to `design-system/src` (name distinct from the existing
      `Tabs` export — see design.md), carrying over the scroll/fade-mask behavior, roving
      tabindex, and the exported `tabId` helper (shipped as `tabStripId`, to avoid a bare,
      collision-prone export name); export it and the helper from `design-system/src/index.ts`.
- [x] 2.2 Add `tab-strip.stories.ts` under `Primitives/TabStrip`: a story with tabs that fit
      the strip and one with enough tabs to force horizontal scroll (to exercise the fade
      mask). No Storybook interaction-test precedent exists in this package, so keyboard-nav
      and ARIA behavior are covered instead by a vitest contract test (`tab-strip.test.ts`),
      matching how `tabs.svelte` is tested.
- [x] 2.3 Verify both stories render correctly with the Storybook toolbar set to `dark`.
- [x] 2.4 Update every `web/src` call site importing `TabRow` (and the `tabId` helper) to use
      `$lib/ui`; delete `web/src/lib/components/TabRow.svelte`.
- [x] 2.5 `pnpm check:adoption` (no `--update`) and confirm the expected delta.

## 3. LoadMore

- [x] 3.1 Add `load-more.svelte` to `design-system/src` (a thin `Button` composition with
      `loading`/`error` states), export from `design-system/src/index.ts`.
- [x] 3.2 Add `load-more.stories.ts` under `Primitives/LoadMore`: idle, loading, and error
      states.
- [x] 3.3 Verify dark-mode rendering, in particular the `text-destructive` error line.
- [x] 3.4 Update every `web/src` call site to import from `$lib/ui`; delete
      `web/src/lib/components/LoadMore.svelte`. (The reuse survey undercounted this one —
      `CompanyFeedbackListDialog.svelte` was a ninth real call site, excluded from the original
      grep because it matched the `*Dialog.svelte` filter used to scope the survey; included
      here since it is a genuine consumer.)
- [x] 3.5 `pnpm check:adoption` (no `--update`) and confirm the expected delta.

## 4. CountryFlag

- [x] 4.1 Add `country-flag.svelte` to `design-system/src`, taking `label: string` as a
      required prop instead of resolving it internally (see design.md); keep the two-ASCII-
      letter validity check and the plain-text fallback for an unrenderable code. Export from
      `design-system/src/index.ts`.
- [x] 4.2 Add `country-flag.stories.ts` under `Primitives/CountryFlag`, importing the
      `flag-icons` stylesheet directly in the story file (not `preview.css` — see design.md);
      cover a renderable code and an unrenderable one (falls back to the text code). Added
      `flag-icons` as a `design-system` devDependency, scoped to Storybook only.
- [x] 4.3 Verify dark-mode rendering, in particular the ring contrast against a light flag
      (e.g. Japan, Nigeria) called out in the original component's comment.
- [x] 4.4 Update every `web/src` call site to import from `$lib/ui` and pass
      `label={countryLabel(code)}` explicitly; delete
      `web/src/lib/components/CountryFlag.svelte`.
- [x] 4.5 `pnpm check:adoption` (no `--update`) and confirm the expected delta.

## 5. CompanyLogo → Avatar extension

- [x] 5.1 Add `shape?: 'circle' | 'square'` to `avatar.svelte` (default `'circle'`); apply it
      to both the image and initials-render branches' corner-radius class.
- [x] 5.2 Add `CompanyLogo`'s broken-image recovery to `Avatar`'s `src`-present branch: the
      `onerror` handler and the `{@attach}`-based `catchMissedError` SSR-race check, falling
      back to the initials render on failure. Preserve the source comment explaining the SSR
      race.
- [x] 5.3 Add an optional `fallbackIcon?: Snippet` prop, rendered only when neither `src` nor
      `name` is present; `Avatar`'s existing bare `'?'` stays the default when the prop is
      omitted.
- [x] 5.4 Extend `avatar.stories.ts` with: `shape="square"` (moved to a dedicated
      `entity-logo.stories.ts`, see below), a broken `src` recovering to initials, and a
      `fallbackIcon` example (`avatar-fallback-icon.stories.ts`, mirroring `CompanyLogo`'s
      `Globe` fallback).
- [x] 5.5 Extend `Avatar`'s contract test (per `design-system-verification`'s requirement that
      every primitive with consumers carries variant-pinning tests) to cover the new `shape`
      variant and the broken-image-fallback branch.
- [x] 5.6 Verify dark-mode rendering for all new states.
- [x] 5.7 Update every `web/src` call site of `CompanyLogo` to call the design system directly
      (`name`, `src={companyLogoUrl(name)}`, `shape="square"`); leave `companyLogoUrl` in
      `web/src/lib/logo.ts` unchanged. Delete `web/src/lib/components/CompanyLogo.svelte`.

      **Deviations from the plan, decided during implementation:**
      - **Exported as `EntityLogo`, a second name for `avatar.svelte`** (not literally
        `Avatar`) — user feedback mid-implementation: "Avatar" reads as a person and is a
        confusing name for a company logo tile. Same component, added as
        `export { default as EntityLogo } from './avatar.svelte'` alongside `Avatar`, with its
        own `Primitives/EntityLogo` story and `docs/dsds/components.json` entity. Zero
        duplicated logic; two names for the two call-site intents.
      - **`size` needed a 4th tier.** `CompanyLogo`'s `size` was a raw Tailwind class
        (`size-4`…`size-11`, 7 distinct values across 19 call sites), not compatible with
        `Avatar`'s closed 3-value enum. Added `xs` (`size-5`, `text-[9px]`, one more `ALLOWED`
        exception in `check-token-coverage.mjs`) and mapped each call site to its nearest
        tier (exact matches where `size-8`/`size-10`/`size-12` already equalled `sm`/`md`/`lg`;
        within ±14% elsewhere). Documented in `design-system-primitives`... no spec change
        needed — this is an implementation-level primitive-contract detail, not new
        externally-observable behavior beyond what the proposal already scoped.
      - **`fallbackIcon` wiring skipped at call sites — revised after code review.** The
        original claim here ("`name` is non-empty at every real call site") was wrong: code
        review found `job.company`/`current.company` (the `Job` type) genuinely can be empty —
        `JobRow.svelte`, `JobView.svelte`, `JobDrawer.svelte` and `SwipeDeck.svelte` already
        defend their own adjacent *display* text with `|| 'Unknown company'`, but the `name`
        prop handed to the logo was passed unguarded. Fixed at all 8 call sites sourced from
        `Job`/`MyAnalysisItem` (`JobRow`, `JobView`, `ArtifactPanel`, `JdIntakeDialog`,
        `HeaderSearch`, `AnalysesView`, `SwipeDeck`, `JobDrawer`) by applying the same
        `|| 'Unknown company'` each site's own visible text already uses (or, for `JobDrawer`/
        `SwipeDeck`, already used only for the text). The remaining call sites source `name`
        from `Company.name` or an already-`||`-guarded value (`company_name || company_slug`,
        `company_slug` fallback baked into a `company()`/`company` helper) — no known-empty
        path, left as-is. `fallbackIcon` itself is still unwired anywhere (now genuinely
        unreachable rather than incorrectly assumed unreachable), and remains available,
        tested and storied for a future caller.
      - **`alt` text and `loading` — also from code review.** `avatar.svelte`'s image branch
        pre-dates this change and always set `alt={name ?? ''}` (a person avatar may be the
        sole identification) and no `loading` attribute. Folding `CompanyLogo` in inherited
        both without adjusting either for the new `shape="square"` case, which regressed two
        things `CompanyLogo` got right: `loading="lazy"` (list-heavy pages now fetched every
        logo eagerly) and `alt=""` (a square logo sits beside the name as visible text at
        every real call site, so naming the image too is a double announcement to a screen
        reader). Fixed: `loading="lazy"` added unconditionally; `alt` is `''` when
        `shape === 'square'` and `name` otherwise, matching each shape's original component's
        own choice. Covered by two new `avatar.test.ts` cases.
- [x] 5.8 `pnpm check:adoption` (no `--update`) and confirm the expected delta, including
      `EntityLogo`'s count rising by `CompanyLogo`'s former call sites (16), and `Button`'s
      dropping by 1 (the deleted `LoadMore.svelte` was itself a `$lib/ui` `Button` consumer;
      its composition now lives inside `design-system/src/load-more.svelte`).

## 6. Ratchet baselines and review

- [x] 6.1 Run `pnpm check:adoption -- --update` in `design-system/` once, producing a single
      diff to `adoption-baseline.json` covering all seven promotions (six new primitives plus
      `EntityLogo`) and the `Button` regression; reviewed — every name and count matches what
      the per-group `check:adoption` runs already reported.
- [x] 6.2 Run `pnpm check:tokens -- --update`: `web-token-baseline.json` dropped exactly the
      four deleted files' entries (`CompanyLogo.svelte`, `ProviderIcon.svelte`,
      `SectionLabel.svelte`, `cv/SettingRow.svelte`), 463 → 455 violations. No new-file entries
      — every relocated primitive is clean in `design-system/src` (two more deliberate
      `ALLOWED` exceptions: Google's literal brand colours, and the `xs` avatar tile's
      `text-[9px]`).
- [x] 6.3 Confirmed: only `web/src/lib/ui/index.ts` (the door itself) names
      `freehire-design-system`; zero references anywhere to the eight deleted
      `lib/components/**` paths.
- [x] 6.4 `pnpm -C design-system check/test/lint/check:tokens/check:adoption/check:dist/
      validate:docs/build-storybook` and `pnpm -C web check/test/lint/build` all pass. (No Go
      changed in this repo-web-only change, so `go build`/`go vet` were not run — nothing in
      `internal/` or `cmd/` was touched.) Also updated `docs/dsds/components.json` (23 entities,
      was 15) so `validate:docs` — which fails on any undocumented story file — passes; this
      was not in the original task list, surfaced by running the check.
- [x] 6.5 `/code-review high` on the full diff, once all groups were done. Found and fixed 4
      real issues: the `name`-non-empty assumption behind skipping `fallbackIcon` wiring was
      wrong for 8 `Job`/`MyAnalysisItem`-sourced call sites (see 5.7's revised note); the
      migrated `<img>` lost `CompanyLogo`'s `loading="lazy"` and `alt=""`, causing eager
      list-page image loads and duplicate screen-reader announcements; and three new
      `check-token-coverage.mjs` `ALLOWED` exceptions left `design-system/AGENTS.md`'s "one
      allowed exception" line stale — updated. All four re-verified with the same full
      check/test suite as 6.4 after fixing.
