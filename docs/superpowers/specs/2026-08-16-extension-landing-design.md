# `/features/extension` — the browser-extension landing

Date: 2026-08-16

## What this is

A marketing landing for the freehire Chrome extension — the side-panel
job-application agent — assembled the same way as the four landings already
under `/features/*`.

The copy is adapted from the Chrome Web Store carousel
(`~/Projects/freehire-carousel/extension.html`), but every claim is checked
against `extension/` rather than transcribed: the carousel predates several
behaviours and states a couple of others loosely.

## Placement

`/features/extension`, following the recipe the other four landings share:

- `web/src/routes/features/extension/+page.svelte` carries only `<Seo>`, the
  JSON-LD block, and the `mx-auto w-full max-w-6xl px-4 py-6` wrapper.
- `web/src/lib/components/ExtensionLandingView.svelte` holds all the markup.
- `web/src/lib/extensionFaq.ts` is the single array behind both the visible FAQ
  and the `FAQPage` payload.

`/extension/*` is not the home for this: that prefix already belongs to the
extension's OAuth handshake (`/extension/connect`), which is machinery, not a
page anyone browses to.

## Install target

`https://chromewebstore.google.com/detail/freehire/ijfaechijopdlikalojadpojmpilplnj`

It lives in `web/src/lib/extensionLinks.ts` as `EXTENSION_STORE_URL`, shared by
the visible buttons and the `SoftwareApplication` JSON-LD's `installUrl` — the
same reason `cliLinks.ts` exists, so structured data cannot name a destination
the page does not offer.

## Previews are markup

No PNG from the carousel ships. Those captures are light-theme only, carry a
real employer's address bar, and freeze a panel that changes every release. Each
preview on this page is drawn with the same Tailwind tokens as the rest of the
app, so it follows the theme and ages with the design system rather than against
it. This repeats the constraint the earlier landings already hold.

## Sections

1. **Hero** — "Apply where you already are", the three-claim pill row (reads the
   page / scores it against your CV / fills the form), `Add to Chrome` primary
   and a secondary into the jobs list. Alongside it, a drawn browser window with
   the panel docked right, address bar showing a neutral `careers.example.com`.
2. **Match** — "Know if you fit before you apply". A drawn match card: coverage
   percent, the skills covered, the ones named as missing. The honest detail the
   carousel omits: the card also works on a posting that is not in the freehire
   catalogue at all, and in that case the actions bound to a catalogue job
   (Analyse, Save) are simply absent rather than broken.
3. **Autofill** — "It reads the form and fills it in", as a `NumberedGrid` of
   four steps drawn from `extension/AGENTS.md`:
   - it maps the form's real fields, custom dropdowns included, into a checklist
     that counts the required questions;
   - it answers from your freehire profile;
   - **it walks the form one question at a time**, scrolling to each and
     outlining it as the value lands — the walk is the audit, which is the part
     the carousel never says;
   - you read it and press Submit yourself.
   Plus the guard: the filler only engages on a page that looks like a real
   application (a CV upload), so a job-alert signup is never written into.
4. **Chat** — "Ask about the page in front of you", with a drawn conversation
   showing the `read_current_page` tool line. Two facts stated plainly: the
   agent decides to read because the question needed it, and each read is named
   in the transcript with query and fragment stripped, because that is where
   session tokens live.
5. **Where it works** — ATS chips (Greenhouse, Lever, Workday, Ashby, iCIMS,
   SmartRecruiters, Recruitee) framed explicitly as examples, not an allowlist:
   the form reader works off the live DOM, so a career page nobody has heard of
   is the same case.
6. **Bounds** — three cards: the relay socket exists only while the panel is
   open; `read_page` refuses any tab that is not `http(s)`, decided from the URL
   before the page is touched; a conversation is readable and deletable from
   your account, and deleting it starts a fresh one.
7. **Getting started** — install, sign in with freehire, open the panel on a
   posting.
8. **FAQ** — from `EXTENSION_FAQ`.
9. **Closing CTA** — `Add to Chrome` again, plus the jobs list.

## Structured data

- `extensionApplicationJsonLd(origin, storeUrl)` in `web/src/lib/seo.ts`, a
  sibling of `cliApplicationJsonLd`: `SoftwareApplication`, category
  `BrowserApplication`, `operatingSystem: 'Chrome'`, free `offers`, `installUrl`
  pointing at the store.
- `faqPageJsonLd(EXTENSION_FAQ)`.
- `breadcrumbJsonLd` — freehire → Features → Browser extension.

## Discovery

Three edits, the same three every landing needs:

- `Footer.svelte` — a fifth entry in the Features group.
- `HomeView.svelte` — a card in the `// features` section, which renders on both
  `/` and `/about`.
- `STATIC_PATHS` in `web/src/lib/sitemap.ts`.

## Testing

`extensionFaq.test.ts` asserts the array is non-empty, that every entry has a
question and an answer, and that no question repeats — the same shape the FAQ
modules already carry. The markup itself is not unit-tested; the existing
landings do not test theirs either, and a snapshot of marketing copy fails on
every edit without catching anything.

Verification is the standard web suite: `pnpm run check` and `pnpm test` under
`web/`.
