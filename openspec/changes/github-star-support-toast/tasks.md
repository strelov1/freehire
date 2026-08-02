## 1. Show condition and dismissal (pure module)

- [ ] 1.1 Write `web/src/lib/supportToast.test.ts` covering the gate: the Product Hunt
      strip still asking, the strip dismissed before launch day, the launch day passed
      with the strip never dismissed, and the toast already dismissed. Watch it fail.
- [ ] 1.2 Write `web/src/lib/supportToast.ts` with the dismissal key, `readDismissed` /
      `writeDismissed`, and a pure `shouldShow`. Import `./productHunt` by relative path
      and reuse its dismissal key rather than restating the literal.
- [ ] 1.3 Extend the test with storage failures — an unreadable store reads as "not
      dismissed", an unwritable one throws nothing — and make them pass.

## 2. The toast

- [ ] 2.1 Create `web/src/lib/components/SupportToast.svelte`: read the gate on mount,
      call `githubStars.load()`, render the count via `formatStars` when present and the
      bare link when not, close on the button and on following the link.
- [ ] 2.2 Gate the render on `bannerVisible()` from `consent.svelte` and on the route not
      being `/open`.
- [ ] 2.3 Position it bottom-right at `z-30`, matching the consent banner's box so the
      two never disagree about the corner.

## 3. Wiring

- [ ] 3.1 Mount `<SupportToast />` in `web/src/routes/+layout.svelte` beside
      `<CookieConsent />`, outside the flex column, with a comment saying why it is not
      next to `<ProductHuntBanner />`.

## 4. Verification

- [ ] 4.1 Run the unit suite and `svelte-check`; both green.
- [ ] 4.2 Check in a live browser: the toast does not appear while the consent banner is
      up, appears once consent is settled and the Product Hunt strip is closed, does not
      cover the hide-a-job Undo, and does not overlap the consent banner on a narrow
      viewport.
