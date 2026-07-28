## Why

The mail stack — Gmail sync, the hosted mailbox, LLM classification, application
linking and automatic stage advance — is live, but nothing on the public site
explains it. A visitor who is not signed in cannot discover the feature at all,
and a signed-in user meets `/my/inbox` with no account connected and no
explanation of what connecting one buys them. The five-slide carousel that
announced the feature (`freehire-carousel`) already carries a working narrative;
this change turns that narrative into a durable page on the site.

## What Changes

- A new public page at `/features/inbox` explaining the inbox: how mail gets in,
  what the classifier tags, how a tagged email links to an application and moves
  its card on the tracking board, what we do and do not read, and how an agent
  harness can push its own mail.
- The page follows the established landing vocabulary of `/about` and
  `/contribute`: a route that carries only `<Seo>` plus the page wrapper, all
  markup in a `*LandingView` component, mono `// section` labels, hairline
  (`gap-px` on `bg-border`) grids, numbered `01/02/03` blocks.
- Product previews are built from markup and theme tokens, not screenshots — so
  they hold up in dark mode, stay responsive, carry no PII, and cannot silently
  drift out of date as image files.
- A FAQ block backed by a `FAQPage` JSON-LD payload, sharing one source with the
  visible text (mirroring `homeFaq.ts`).
- Discovery: a footer link, a sentence with a link from the `HomeView`
  "track your search" section (which renders on both `/` and `/about`), and the
  URL added to `sitemap-pages.xml`.
- `/features/*` is introduced as the home for feature landings. The existing
  referrals landing moves there too (`/referrals` → `/features/referrals`, with a
  301 on the old address), so the space has the two pages it describes rather
  than one page and a promise. No `/features` index page until the set is big
  enough to need a hub.
- Two presentational primitives that already repeat verbatim across `HomeView`,
  `AboutValues` and `ContributeLandingView` — the mono section label and the
  numbered hairline grid — are extracted so the planned tracking landing can be
  assembled from them rather than copy-pasted a fourth time.

## Capabilities

### New Capabilities
- `inbox-feature-landing`: the public `/features/inbox` page — its sections,
  the claims it is allowed to make about classification and stage advance, its
  SEO payload, and the links that lead to it.

### Modified Capabilities

None. This adds a marketing surface; no existing requirement changes. The
existing `email-inbox`, `application-from-mail` and `email-body-classification`
specs describe the behaviour the page documents, and stay as they are.

## Impact

- **New:** `web/src/routes/features/inbox/+page.svelte`,
  `web/src/lib/components/InboxLandingView.svelte`, `web/src/lib/inboxFaq.ts`,
  and two extracted primitives under `web/src/lib/components/`.
- **Moved:** `web/src/routes/referrals/+page.svelte` →
  `web/src/routes/features/referrals/+page.svelte`, with a 301 left behind at the
  old path.
- **Modified:** `web/src/lib/components/Footer.svelte` (a new "Features" group,
  four columns instead of three), `web/src/lib/components/HomeView.svelte` (a
  sentence in "track your search" plus a features section),
  `web/src/lib/sitemap.ts` (`STATIC_PATHS`).
- **No backend change.** The page is static marketing copy; it reads no API and
  needs no new endpoint. It does depend on the accuracy of
  `internal/mailclassify` (the status vocabulary and the forward-only stage
  rules) — the copy must match that code, not overstate it.
- **Honesty constraints carried from the code:** only a deterministic match
  (`TierThread`/`TierName`) auto-links an email; a confident model verdict is a
  *suggestion* the user confirms. Stage advance is strictly forward, never out of
  a settled stage, and rejection never moves a card automatically. Gmail sync
  runs under an unverified restricted-scope OAuth app (test users only), so the
  hosted mailbox — which works for everyone — is the primary call to action and
  Gmail is the secondary one.
