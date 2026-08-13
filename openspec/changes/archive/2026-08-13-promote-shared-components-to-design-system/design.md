## Context

See proposal.md - Why/What Changes for the full component list and the reuse counts that
motivated it. This document resolves the three decisions the proposal left open and lays out
the migration order for all seven promotions plus the one exclusion (`StatusChip`).

Two existing mechanisms constrain every decision here:
- `internal` — n/a, this is `web`/`design-system` only.
- `openspec/specs/design-system-verification/spec.md` already requires (1) app code reach
  primitives only through `web/src/lib/ui` (the single door), (2) every primitive with
  consumers under `web/src` carry contract tests pinning its variant→class mapping, and (3)
  an adoption-count ratchet that fails on ANY change from its committed baseline unless rerun
  with `--update`. This change moves that baseline; it does not change the requirement.

## Goals / Non-Goals

**Goals:**
- Resolve, with rationale, whether `CompanyLogo`, `CountryFlag`, and `LoadMore` become new
  primitives, variants of existing ones, or stay as thin app-side wrappers.
- Sequence the seven promotions so the adoption ratchet and contract-test requirement are
  satisfied by construction rather than patched after the fact.

**Non-Goals:**
- Redesigning `Tabs`, `Avatar`, or `Button`'s existing variants beyond what a decision below
  requires.
- Auditing the rest of `web/src/lib/components` for further promotion candidates — the
  proposal's reuse survey is the fixed input to this change.

## Decisions

### CompanyLogo extends Avatar; it does not become a second primitive

`Avatar` already owns "image with a name-derived initials fallback." `CompanyLogo` differs in
exactly two ways: it's a rounded square instead of a circle, and its image branch recovers
from a broken/missing `src` by falling back to the initials render — a case `Avatar`'s current
`{#if src}` branch has no handling for at all (a broken `src` today just shows the browser's
broken-image icon). That gap is a strict improvement to fold into `Avatar` rather than a
reason to fork it.

`Avatar` gains:
- `shape?: 'circle' | 'square'` (default `'circle'`, so every existing call site is
  unaffected).
- The `src`-present branch adopts `CompanyLogo`'s two-part broken-image recovery: an `onerror`
  handler for a fetch that fails after hydration, and the `{@attach}`-based `catchMissedError`
  check for the SSR race where the image already failed before hydration's `onerror` listener
  attached. Keep the source comment explaining the race — it is the non-obvious reason both
  checks exist.
- An optional `fallbackIcon?: Snippet` slot, rendered only when neither `src` nor `name` is
  present (today `Avatar` renders `'?'` in that case; `CompanyLogo` renders a `Globe` icon).
  Passing nothing preserves `Avatar`'s current `'?'` behavior.

`companyLogoUrl` (the logo.freehire.me proxy lookup) is app data-fetching and stays in
`web/src/lib/logo.ts`; only `CompanyLogo.svelte`'s render logic moves, and the file itself is
deleted once call sites pass `src={companyLogoUrl(name)}` to `Avatar` directly.

**Alternative considered:** a separate `LogoTile` primitive. Rejected — it would duplicate the
hue-hash, initials-derivation, and image-branch logic that already lives in `Avatar` for a
shape-only delta, and would leave two near-identical primitives for a future call site to
choose between inconsistently, which is exactly the drift this change exists to close.

### CountryFlag takes `label` as a prop; `flag-icons` stays a `web/`-only dependency

The primitive keeps the `fi fis fi-{cc}` rendering and the two-ASCII-letter validity check,
but takes `label: string` from the caller instead of resolving it via `$lib/facets`'
`countryLabel` internally — the same treatment any other app-vocabulary lookup gets at this
boundary.

`flag-icons` is not added as a `design-system` dependency. The package's only consumer today
is `web/`, which already loads the sheet once in its root layout; adding it to
`design-system` would give every primitive's Storybook a second global stylesheet dependency
(alongside the token contract in `preview.css`) for one primitive's sake, and would blur the
boundary the package exists to keep — generic visual primitives, not bundled third-party
icon-font integrations. The `CountryFlag` story imports the sheet directly in its own
`*.stories.ts` (the same pattern `button-icon.stories.ts` already uses for a story-local
dependency), so Storybook renders real flags without the package as a whole taking on the
asset.

**Alternative considered:** bundle `flag-icons` into `design-system`. Rejected as the wrong
layering per above.

### LoadMore is promoted as its own primitive, not documented as a Button pattern

It is two lines of markup, but it encodes a rule — the error line's presence, wording, and
spacing — that today has three independent copies. Leaving it as a documented "pattern"
(copy this snippet) instead of a component reintroduces the exact drift promotion is meant to
prevent: nothing stops a fourth call site from copying the pattern with a variation. A real
primitive is enforced by the adoption ratchet and is one place to change the copy or spacing
later.

**Alternative considered:** a Storybook docs-only pattern page, no component. Rejected — not
exported from `index.ts`, not tracked by `check:adoption`, so it constrains nothing.

### Migration order

One promotion at a time, each carried through fully (component → story → dark-mode check →
call-site rewrite → dead file deleted) before starting the next, so a partial promotion is
never left in a mixed state:

1. `SectionLabel`, `ProviderIcon`, `NumberedGrid`, `SettingRow` — no domain coupling, no open
   decision, lowest risk; establishes the pattern for the rest.
2. `TabRow` — largest single move (156 lines); ships as a second tabs primitive alongside the
   existing `Tabs`, not a variant of it (see proposal.md — the two solve different layouts:
   pill/segmented vs. scrolling underline strip).
3. `LoadMore` — depends on nothing else in this list.
4. `CountryFlag` — the `label` prop change touches every call site, not just the import path.
5. `CompanyLogo` → `Avatar` extension — touches `Avatar`'s existing contract test, so it goes
   last, after the ratchet pattern from the earlier promotions is already proven out.

`check:adoption --update` runs once, after step 5, producing a single reviewable baseline
diff rather than one per promotion.

`StatusChip` is not part of this migration — see proposal.md's Excluded note.

## Risks / Trade-offs

- **Avatar's contract test must grow a `shape` case and a broken-image-fallback case**, or
  `design-system-verification`'s "primitives the app depends on carry tests" requirement is
  violated the moment `Avatar` gains a consumer that exercises the new branch → covered
  explicitly in tasks.md as part of the `Avatar` step, not left implicit.
- **A single end-of-migration `check:adoption --update` obscures which promotion moved which
  count** if something goes wrong → each promotion's task still runs `check:adoption` (without
  `--update`) to see its own delta reported before the final update, so a mistake surfaces at
  the step that caused it.
- **The `flag-icons` sheet import lives in the story file, not `preview.css`** — if a future
  primitive also needs a story-local stylesheet, this is the precedent to follow rather than
  growing `preview.css` into a dumping ground for one-off assets.
