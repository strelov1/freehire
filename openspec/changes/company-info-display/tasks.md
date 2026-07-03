## 1. Types

- [x] 1.1 Add optional company-info fields to the `Company` interface in `web/src/lib/types.ts`: `industries?`, `year_founded?`, `employee_count?`, `hq_country?`, `organization_type?`, `tagline?`, `company_info?: CompanyInfo`
- [x] 1.2 Add a `CompanyInfo` interface mirroring the loader's JSONB: `{ homepage?, parent?, subsidiaries?, activities?, funding?: { type?, amount?, year?, investors? }, stock?: { symbol?, exchange? } }`

## 2. Component

- [x] 2.1 Add `web/src/lib/components/CompanyInfoCard.svelte` taking a `company: Company` prop; compute a `hasInfo` guard and render nothing when false
- [x] 2.2 Render tagline; inline facts line (founded · `toLocaleString` employees · `countryLabel(hq_country)` · organization_type — present only); industry chips (rounded-full badge)
- [x] 2.3 Render website link (`target=_blank rel=noopener`) from `company_info.homepage`; and the rare blocks (funding, stock `EXCHANGE: SYMBOL`, parent, subsidiaries) each conditional on presence; add a small funding-amount formatter (e.g. `$250M`)

## 3. Wire-in

- [x] 3.1 Render `CompanyInfoCard` in `CompanyView.svelte` between the header and the `mt-6` jobs block

## 4. Verify

- [x] 4.1 `npm run check` (svelte-check) clean for the touched files; visual check via headless screenshot on an enriched company, an unenriched company (no card), and a reference company (card + empty jobs)
