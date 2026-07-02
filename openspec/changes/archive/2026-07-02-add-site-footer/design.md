## Context

The footer today is inline `<footer>` markup in `web/src/routes/+layout.svelte`
(a single row: tagline + CLI/API/Telegram/GitHub). The layout already wraps the app
in a `flex min-h-svh flex-col` column, so a bottom-pinned footer works by construction.
The design system (`web/src/app.css`) is a deliberate flat-neutral palette: oklch
grayscale, one background fill, separation via spacing + thin borders, system fonts.
`ProviderIcon.svelte` already renders github/linkedin/telegram brand marks. The web
app has **no test runner** (memory: `hire web no test runner`) — verification is
`svelte-check` + lint + visual.

## Goals / Non-Goals

**Goals:**
- A dedicated, reusable `Footer.svelte` with a restrained multi-column layout.
- Add the LinkedIn social link; keep GitHub + Telegram.
- Faithful to the flat-neutral aesthetic; responsive (stacks on mobile); theme-safe.

**Non-Goals:**
- No new fonts, colors, or dependencies.
- No newsletter form, language switcher, or extra pages beyond existing routes.
- No changes to `ProviderIcon`, header, or any backend/API.

## Decisions

- **Extract into `$lib/components/Footer.svelte`, mount once from the layout.**
  Mirrors the existing `TopBar` component pattern (layout renders `<TopBar />`).
  Alternative — keep it inline — rejected: a multi-column footer is too much markup
  to leave in the layout and isn't reusable/testable in isolation.
- **Link groups map to existing routes only** (Product / Resources / Company). Chosen
  over inventing new destinations so every link resolves today; `resolve()` from
  `$app/paths` keeps internal links correct under base-path config, matching the
  current footer's usage.
- **Styling with existing Tailwind semantic tokens** (`border`, `muted-foreground`,
  `foreground`) and a responsive grid (`grid-cols-2 md:grid-cols-4` or similar) that
  stacks on mobile. No custom CSS beyond what tokens provide. Chosen over a bespoke
  palette to honor `surgical changes` / `match existing style`.
- **Social links reuse `ProviderIcon`** with `target="_blank" rel="noopener noreferrer"`,
  exactly as the current external links do.

## Risks / Trade-offs

- [No web test runner → scenarios can't be unit-tested] → verify via `svelte-check`
  (types/a11y), lint, and a visual check in both themes and at a mobile width.
- [Route drift: a linked route is renamed/removed later] → links use existing routes
  observed in `src/routes`; `resolve()` surfaces bad paths at build/type time.
- [Over-cluttering the footer] → constrain to the three named groups + brand + bottom
  bar; the spec forbids links beyond those listed.

## Migration Plan

Pure frontend swap: replace the inline `<footer>` block in `+layout.svelte` with
`<Footer />`. No data migration, no feature flag. Rollback = revert the commit.
