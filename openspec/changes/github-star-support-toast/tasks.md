## 1. Show condition and dismissal (pure module)

- [x] 1.1 Write `web/src/lib/supportToast.test.ts` covering the gate: the Product Hunt
      strip still asking, the strip dismissed before launch day, the launch day passed
      with the strip never dismissed, and the toast already dismissed. Watch it fail.
- [x] 1.2 Write `web/src/lib/supportToast.ts` with the dismissal key, `readDismissed` /
      `writeDismissed`, and a pure `shouldShow`. Import `./productHunt` by relative path.
- [x] 1.3 Extend the test with storage failures — an unreadable store reads as "not
      dismissed", an unwritable one throws nothing — and make them pass.

## 2. The toast

- [x] 2.1 Create `web/src/lib/components/SupportToast.svelte`: read the gate on mount,
      call `githubStars.load()`, render the count via `formatStars` when present and the
      bare link when not, close on the button and on following the link.
- [x] 2.2 Gate the render on `bannerVisible()` from `consent.svelte` and on the route not
      being `/open`.
- [x] 2.3 Position it bottom-right at `z-30`, matching the consent banner's box so the
      two never disagree about the corner.

## 3. Wiring

- [x] 3.1 Mount `<SupportToast />` in `web/src/routes/+layout.svelte` beside
      `<CookieConsent />`, outside the flex column, with a comment saying why it is not
      next to `<ProductHuntBanner />`.

## 4. Verification

- [x] 4.1 Run the unit suite and `svelte-check`; both green.
- [x] 4.2 Check in a live browser: the toast does not appear while the consent banner is
      up, appears once consent is settled and the Product Hunt strip is closed, does not
      cover the hide-a-job Undo, and does not overlap the consent banner on a narrow
      viewport.

## 5. From code review

- [x] 5.1 Move the Product Hunt strip's dismissal into a shared reactive flag
      (`web/src/lib/phBanner.svelte.ts`), so closing the strip reveals the toast in the
      same session rather than on the next page load — before the launch day that is the
      only path to the toast at all.
- [x] 5.2 Stop the toast covering the job page's sticky mobile Apply bar: add the pure
      `ownsMobileStickyCta` rule and hold the toast to `lg` and up on those routes.
- [x] 5.3 Move the route rules into the pure module and cover them with tests; add the
      missing `'live'`-phase case; replace the tautological key-comparison test with one
      asserting `writeDismissed()` leaves the Product Hunt key alone.
- [x] 5.4 Accessibility and copy: `role="complementary"` with a label instead of
      `role="status"` (a standing promo must not be announced as a live status), a
      qualified dismiss label, and copy that no longer claims stars keep the site free.
- [x] 5.5 Update the proposal, design and spec to match — including the new scenarios for
      the in-session reveal and for a page's own bottom-anchored action.
