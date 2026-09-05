## Why

The job page's action strip has grown from three controls to six — Discussion, Report,
Save, Add-to-list, Auto-apply, Apply — and it shares one row with the content TabStrip.
The strip does not shrink, so every pixel it gained came out of the tabs: on a standard
desktop column the tab row is squeezed to a scrolling sliver, and "Applications" reads as
"Applicat…" with the Discussion link fading over it. The row cannot hold both halves any
more.

The same row also states the wrong priority. Auto-apply is the feature this page is meant
to sell — it is Pro-only and it does the work for the reader — yet it renders as a quiet
grey `secondary` button beside a loud green `Apply` that only opens someone else's site.

## What Changes

- Move the two call-to-action buttons (Auto-apply, Apply) off the tab row and up under the
  job title, on a right-aligned row of their own, where the page's primary actions belong.
  The tab row keeps only the quiet actions — Discussion, Report, Save, Add-to-list — and
  the tabs get their width back.
- Promote Auto-apply to the page's primary CTA when it is offered and clickable: the brand
  green fill, plus a small `Pro` marker inside the button naming the plan it needs.
  Non-clickable states (queued, declined, failed, already applied) stay quiet and disabled
  — a green button nobody can press is a lie.
- Demote the external-apply button to `Show origin` with an outline treatment whenever
  Auto-apply is offered for the posting. It keeps its destination, its `nofollow` rel, its
  new-tab target, and its apply-intent tracking — only its rank on the page changes.
- Where Auto-apply is not offered (every non-Greenhouse posting, which is most of the
  catalogue), the title row carries the unchanged green `Apply` alone.

- Give the phone an auto-apply button. It had none: the sticky bottom bar carried the
  external link and nothing else, so the feature the desktop page now leads with was
  invisible to most of this page's traffic. The bar takes whichever control the plan made
  primary, and the link it displaces joins the quiet strip beside Save.
- Move the view and applied counters out of the sidebar onto the dates line. Both are facts
  about how the posting is doing rather than about the role, and each is an icon and a
  number; in the sidebar a reader weighing "posted 20 minutes ago" against "3 views" had to
  hold one thought across two places.

The same header states the posting's date three times — the reality badge's age chip, the
contrast note beside it, and the dates group — and two of those three are the identical
phrase. The contrast note goes (the chip and the date sit on one line, so the contrast is
already there to read), and the dates group trades the words "Posted" and "Updated" for a
clock and a refresh icon with the short relative time beside each, keeping the words in the
accessible name and the exact instant in the tooltip.

## Capabilities

### New Capabilities

- `job-page-actions`: how the job detail page ranks and places what a reader can do with a
  posting — which control is the primary CTA, where the CTAs sit relative to the title and
  the content tabs, and how the CTA pair changes when auto-apply is available.
- `job-page-provenance`: how the same header states the facts ABOUT the posting — how
  compactly its dates read, and the rule that its date is stated once per page.

### Modified Capabilities

<!-- None. The auto-apply submit trigger's own behaviour (eligibility, statuses, the POST)
     is unchanged; this change only re-ranks and re-places its button. -->

## Impact

- `web/src/lib/components/JobView.svelte` — the `applyCta`, `autoApplyCta` and
  `actionStrip` snippets, and the two rows that render them (the `<header>` block and the
  tab row).
- `web/src/lib/autoApplyButton.ts` — gains `JobCtaPlan` and `jobCtaPlan`, the six-state
  table that maps the `kind` it already returns onto what each of the two buttons says and
  how loud it is. All four render sites read it; there is no per-bar exception.
- `web/src/lib/utils.ts` — `timeAgo`/`formatDateOrAgo` gain an optional short style, passed
  through to `Intl.RelativeTimeFormat` so the compact form stays locale-aware.
- `web/src/lib/reality.ts` — `postingContrast` and its test are removed; `JobView` was its
  only caller and the phrase it produced is what now reads twice.
- `web/src/lib/components/RealityBadge.svelte` — loses the `postedAt` prop that fed it.
- No backend, API, or schema change. No new dependency.
