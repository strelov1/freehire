## Why

Users can upload one résumé and get it analyzed, but they cannot *author* tailored CVs inside freehire — they must edit and export documents elsewhere. The structured data we already extract into the profile (`resume_structured`) is a ready seed for a CV builder. Standing this up now unlocks the natural next step the analysis was always heading toward: modifying a CV per vacancy and exporting an ATS-clean PDF.

## What Changes

- Introduce a **CV builder**: a user owns N editable CVs, each a structured JSON document, edited via a form under `/my/cvs`.
- Add a **PDF renderer behind a `Renderer` interface**, implemented with **Typst** (`typst compile`). The engine is data-first: the CV schema and storage are independent of the rendering engine, so a future Chrome/LaTeX renderer swaps in without touching data.
- **Seed** the first CV from the existing `resume_structured` extraction; an empty skeleton when none exists.
- Render PDFs **on-demand and stream** them (`application/pdf`); nothing is persisted (S3 caching is a noted seam).
- Ship **one ATS-safe, single-column template** (`classic-ats`), with a `template_id` column and a template registry ready to grow.
- Renderer is **feature-gated / nil-safe** (like blobstore/meili/llm): with no Typst binary configured the CRUD surface still works and the PDF endpoint returns `501`.
- The `cvs` table carries a **nullable `job_id` seam** for the follow-up tailoring phase; no tailoring logic is built here.
- Existing `job-fit-analysis`, `resume-verdict`, and `resume-storage` behavior is **untouched** — this change is purely additive.

## Capabilities

### New Capabilities
- `cv-builder`: authoring, storing, seeding, and PDF-rendering of per-user structured CVs, including the `Renderer` abstraction and the ATS template contract.

### Modified Capabilities
- `resume-structured-profile`: the structured-résumé contract is extended so a CV seeded from it is complete — per-role location, achievement highlights, and technology stack, plus a flat skills list and portfolio projects.

## Impact

- **New code:** `internal/cv/` (schema+Sanitize, seed, Renderer/TypstRenderer, template registry+`classic-ats.typ`+fonts, store), `internal/handler/cv.go`, `internal/db/queries/cvs.sql`, new migration, `web/src/routes/my/cvs/`.
- **DB:** new `cvs` table (`id uuid`, `user_id bigint FK`, `title`, `template_id`, `data jsonb`, `job_id bigint NULL FK`, timestamps). Migration applied manually on prod per convention.
- **Contracts:** `cv.Document` wire shape generated to `web/src/lib/generated/contracts.ts` via `cmd/gen-contracts`.
- **Config/deploy:** new optional `TYPST_BIN` env; Typst binary + bundled fonts baked into the distroless image (multi-stage `COPY` + `--font-path`).
- **API:** new `RequireAuth` (cookie-only) routes under `/api/v1/me/cvs`.
- **Reuses, does not modify:** `resumeextract.Structured` (seed source), `ledongthuc/pdf` (ATS text-extraction test), existing feature-gating and `Sanitize` patterns.
