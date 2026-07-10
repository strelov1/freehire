## Context

The company page (`web/src/routes/companies/[slug]/+page.svelte` → `CompanyView.svelte`)
renders a header (logo, name, follow) then a streamed jobs list. The company-detail API
returns `db.Company`, which now carries the company-info columns (`industries`,
`year_founded`, `employee_count`, `hq_country`, `organization_type`, `tagline`) and the
`company_info` JSONB. The hand-written web `Company` type (`web/src/lib/types.ts`) does not
expose them yet. `facets.ts` already has `countryLabel(code)` (ISO alpha-2 → English name).

## Goals / Non-Goals

**Goals:**
- Show the company's authoritative facts on its page, degrading gracefully as fields are
  missing.

**Non-Goals:**
- No backend/API/schema change (data already served).
- No `industries` search facet or company-list changes (a separate change).
- No JSON-LD enrichment (foundingDate/numberOfEmployees) — noted as a later refinement.

## Decisions

**A single stacked card, not a sidebar.** The card sits between the header and the jobs
list, full width. *Alternative — a two-column sidebar* — was rejected during design as a
larger layout change for little gain; the card reads well stacked and keeps the jobs list
full width.

**Render-only-what-exists, hide-when-empty.** Every field is conditional; the whole card
returns nothing when no company-info field is set. This keeps unenriched (job-only)
companies visually unchanged and avoids empty scaffolding — the same "dict-silent stays
silent" spirit as the backend facets.

**Type the `company_info` JSONB explicitly.** Add a `CompanyInfo` interface
(`{ homepage?, parent?, subsidiaries?[], activities?[], funding?{type?,amount?,year?,investors?[]}, stock?{symbol?,exchange?} }`)
rather than `unknown`, so the template reads typed fields. The company-info columns are added
to the `Company` interface as optional (older API responses / unenriched rows omit them).

**Reuse existing helpers.** HQ uses `countryLabel`; chips reuse the app's rounded-full badge
styling; employee count is formatted with `toLocaleString`; a small local helper formats a
funding amount (e.g. `$250M`).

## Risks / Trade-offs

- **Type drift between hand-written `Company` and the API** → fields are optional and
  defensively read, so a missing field just hides its line; no runtime break.
- **`company_info` shape depends on the backfill's JSONB assembly** → the `CompanyInfo` type
  mirrors exactly what the loader writes (`funding`/`stock`/`homepage`/`parent`/
  `subsidiaries`/`activities`); documented in both places.

## Migration Plan

Frontend-only; ships with the normal web deploy. No data migration. Depends on the
`company-info-backfill` change being merged and run so the fields are populated (before that,
the card simply never renders).

## Open Questions

- Whether to later feed `year_founded`/`employee_count` into the company `organizationJsonLd`
  for richer structured data — deferred.
