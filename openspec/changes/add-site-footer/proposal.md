## Why

The site footer is a thin single-row strip inlined in the root layout: a tagline
plus four links (CLI, API, Telegram, GitHub). It reads as a placeholder, offers no
site navigation, and omits the project's LinkedIn presence
(`linkedin.com/company/freehire-dev/`). A proper footer improves wayfinding and
makes freehire look professionally maintained.

## What Changes

- Extract the inline footer out of `web/src/routes/+layout.svelte` into a dedicated
  `Footer.svelte` component.
- Give it a restrained multi-column layout: a brand block (name + tagline + social
  icons) and navigation columns grouping existing routes (Product, Resources,
  Company).
- Add the **LinkedIn** social link (`https://linkedin.com/company/freehire-dev/`)
  alongside the existing GitHub and Telegram links, reusing `ProviderIcon`.
- Add a bottom bar: copyright line + open-source note.
- Keep it clean and uncluttered, responsive (columns stack on mobile), and faithful
  to the flat-neutral design system (oklch grayscale, thin borders, system fonts —
  no new fonts or colors beyond existing brand marks).

## Capabilities

### New Capabilities
- `site-footer`: the global site footer — its layout, the link groups it exposes,
  the social links it carries, and its responsive/theming behavior.

### Modified Capabilities
<!-- None: the footer today has no spec of its own; header-navigation and
     web-frontend are unaffected. -->

## Impact

- `web/src/routes/+layout.svelte` — remove the inline `<footer>` markup, render
  `<Footer />` instead.
- `web/src/lib/components/Footer.svelte` — new component.
- Reuses `web/src/lib/components/ProviderIcon.svelte` (already supports
  github/linkedin/telegram); no icon changes needed.
- No backend, API, or DB impact. No new dependencies. Verified via `svelte-check` +
  lint + visual check (the web app has no test runner).
