## Context

The site already has three feature landings — `/contribute`, `/cli`, `/chatgpt` —
plus `/about`, and they share one unwritten template: a route file that carries
only `<Seo>` and a `max-w-6xl px-4 py-6` wrapper, with the whole page living in a
`*View` component. Their visual vocabulary is equally settled: a mono
`// section` label above each block, hairline grids built as `gap-px` on a
`bg-border` background, `01/02/03` numbering, and `figure` elements framed like a
browser or terminal window.

The inbox feature this page describes is implemented across six packages
(`docs/agents/mail-stack.md`). Two of its rules constrain the copy directly:
only a deterministic tier auto-links an email, and stage advance is strictly
forward. A landing page that flattens those into "we sort your mail for you"
would be describing a product we did not build.

A tracking landing (`/features/tracking`) is planned next and will reuse this
page's shape.

## Goals / Non-Goals

**Goals:**

- One public page that a stranger can read and understand the inbox from.
- Copy that survives a reading by someone with the source open.
- Previews that stay correct across light/dark, viewport widths, and future
  redesigns of the real UI.
- Leave the next feature landing cheaper to build than this one was.

**Non-Goals:**

- No `/features` index page. One child does not justify a hub.
- No live data. The page renders no API call; the previews are illustrative,
  exactly as `HomeView`'s board and funnel previews already are.
- No generalized "feature landing" framework. Two shared primitives, not a
  page-builder.
- No new backend surface, no changes to the mail stack.

## Decisions

**Markup previews over screenshots.** The carousel's PNGs are light-theme, carry
a real address (`strelov1@gmail.com`), and freeze a UI that changes. Rebuilding
the three previews — inbox list, connect settings, application card with an
Emails tab — from divs and theme tokens costs more markup but yields a page that
is theme-correct, responsive, PII-free and weightless. `HomeView` already proves
the approach with its board and funnel previews.

**Two primitives extracted, not a framework.** `SectionLabel` (the
`// section` mono label) and `NumberedGrid` (the `01/02/03` hairline grid) each
appear verbatim in three existing components. Extracting them is deduplication of
what exists; anything beyond that would be inventing an abstraction for one
future page. `HomeView`, `AboutValues` and `ContributeLandingView` are migrated
onto them so there is a single definition, not a fourth copy.

*Alternative considered:* a `FeatureLandingLayout` component taking sections as
props. Rejected — every landing so far has a different section order and mix, so
the layout would be a parameter bag with no behaviour.

**FAQ content in a module, mirroring `homeFaq.ts`.** Google requires the visible
answers to match the `FAQPage` payload. A shared `inboxFaq.ts` makes drift
impossible by construction and is directly unit-testable, unlike markup.

**Hosted mailbox is the primary CTA.** Gmail sync runs under an unverified
restricted-scope OAuth app, so it works for test users only until Google's
review completes. Leading with "Connect Gmail" would send most visitors into a
consent screen that rejects them. The hosted address works for everyone, so it
leads; Gmail is presented second.

**The status section lists the vocabulary, not a marketing subset.** All nine
`mailclassify` signals, including `incomplete_application` (an actionable to-do,
deliberately not a stage) and the `other` fallback. Showing the real vocabulary
is also the honest way to explain why nothing gets silently mislabelled: an
out-of-vocabulary model answer is sanitized to `other` before it is stored.

**Testing follows what the repo can test.** `web/` runs vitest over `.ts`
modules; there is no component test runner. So the testable units are
`inboxFaq.ts` (shape, non-empty, unique questions) and `STATIC_PATHS` (the
sitemap entry). Everything visual is verified by build + lint + a headless-Chrome
screenshot pass, which is how the other landings were verified.

## Risks / Trade-offs

- **Markup previews drift from the real UI** → They are illustrative by
  construction, like the existing home-page previews, and are labelled as
  product views rather than live data. A redesign makes them dated, not wrong.
- **Copy drifts from `mailclassify` if the vocabulary changes** → The spec pins
  the claim to the code, and the FAQ/status list is small and localized. A
  vocabulary change is already a change that touches specs.
- **Extracting primitives touches three shipped components** → Pure
  presentational extraction with identical output; verified by build and a
  visual pass on `/`, `/about` and `/contribute`.
- **Gmail positioning could look like under-selling** → It reflects the actual
  access state. When Google verification lands, promoting Gmail is a one-line
  copy change.

## Migration Plan

Additive, frontend-only. Deploy is the normal web build; rollback is reverting
the commit. No migration, no config, no backend release ordering.

## Open Questions

None.
