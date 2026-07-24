# internal/jobview — Public Wire Shape of a Job

The single JSON representation of a job served by the list, detail, and search
endpoints and stored in the search index. One type, projected from the `job.Job`
aggregate, so the API surfaces cannot drift apart.

## Design

- `FromDomain` is the projection source of truth; `FromRow` is a thin shim that
  hydrates a `db.Job` into the domain (`job.FromRow`) and delegates.
- Dictionary columns win over the LLM's enrichment values; countries/regions/cities
  are dict-then-LLM hybrids (see the `Job` field docs).
- `ClassifyReality` is the deliberate exception to the domain-input rule — see its
  doc comment for why it stays row-based.

## Fail-loud enrichment decode (by design)

A single row whose `jobs.enrichment` JSONB fails to decode fails the WHOLE
projection: `FromRow`/`FromDomain` return the error and `FromRows` aborts the batch,
so one broken row 500s the entire list endpoint rather than serving a partial page.
This is intentional: enrichment is written only through the typed, validated
`enrich.Enrichment` contract, so an undecodable row means data corruption or a
contract drift — a loud failure that surfaces immediately, not silent per-row
degradation that hides it. Do not add per-row skip/fallback logic here; fix the row
or the contract instead.
