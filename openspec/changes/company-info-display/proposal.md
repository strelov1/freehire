## Why

The company-info backfill (previous change) lands authoritative company facts — size,
founding year, HQ, organization type, industries, tagline, website, and occasional
funding/stock/parent data — on the `companies` row, and the company-detail API already
returns them. Nothing surfaces them yet: the company page shows only a name header and the
jobs list. This change renders those facts so a visitor sees who the company is, not just
its openings.

## What Changes

- Add a `CompanyInfoCard` component to the company page (`CompanyView`), a single-column
  card between the header and the jobs list.
- The card renders **only the fields that are present**, and renders nothing at all when the
  company has no company-info (so unenriched companies show no empty box).
- Card content: tagline; an inline facts line (founded · employees · HQ country · org type);
  industry chips; a website link; and — when present — funding, stock listing, parent
  company, and subsidiaries.
- Extend the hand-written web `Company` type with the company-info fields and a `CompanyInfo`
  shape for the `company_info` JSONB.

## Capabilities

### New Capabilities
- `company-info-display`: rendering the company's authoritative firmographic facts on the
  company detail page, conditional on availability.

### Modified Capabilities
<!-- none: purely additive frontend rendering of data the API already returns -->

## Impact

- **Frontend only.** New `web/src/lib/components/CompanyInfoCard.svelte`; wired into
  `CompanyView.svelte`. `web/src/lib/types.ts` gains optional company-info fields +
  `CompanyInfo`. Reuses `countryLabel` (facets.ts) for HQ.
- No backend/API/schema change (the company-detail response already carries the fields).
- Depends on the `company-info-backfill` change being merged so the data exists.
