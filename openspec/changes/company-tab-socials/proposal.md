## Why

The job page's Company tab shipped with a fraction of what the catalogue holds. Sampling
40 YC companies on 2026-08-16: the CEO is recorded for 25 of them, a website for 35, a
LinkedIn page for 29, X for 22, Facebook for 21, and office countries for 27 — and the tab
shows none of it.

None of this needs a backend change. The API serves `company_info` as raw JSON, so every
one of these keys already reaches the browser; they were simply never declared in the
front-end type and never rendered.

## What Changes

- The Company tab gains a row of the employer's own links, rendered as brand marks
  rather than URLs: website, LinkedIn, X, Facebook, Instagram — whichever are present.
- The CEO joins the facts row, between Headquarters and Type.
- The company's office countries appear as an overlapping flag cluster.
- `CompanyInfo` in the front-end types gains the keys the importer has always written:
  `ceo`, `twitter`, `facebook`, `instagram`, `locations`.
- The design system's `ProviderIcon` gains X, Facebook and Instagram marks. Brand logos
  live there because icon libraries do not ship them; `twitter` and `x` name the same mark.

**Not included: `tech_stack`.** It is present for 22 of 40, but it is a scanner's
inventory rather than the company's own claim — Stripe's 54 entries are alphabetical and
open with Adobe After Effects and Adobe Premiere Pro. Shown verbatim it would mislead the
reader it is meant to inform. The useful version is its intersection with the curated
skill dictionary, which is a separate piece of work.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `company-info-display`: the requirement covering the job page's Company tab is extended
  with the employer's outbound links, the CEO, and the office countries — including the
  rule that a stored link is only rendered if its scheme is http(s).

## Impact

- `web/src/lib/types.ts` — `CompanyInfo` gains five keys that were already on the wire.
- `web/src/lib/companyDetails.ts` — `companySocials` and `companyLocations` are added, the
  CEO joins `companyFacts`, and both feed `hasCompanyDetails` so a company known only by
  its links no longer reports as empty.
- `web/src/lib/components/JobCompanyPanel.svelte` — renders the three additions.
- `design-system/src/provider-icon.svelte` — three new brand marks.
- `design-system/scripts/adoption-baseline.json` — `ProviderIcon` gains a consumer.
- No backend change, no migration, no search-index change.

## Security note

These link values originate with an external importer and land in an `href`. A
`javascript:` or `data:` URL there would execute script on our own origin, so the scheme
is allow-listed rather than sanitised: anything that is not `http:` or `https:` is dropped.
