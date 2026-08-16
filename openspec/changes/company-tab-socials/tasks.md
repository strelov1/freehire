## 1. Types and derivations

- [x] 1.1 Declare the already-on-the-wire `CompanyInfo` keys in `web/src/lib/types.ts`:
  `ceo`, `twitter`, `facebook`, `instagram`, `locations`.
- [x] 1.2 Write the failing tests first in `web/src/lib/companyDetails.test.ts`:
  `companySocials` order, accessible label, trimming, and the scheme allow-list
  (`javascript:`, `data:`, `file:`, protocol-relative, non-URL); `companyLocations`
  upper-casing, dedup, and rejection of non-two-letter codes; the CEO's position in
  `companyFacts`; and the two new `hasCompanyDetails` paths.
- [x] 1.3 Add `companySocials` and `companyLocations` to `web/src/lib/companyDetails.ts`,
  put the CEO in `companyFacts` between Headquarters and Type, and fold both new
  derivations into `hasCompanyDetails`.

## 2. Brand marks

- [x] 2.1 Add X, Facebook and Instagram to `design-system/src/provider-icon.svelte`,
  accepting both `twitter` and `x` for the same mark.

## 3. Panel

- [x] 3.1 Render the link row beside the badges: brand marks, `nofollow noopener
  noreferrer`, `target="_blank"`, an `aria-label` per link, `Globe` for the website.
- [x] 3.2 Render the office countries as a capped `CountryFlagStack` in its own row, with
  linking off.

## 4. Verification

- [x] 4.1 `pnpm --dir web test`, `pnpm --dir web check`, `pnpm --dir web lint` — all clean
  of new issues. Design-system `lint`, `check`, `test`, `check:dist`, `check:tokens`,
  `validate:docs` clean, and `check:adoption` baseline updated for the new
  `ProviderIcon` consumer.
- [x] 4.2 Verify in headless Chrome against live data: a company with several links shows
  the marks in order with the right `rel` and accessible names; the CEO appears in the
  facts row; the office flags render and cap.
- [x] 4.3 Confirm the server-rendered job HTML still carries none of this — the tab is
  client-only by design.
