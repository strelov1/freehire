## Context

Two promo surfaces already exist and both are relevant.

`ProductHuntBanner.svelte` is a strip under the header. It renders on the server and is
hidden by a class the no-flash script in `app.html` sets before first paint, because the
strip sits in the document flow and mounting it after hydration would shove the page down
under someone already reading. Its wording follows the clock through `launchPhase()`, so
it turns itself from "launches on 26 August" into "live today" and then retires — nobody
ships a web build on launch day. It records dismissal under
`hire.ph-banner-dismissed`.

`github.svelte.ts` already fetches and caches the star count for the header badge: a
singleton store with `$state`, an idempotent `load()`, a 6-hour `localStorage` cache under
`hire.gh_stars`, silent degradation to no-number on failure, and `formatStars`.

`CookieConsent.svelte` occupies the same corner the toast wants —
`fixed inset-x-4 bottom-4 z-50 … sm:right-4 sm:max-w-md` — and gates on `bannerVisible()`
from `consent.svelte.ts`. `HiddenToast.svelte` is the feed's Undo affordance at `z-40`.

Constraint worth restating: the single inline `<script>` in `app.html` is allowed by the
SHA-256 hash of its exact contents. Editing it breaks the hash silently, and the browser
then blocks the whole block, anti-FOUC theme script included. `svelte-check`, vitest and
`vite build` all stay green; the only symptom is a flash of the wrong theme.

## Goals / Non-Goals

**Goals:**

- Ask for a GitHub star without ever competing with another ask on screen.
- Reuse the star count already cached for the header badge — no second fetch path.
- Keep the show condition pure and unit-tested.

**Non-Goals:**

- Changing the Product Hunt banner, its copy, or its retirement date.
- Any backend, database, or API work.
- Re-asking on a schedule. A dismissal is permanent.
- Server-side rendering of the toast.

## Decisions

**Render after mount, not on the server.** The strip needed SSR plus a pre-paint class
because it is in the document flow. A `fixed` toast moves nothing, so it can simply appear
on mount. The payoff is that `app.html` is not touched, and the CSP hash stays intact.

Alternative considered: mirroring the strip's SSR machinery for consistency. Rejected —
it would mean editing the inline script for a second dismissal key, buying a silent CSP
failure mode for a surface that does not need it.

**Gate on the Product Hunt key plus the clock.**

```
show = !selfDismissed && (phBannerDismissed || launchPhase(now) === 'over')
```

The second disjunct is load-bearing: after 26 August the strip does not render, so its key
can never be written, and a gate on the key alone would hide the toast from everyone
arriving after launch day.

Alternative considered: a new "the PH ask is done" flag written by the banner. Rejected —
"the visitor closed the strip" is already an observable fact, and a second flag would have
to be kept in step with the first.

**The strip's dismissal becomes a shared reactive flag.** It used to live in
`ProductHuntBanner`'s own `$state`, which was enough while that component was its only
reader. A snapshot taken in the toast's `onMount` would leave the toast invisible for the
rest of the session in which the visitor actually closed the strip — and before the launch
day, closing the strip is the *only* way the toast can appear at all. So the flag moves to
`phBanner.svelte.ts` and both surfaces read it. This removes a second source of truth
rather than adding one; the strip's own behaviour is unchanged.

**The corner has an order of precedence.** It now holds four surfaces, so the rule is
worth stating: consent banner (an obligation) > Undo (a five-second window on a reversible
action) > a page's own bottom-anchored call to action > this promo. The last of these is
why the toast waits for `lg` on a job page, where the Apply bar is `lg:hidden` and sits on
the same layer in the same box. Excluding job pages outright was rejected — they are the
highest-traffic surface on the site, and the conflict only exists below `lg`.

**Yield to consent, sit under Undo.** Consent is an obligation and a promo is not, so the
toast does not render while `bannerVisible()` is true; because that is a rune-backed
getter, the toast appears on its own once consent is settled, with no timer and no
subscription. `z-30` puts the toast under `HiddenToast`'s `z-40`: Undo has a five-second
window and a promo has no right to cover it.

**Following the link dismisses.** Someone who went to star the repository has answered the
ask. Treating only the close button as an answer would keep pestering exactly the people
who complied.

**Pure module, relative import.** The gate and the persistence live in
`web/src/lib/supportToast.ts`, free of runes and of SvelteKit imports, so it unit-tests in
the plain-node vitest environment the way `productHunt.ts` does. It imports
`./productHunt` by relative path: the project's vitest setup does not resolve `$lib`, and
an aliased import fails at module load rather than inside a test.

**Storage failures follow the Product Hunt precedent.** An unreadable store reads as "not
dismissed"; an unwritable one limits the dismissal to the current page. Showing a banner
to someone who closed it is a smaller harm than a throw inside the layout.

## Risks / Trade-offs

- **The toast and the cookie banner both want the bottom-right corner** → they are mutually
  exclusive by gate, so they can share the position; the exclusion is what makes it safe,
  and it is covered by a scenario.
- **On a narrow viewport the toast spans the width and can meet the centred Undo toast** →
  `z-30` keeps Undo on top and clickable. Neither collision is visible to the build, so
  both are on the manual check list.
- **The gate depends on another surface's storage key** → a rename of
  `hire.ph-banner-dismissed` would silently delay the toast until after launch day rather
  than break a build. Mitigated by importing the key from `productHunt.ts` instead of
  restating the literal.
- **A permanent dismissal means one shot per visitor** → accepted. Re-asking a person who
  already said no costs more goodwill than the marginal star is worth.
- **The count can be stale by up to six hours** → accepted; it is the same number the header
  badge shows, and a stale-but-instant number beats a spinner.

## Migration Plan

None. Additive frontend change, no schema and no API. Rollback is reverting the commit;
the only residue is a `hire.support-toast-dismissed` key in some visitors' local storage,
which is inert.

## Open Questions

None. Copy is set in the proposal and may be tuned at review without changing behaviour.
