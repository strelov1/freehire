## 1. Shared primitives

- [x] 1.1 Add `SectionLabel.svelte` (the `// section` mono label) and migrate `HomeView`, `AboutValues` and `ContributeLandingView` onto it
- [x] 1.2 Add `NumberedGrid.svelte` (the `01/02/03` hairline grid) and migrate the `sourced`/`steps` blocks in `HomeView` onto it

## 2. FAQ content module

- [x] 2.1 Write `web/src/lib/inboxFaq.test.ts` covering: the export is non-empty, every question is unique, every answer is non-empty (RED)
- [x] 2.2 Add `web/src/lib/inboxFaq.ts` exporting `INBOX_FAQ: FaqItem[]` with answer-first entries (GREEN)

## 3. The landing page

- [x] 3.1 Add `InboxLandingView.svelte` — hero with the inbox-list preview
- [x] 3.2 Add the connect section: hosted address first, Gmail second
- [x] 3.3 Add the status-vocabulary section, driven by a tested `inboxStatusGuide.ts` pinned to `emailStatus.ts` (RED→GREEN)
- [x] 3.4 Add the board section: deterministic auto-link vs suggestion, forward-only stage ladder, application-card preview with an Emails tab, link to `/my/tracking`
- [x] 3.5 Add the privacy section and the agent-harness section (`POST /me/emails`, never classified by freehire)
- [x] 3.6 Add the FAQ section rendering `INBOX_FAQ`, and the closing call to action
- [x] 3.7 Add `web/src/routes/features/inbox/+page.svelte` — `<Seo>`, canonical, `faqPageJsonLd(INBOX_FAQ)`, page wrapper

## 4. Move the referrals landing under /features

- [x] 4.1 `git mv` the referrals page to `web/src/routes/features/referrals/`, updating its canonical
- [x] 4.2 Leave a 301 at `/referrals` so shared links and ranking survive

## 5. Discovery

- [x] 5.1 Extend `web/src/lib/sitemap.test.ts` for both feature landings and the dropped `/referrals` (RED), then update `STATIC_PATHS` (GREEN)
- [x] 5.2 Add a "Features" group to the footer (Inbox, Referrals) and widen the grid to four columns
- [x] 5.3 Link both landings from `HomeView` — a sentence in "track your search" plus a features section

## 6. Verification

- [x] 6.1 `pnpm test`, `pnpm lint`, `pnpm build` all green
- [x] 6.2 Visual pass in headless Chrome on `/features/inbox` (light + dark) and a regression check of `/`, `/about`, `/contribute` after the primitive extraction
- [x] 6.3 `simplify` pass over the diff, tests still green
