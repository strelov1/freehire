## Why

`web/src/lib/components` has grown several components with no domain coupling that are each
reused across three or more unrelated pages — a company/logo tile, a mono section heading, an
OAuth brand-icon set, a country flag, a hairline numbered grid, a label/control settings row,
and a horizontally-scrolling underline tab strip. None of them live in `freehire-design-system`
or have a Storybook story, so each one was built once inside `web/` and is invisible to the
`check:adoption` ratchet, to the primitive-contract tests the verification suite requires, and
to anyone browsing the component catalogue in Storybook to see what already exists before
building something new.

## What Changes

- Move the following components' presentation into `design-system/src` as new primitives,
  each exported from `src/index.ts` and given a `*.stories.ts` under `Primitives/<Name>`
  covering every variant currently in use across `web/src`, verified in both the light and
  dark Storybook toolbar themes:
  - `SectionLabel` — mono `// label` heading, no domain coupling, moves as-is.
  - `ProviderIcon` — inlined OAuth/brand SVG marks, no domain coupling, moves as-is.
  - `NumberedGrid` — hairline numbered grid layout, no domain coupling, moves as-is.
  - `SettingRow` — label/control settings row layout, no domain coupling, moves as-is.
  - `TabRow` — a horizontally-scrolling, fade-masked, roving-tabindex underline tab strip.
    Distinct from the existing `Tabs` primitive (a pill/segmented control with no overflow
    handling) rather than a variant of it — ships as a second, separate tabs primitive.
  - `CompanyLogo` — its rendering (image with an SSR-safe broken-image fallback to an
    initial-letter monogram, and a final icon fallback when there is no name at all) is
    generic; its data source (`companyLogoUrl`, the logo.freehire.me proxy lookup) is not
    and stays in `web/`. **Design decision, not pre-made here:** whether the presentational
    half becomes a new primitive or an extension of the existing `Avatar` primitive (which
    already renders `name`/`src`/`size` but has no broken-image fallback and is always
    circular) — see design.md.
  - `CountryFlag` — the flag-icons-backed rendering is generic; its label lookup
    (`countryLabel` from `$lib/facets`) is app vocabulary and moves to a required prop
    instead. **Design decision, not pre-made here:** whether `design-system` takes on the
    `flag-icons` dependency itself or the primitive accepts a pre-resolved class/glyph — see
    design.md.
  - `LoadMore` — a two-line composition over the existing `Button` primitive (loading label,
    optional error line). **Design decision, not pre-made here:** whether this becomes its
    own primitive or a documented `Button` usage pattern with no new component — see
    design.md.
- Update every `web/src` call site of a promoted component to import it from `$lib/ui`
  instead of its old `lib/components/**` path, then delete the now-dead local file.
- Run `check:adoption --update` (and any other ratchet this change moves) once, as a single
  reviewable diff to the baseline files.
- **Explicitly excluded:** `StatusChip` was in the original reuse survey (3 call sites) but is
  domain-coupled to `$lib/emailStatus` (mail classification vocabulary) underneath a thin
  `Badge` call — it already goes through the single door and gains nothing from promotion. It
  stays in `web/`.

No **BREAKING** changes — this only relocates and re-exports existing presentation; no prop
contract loses a capability a call site currently depends on (any prop rename is a mechanical
rewrite of call sites in the same change, not a compatibility break for anyone else, since
`design-system` has no external consumers besides `web/`).

## Capabilities

### New Capabilities
- `design-system-primitives`: the contract every promoted or newly-added reusable UI
  component in `freehire-design-system` must meet — exported from `src/index.ts`, reachable
  only through `web`'s single door (`$lib/ui`), given a Storybook story per primitive with
  variant coverage matching real call-site usage, and rendering correctly under both the
  light and dark Storybook theme toggle.

### Modified Capabilities
(none — `design-system-verification`'s existing requirements, e.g. the adoption ratchet and
the single-door check, are unchanged in behavior; this change only moves the baseline numbers
they measure, which is a data update covered in tasks.md, not a requirement change)

## Impact

- `design-system/src/*.svelte`, `design-system/src/index.ts`, `design-system/src/*.stories.ts`
  — new primitive implementations and stories.
- `design-system/scripts/adoption-baseline.json` (and `web-token-baseline.json` if any
  relocated class moves a literal/utility across the ratchet's boundary) — baseline rewrite.
- `web/src/lib/components/{SectionLabel,ProviderIcon,NumberedGrid,SettingRow,TabRow,
  CompanyLogo,CountryFlag,LoadMore}.svelte` — deleted after call sites move to `$lib/ui`.
- Every `web/src` file currently importing one of these from `lib/components/**` (37 files
  across the reuse survey) — import path updated.
- No API, schema, or backend impact.
