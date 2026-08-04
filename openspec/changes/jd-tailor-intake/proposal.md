## Why

Tailoring a CV today only works if the job already exists as a catalog `jobs` row reachable by
slug — there is no way to tailor against a posting that lives on another site, or against a JD
the user only has as pasted text. `/my/cvs` needs an entry point that accepts a job from three
sources (our own catalog, an external URL, or plain text) and turns all three into a real
`jobs.id` so the existing tailor/fit stack — which is hard-keyed to one — can run unmodified.

## What Changes

- Add a button/form on `/my/cvs` with three input tabs: pick an existing freehire vacancy, paste
  an external URL, or paste plain JD text. All three redirect to the existing `/tailor/[slug]`
  workspace once resolved.
- Add `POST /api/v1/me/jd/resolve`, accepting `{job_slug}` / `{url}` / `{text, title?, company?}`
  and returning `{job_slug}`.
  - `job_slug`: used as-is, no new work.
  - `url` recognized by a supported ATS (host-scoped `linksource` adapter or board coverage):
    resolved through the existing single-link import path and written as a normal **public**
    job (enrichment queue, Meilisearch indexing — identical to any other ingested job).
  - `url` that only a generic scrape can read, or whose fetch/parse fails outright (422), and
    plain `text`: written as a new **private** `jobs` row.
- Add `jobs.is_private boolean not null default false` (new migration). Private rows get a
  synthetic `external_id` (never deduped against the public `(source, external_id)` space),
  `created_by` set to the submitting user, and synchronous `internal/jobderive.Derive` for
  skills/facets — but are never enqueued onto `enrichment_outbox` (no LLM enrichment spend on a
  one-off private submission).
- Exclude `is_private` rows from the Meilisearch index and from public job listing/search —
  never discoverable by browsing or searching, exactly like a closed job. A private job's own
  slug remains a working direct link, again like a closed job: `GET /api/v1/jobs/:slug`,
  fit-analysis, and CV tailoring do not gate on `created_by` — anyone holding the exact slug can
  read and tailor against it. The privacy guarantee is "never surfaced", not "access-controlled";
  the slug itself (derived from a synthetic UUID) is the only thing standing between a private
  job and the open web.
- One listing DOES need an explicit `is_private` exclusion despite requiring the exact slug to
  reach at all: `GET /jobs/:slug/copies` (openings sharing a role cluster) joins on
  `company_slug`/`role_fingerprint`, so a private job can coincidentally cluster with an
  unrelated **public** one and surface (slug, location, url) to anyone browsing that public
  job's copies — a listing leak reachable without ever knowing the private slug, unlike every
  other read path.

## Capabilities

### New Capabilities
- `jd-tailor-intake`: the `/api/v1/me/jd/resolve` endpoint, the URL-vs-text resolution branching,
  the `is_private` job concept (synthetic external_id, creator-only visibility, no enrichment
  queue), and the `/my/cvs` entry-point UI.

### Modified Capabilities
- `job-search`: the Meilisearch index must exclude `is_private` jobs, and the separate DB-backed
  `GET /api/v1/jobs` list must also exclude them, so a private job never appears in any public
  listing or search surface.
- `job-cluster-copies`: `GET /jobs/:slug/copies` must exclude `is_private` rows from the listed
  cluster members (see Why, above) — the one listing surface reachable without already knowing
  the private job's own slug.

No changes to `job-public-identity`, `job-fit-analysis`, or `cv-tailoring`: a private job's
direct-link read/analyze/tailor behavior is identical to any other job's. The privacy model is
unguessability + never-listed, not per-request ownership checks — deliberately simpler than an
owner-gate applied across every job-by-slug consumer (vote, reminders, saved-search, ghost
reports, community threads, the in-app assistant's job-lookup tools, …), all of which would
otherwise need auditing and would need re-auditing again for every future feature that reads a
job by slug.

## Impact

- **Schema**: new migration adding `jobs.is_private`.
- **Backend**: new handler + route for `/api/v1/me/jd/resolve`; reuse of
  `internal/linkimport`/`internal/linksource` for the recognized-ATS branch; reuse of
  `internal/jobderive.Derive` for the private branch; an `is_private` exclusion added to the
  Meilisearch-indexing query in `internal/search`, the DB-backed `GET /api/v1/jobs` list query,
  and `ListRoleClusterCopies` (backs `GET /jobs/:slug/copies`).
- **Frontend**: new form/tabs on `web/src/routes/my/cvs/+page.svelte`, reusing the existing job
  search combobox and the existing `/tailor/[slug]` workspace unchanged.
- **No changes** to `internal/cvedit`, `internal/matchanalysis`'s prompt chain, or the
  `/tailor/[slug]` workspace UI itself.
