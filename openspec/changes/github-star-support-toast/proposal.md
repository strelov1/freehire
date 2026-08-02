## Why

freehire is open source, but nothing on the site ever asks for the one form of support
that costs a visitor nothing — a GitHub star. The footer links to the repository and the
header shows a star count, yet neither asks.

The ask cannot simply be stacked on what is already there: until 26 August the Product
Hunt strip under the header occupies the "support us" slot, and two simultaneous pleas
read as nagging. The asks have to queue.

## What Changes

- A new dismissible toast, floating bottom-right, asking the visitor to star the
  repository, with the live star count when one is available.
- The toast appears only once the Product Hunt strip has stopped asking — either the
  visitor closed it, or the launch day has passed and it retired itself.
- The toast yields the corner to the cookie-consent banner and sits below the
  hide-a-job Undo toast, so neither is ever covered.
- Dismissal is permanent, and following the link counts as dismissal.
- Not shown on `/open`, which is itself the open-source pitch.
- No change to the Product Hunt banner, the star store, `app.html`, or the CSP.

## Capabilities

### New Capabilities
- `open-source-support-toast`: when the site may ask a visitor to star the repository,
  how that ask queues behind the Product Hunt strip and the consent banner, and when it
  stops being shown.

### Modified Capabilities

None. The Product Hunt banner keeps its current behaviour; the new toast only reads the
dismissal it already records.

## Impact

- `web/src/lib/supportToast.ts` (new) — show condition and dismissal persistence.
- `web/src/lib/supportToast.test.ts` (new) — unit coverage for the gate.
- `web/src/lib/components/SupportToast.svelte` (new) — markup and mount gate.
- `web/src/routes/+layout.svelte` — mounts the toast beside `<CookieConsent />`.

Reads, without modifying: `productHunt.ts` (`launchPhase`, the dismissal key),
`github.svelte.ts` (`githubStars`, `formatStars`, `GITHUB_URL`), `consent.svelte.ts`
(`bannerVisible`).

No backend, database, or API surface is touched. No new external request in the common
case: the star count comes from the cache the header badge already fills.
