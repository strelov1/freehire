## Why

Almost everything a job seeker filters on — seniority, work mode, salary, tech
stack, English level, employment type, relocation — currently lives unstructured
inside `jobs.description` free text. Established job boards expose these as
discrete, faceted fields. To eventually offer
the same filtering, the system first needs a defined, structured **target schema**
that an AI enrichment layer can populate. This change defines that schema only;
populating it (AI extraction) and filtering over it (search) are separate later
changes.

## What Changes

- Add a single `jobs.enrichment JSONB` column holding the structured,
  AI-derived view of a job. Raw source fields on `jobs` are left untouched —
  enrichment is purely additive and derived.
- Define a typed Go domain contract (`internal/enrich`) for that JSONB payload:
  one struct whose fields and **controlled vocabularies** (allowed enum values)
  are the schema's source of truth.
- Define the field catalog drawn from reference boards: work arrangement,
  location/eligibility, compensation, requirements/qualifications,
  classification, and job-time company descriptors.
- Add row-level provenance columns (`enriched_at`, `enrichment_version`) so the
  future enrichment job can find un-enriched rows and re-run on a schema bump.
- Expose `enrichment` (and provenance) on the existing jobs read responses so
  the data is observable end-to-end, defaulting to an empty payload when a job
  has not been enriched.

Non-goals (explicitly out of scope): the AI extraction logic itself, any
filtering/search API or UI, a Meilisearch index, per-field confidence scoring,
and promoting company descriptors to the `companies` entity. The schema is
designed so each of these slots in later without reshaping it.

## Capabilities

### New Capabilities

- `job-enrichment`: The structured, AI-derived field model for a job — its
  storage (a `jobs.enrichment` JSONB payload), the typed contract and controlled
  vocabularies that define every field and its allowed values, provenance
  tracking, and exposure on the jobs read API. Does not include how the fields
  get populated or queried.

### Modified Capabilities
<!-- None. The jobs and companies read endpoints gain an additive `enrichment`
     field in their response payloads, but no existing requirement changes:
     the jobs read behaviour is not specified in openspec/specs/, and the
     companies spec is unaffected. -->

## Impact

- **Schema/migrations**: new migration adding `enrichment JSONB NOT NULL DEFAULT
  '{}'`, `enriched_at TIMESTAMPTZ`, and `enrichment_version INT NOT NULL DEFAULT
  0` to `jobs`. Applies via initdb on a fresh volume (the known
  no-migration-runner seam still stands; this change does not introduce one).
- **sqlc**: regenerate after editing `internal/db/queries/jobs.sql`; `UpsertJob`
  and the read queries surface the new columns. `enrichment` is read as
  `json.RawMessage`.
- **New package** `internal/enrich`: the typed `Enrichment` struct + vocabulary
  constants. No AI calls — this is the contract only.
- **Handlers**: jobs read responses include `enrichment`, `enriched_at`,
  `enrichment_version`. No new endpoints.
- **Frontend/Meilisearch**: untouched now; the JSONB payload is intentionally
  shaped to become the future Meilisearch document.
