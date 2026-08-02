# GitHub star support toast

**Date:** 2026-08-02
**Status:** approved, ready for planning

## Problem

freehire is open source and nothing on the site asks for the one form of support
that costs a visitor nothing: a GitHub star. The footer carries a "View source on
GitHub" link and the header carries a star badge, but neither asks.

A second ask cannot simply be added on top of the Product Hunt strip. Until 26
August that strip already occupies the "support us" slot under the header, and two
simultaneous pleas read as nagging. The asks must queue, not stack.

## Solution

A dismissible toast, floating bottom-right, that appears only once the Product Hunt
strip has stopped asking — either because the visitor closed it, or because the
launch day has passed and it retired itself.

### Why a toast and not a second strip

The Product Hunt banner renders on the server and hides via a class set before first
paint. That machinery exists for one reason: the strip sits in the document flow, so
mounting it after hydration would shove the page down under someone already reading.

A `fixed` toast moves nothing, so none of that applies. It renders after mount, which
means **`app.html` is not touched at all** — and the SHA-256 CSP hash covering its
single inline script, which breaks silently and takes the anti-FOUC theme script down
with it, stays intact.

### Show condition

Pure, testable, no clock beyond what Product Hunt already exposes:

```
show = !selfDismissed && (phBannerDismissed || launchPhase(now) === 'over')
```

The second disjunct is load-bearing. After 26 August `ProductHuntBanner` returns
nothing, so its dismissal key can never be written; gating on the key alone would
mean the toast never appears for anyone who arrives after launch day.

`phBannerDismissed` reads the existing `hire.ph-banner-dismissed` key. "The visitor
closed the Product Hunt strip" is already an observable fact — no new state is
introduced to represent it.

### Deferring to the cookie banner

`CookieConsent` is `fixed inset-x-4 bottom-4 z-50 … sm:right-4 sm:max-w-md` — the
same corner. While `bannerVisible()` is true the toast does not render. Consent is an
obligation; a promo is not. This mirrors the rule already recorded in the layout, that
a promo strip must not push a security notice further from the header.

`bannerVisible` is a rune-backed getter, so the toast appears on its own the moment
consent is settled. No timer, no subscription.

### Stacking

`z-30` — below `HiddenToast` (`z-40`). Undo-after-hide has a five-second window; a
promo has no right to cover it.

### Star count

`web/src/lib/github.svelte.ts` already provides everything: a singleton store with
`$state`, an idempotent `load()`, a 6-hour `localStorage` cache, silent degradation to
no-number on API failure, and `formatStars` (10870 → "10.9k"). The toast calls
`githubStars.load()` and reads `githubStars.count`. Because the cache is shared with
the header badge, the toast adds no GitHub request in the common case.

`connect-src` is deliberately unset in `web/svelte.config.js` (and there is no
`default-src`), so the call to `api.github.com` is not a CSP concern. The pinned
`img-src` is untouched — the toast ships no external image.

### Dismissal

Key `hire.support-toast-dismissed`, permanent. Clicking through to GitHub dismisses it
exactly as the close button does: someone who went to star the repo must not be asked
again.

Storage failures follow the Product Hunt precedent — an unreadable store reads as "not
dismissed", an unwritable one makes the dismissal last for the page only. A promo
banner must never throw inside the layout.

### Route exclusion

Not shown on `/open`. That page is the open-source pitch; a toast repeating it is
noise.

## Components

| File | Role |
|---|---|
| `web/src/lib/supportToast.ts` | new — show condition and dismissal persistence, no runes, no `$lib` imports |
| `web/src/lib/supportToast.test.ts` | new — gate table and storage-failure cases |
| `web/src/lib/components/SupportToast.svelte` | new — markup, mount gate, route exclusion |
| `web/src/routes/+layout.svelte` | edit — mount beside `<CookieConsent />` |

`supportToast.ts` imports `./productHunt` by relative path, not through `$lib`: the
project's vitest environment does not resolve the alias, and an aliased import fails at
module load rather than in a test body.

Untouched: `productHunt.ts`, `github.svelte.ts`, `app.html`, `svelte.config.js`.

Superseded during implementation, after code review: `ProductHuntBanner.svelte` also
changes, and `web/src/lib/phBanner.svelte.ts` is added. A dismissal snapshot taken at the
toast's mount would not fire in the session where the visitor closed the strip — which,
before the launch day, is the only path to the toast. The authoritative record is
`openspec/changes/github-star-support-toast/design.md`.

## Copy

> **freehire is open source.** Free to use, and it stays that way because people star it.
>
> Star on GitHub ★ 1.2k →

The count is omitted when `githubStars.count` is null, leaving a working link.

## Testing

`supportToast.test.ts` covers the gate — before launch with the strip untouched, strip
dismissed, after launch, self-dismissed — and dismissal persistence when `localStorage`
throws. Markup is not tested: the project has no `.svelte` test runner, and the unit
suite runs plain `.ts` in a node environment.

Manual check in a live browser, because two failure modes here are invisible to the
build: the toast must not overlap the cookie banner on a narrow viewport, and it must
not cover `HiddenToast`'s Undo.
