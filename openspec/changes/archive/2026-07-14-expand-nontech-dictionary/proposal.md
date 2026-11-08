## Why

Post-classification measurement shows the `is_tech=unknown` bucket is 58% of the open catalogue (1.8M), and it is dominated by **non-tech roles the `classify.IsNonTech` dictionary (~70 terms) misses**, not by hidden tech. Generic enterprise ATS boards (Workday/Oracle/UKG) dump whole company boards, so the unknown mass is nurses, cooks, cashiers, warehouse, trades, paralegals, teachers. A cluster census of the unknown bucket: healthcare ~80K, warehouse/logistics ~62K, retail ~45K, food ~43K, trades ~31K, office/finance ~25K, education ~24K, facilities ~12K — ≥500K obvious non-tech left as `unknown`. This blunts the "exclude Non-tech" filter (it can only hide the 31% already classified) and keeps the catalogue noisy.

## What Changes

- Substantially expand `classify.nonTechTitleTerms` with high-frequency, unambiguous non-tech role nouns across the measured clusters: healthcare (registered nurse, CNA, LPN, medical/dental/pharmacy roles, therapists), food service (cook, food service), retail (cashier variants, retail/store associate, sales clerk), warehouse/logistics (warehouse associate/worker, order picker, material handler, CDL driver, courier), trades (ironworker, laborer, mechanic, pipefitter, mason, roofer, HVAC technician), office/finance (paralegal, payroll, bookkeeper, accountant, teller, administrative assistant), education (substitute teacher, childcare, teaching assistant), facilities (attendant variants).
- Same "never guess" doctrine: only unambiguous non-tech nouns; deliberately keep excluding tech-colliding terms (bare "technician", "engineer", "analyst", "coordinator", "specialist", "server", "warehouse", "chef", "administrator", "officer") so an "IT Technician" / "Security Engineer" / "Data Warehouse Engineer" is never mislabelled non-tech.

## Capabilities

### Modified Capabilities

- `tech-classification`: broaden the non-tech title dictionary that feeds the `is_tech=false` branch (no requirement wording change — the existing "Deterministic non-tech title detection" requirement already governs it; this widens coverage under the same rules).

## Impact

- **Code:** `internal/classify` (grow `nonTechTitleTerms` + tests). No derivation, schema, wire, facet, or UI change — only more `unknown` jobs resolve to `non_tech`.
- **Ops:** after deploy, `cmd/backfill-derive` + reindex. Expect `non_tech` to grow by ~300-500K and `unknown` to shrink correspondingly; `tech` unchanged.
