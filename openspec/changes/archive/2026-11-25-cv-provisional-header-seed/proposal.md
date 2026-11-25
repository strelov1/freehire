## Why

While a résumé re-extract is pending, Profile already shows provisional contacts from the superseded structure — but CV seed still treats that window as “no structure.” Tailor then copies (or leaves) a base CV with an empty header, so opening a tailored CV looks broken even though identity is known.

## What Changes

- Whole-document CV seed MAY use **provisional contacts** (name, email, phone, location, links) from a superseded structured blob when the stamp is stale or pending, together with banked experience — without seeding superseded semantic sections (summary, education, skills, etc.).
- That composition is **usable** for create/replace when provisional contacts are present and the rest of the seed still has something to write (typically banked roles), so stale-base refresh and first-time bootstrap can fill the header instead of no-opping or refusing while identity is available.
- Opening a tailored CV whose header contact block is empty SHALL **backfill** empty header fields from provisional contacts (same merge rules as seed apply: never overwrite non-empty fields) and persist the heal so reloads stay filled.
- Bank-only with **no** provisional contacts remains unusable for whole-document seed (no blanking defence is weakened).

## Capabilities

### New Capabilities

<!-- none — this extends existing CV seed / résumé identity behaviour -->

### Modified Capabilities

- `cv-tailoring`: Relax the “current structure only” usable-seed gate to admit provisional contacts; add empty-header backfill when a tailored CV is opened/loaded.
- `resume-structured-profile`: Provisional contacts are not display-only on Profile — the same identity source feeds CV seed and header heal paths.

## Impact

- `internal/resume` — expose provisional contacts (or a seed-oriented read) beyond `ProfileReadForUser`.
- `internal/handler` — `bankedSeeder`, `reseedBaseIfStaleVsUpload`, reset-from-résumé, and the tailored CV read/open path (GET and/or tailor bootstrap returning an existing copy).
- `mergeSeedHeader` / `applySeedContent` — reuse for backfill; no wire-shape change required unless we surface a heal signal (not planned).
- Existing blank-header tailored/base rows heal on next open once provisional contacts exist; full semantic reseed still waits for a current stamp or explicit reset after extract completes.
