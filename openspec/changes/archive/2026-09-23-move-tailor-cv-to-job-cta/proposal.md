## Why

Tailoring a CV against the posting is the most valuable thing the job page can offer, and
it is currently the one action buried furthest from where a reader decides: a button at the
bottom of the Profile-match sidebar, below a score, a progress bar and up to thirty skill
chips — on a phone, a whole screen above the description it belongs to. The page's own CTA
row, directly under the title, carries only the ways to leave for someone else's site.

Moving it there also settles what the page is FOR. Applying sends the reader away; tailoring
keeps them here and is the paid feature the plan meters. The CTA row should say so.

## What Changes

- The `Tailor my CV` button moves from the Profile-match sidebar to the job page's CTA row,
  beside `Apply`, and appears in all three places that row is rendered: under the title on
  desktop, in the pinned header once the title scrolls away, and in the phone's sticky
  bottom bar.
- **BREAKING (visual hierarchy):** `Tailor my CV` becomes the page's single primary
  (brand-fill) CTA in every state. `Apply` is always an outline button; auto-apply never
  carries the brand fill. The existing "auto-apply is the primary CTA when it can be
  started" rule is replaced.
- Auto-apply keeps its own rank question — whether it is the offered WAY TO APPLY — which
  still decides the phone's sticky bar and whether the external link reads `Apply` or
  `Show origin`. That question is separated from the brand fill it used to imply.
- The sidebar loses the button, the `N of today's CV tailorings left` caption and the
  spent-allowance message. The confirmation dialog already says both **for tailoring**, at
  the moment the reader decides. The sidebar keeps the `Upload a CV to analyse` prompt and
  the cached analysis card.
- **A spent FIT-ANALYSIS allowance no longer hides the button.** The old block suppressed it
  on either feature's ceiling; the button spends a tailoring session, and the tailoring
  workspace reads the analysis best-effort, so tailoring still works. See `design.md`.
- A guest sees the CTA button and gets the sign-in dialog, as they do from the sidebar
  today. The sidebar's separate guest-only rendering of the offer goes away with it.
- The button is withheld only when the page knows the reader has no CV; the sidebar's own
  `Upload a CV` prompt is the next step there.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `job-page-actions`: the page's primary CTA is `Tailor my CV`, not the apply link or
  auto-apply; the CTA row carries three controls; the phone's sticky bar carries two.
- `job-profile-match`: the sidebar match block no longer carries a tailoring
  call-to-action, and the guest offer is made from the CTA row instead.

## Impact

- `web/src/lib/autoApplyButton.ts` — `JobCtaPlan` loses `primary` on both controls; the
  auto-apply entry gains `leads` (is auto-apply the offered way to apply). Its unit tests
  move with it.
- `web/src/lib/components/JobView.svelte` — new `tailorCta` snippet; owns the tailoring
  action (`askConfirmTailor` + navigate) and the guest sign-in branch; owns the single
  `getMatchAnalysis` read for the page and passes it down.
- `web/src/lib/components/JobMatch.svelte` — takes the match analysis as a prop; drops its
  guest-only render of the offer.
- `web/src/lib/components/JobDrawer.svelte` and `SwipeDeck.svelte` — the block's other two
  callers, which hand it `null`. **Both lose the compact fit card and the tailoring button
  they showed today.** For the drawer that is a gain: `MatchAnalysisFull` renders directly
  below it, and the drawer has a `Tailor CV` button of its own. For the swipe deck it is a
  genuine removal — the deck is rapid triage with no call-to-action row, so the button has
  nowhere to live under the one-primary-CTA-per-page rule, and reading an analysis per card
  would spend a request on a verdict almost no card has.
- `web/src/lib/components/MatchSummary.svelte` — stops fetching, loses the button and both
  allowance branches.
- No backend, API or database change. `MatchAnalysisResponse.tailor_allowance` stays on the
  wire; only the sidebar stops reading it.
