## Why

The `JobPosting` JSON-LD on job-detail pages — the structured data that makes
postings eligible for Google Jobs across ~99% of the catalogue — carries only the
baseline fields (title, description, hiring org, location, date, salary). It omits
several fields Google reads for richer job results that we already have data for:
the company logo, the required skills, and the experience/education requirements.

## What Changes

- Add to the `JobPosting` JSON-LD, each emitted only when the underlying data is
  present:
  - `hiringOrganization.logo` — the logo.dev URL for the company name (same
    source the SPA and OG cards already use).
  - `skills` — the job's canonical dictionary skills (`job.skills`) joined as
    Google-`Text`.
  - `experienceRequirements` — an `OccupationalExperienceRequirements` with
    `monthsOfExperience` = `experience_years_min × 12`, emitted only when the
    minimum is **greater than zero** (a 0 carries no SEO signal).
  - `educationRequirements` — an `EducationalOccupationalCredential` whose
    `credentialCategory` maps the enrichment `education_level`
    (`bachelor`→"bachelor degree", `master`/`phd`→"postgraduate degree";
    `none` is omitted).

Out of scope (deliberately deferred): `directApply`, `identifier`, `jobBenefits`.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `web-ssr-seo`: the "JobPosting structured data" requirement is strengthened —
  the `JobPosting` object additionally carries the company logo, skills, and
  experience/education requirements it has data for.

## Impact

- `web/src/lib/seo.ts` — `jobPostingJsonLd` gains four conditional fields; imports
  `logoDevUrl` from `./logo`.
- `web/src/lib/seo.test.ts` — new vitest coverage for `jobPostingJsonLd`
  (previously untested).
- No backend, API, schema, or data changes; no route wiring (the function is
  already called on `/jobs/:slug`). No new dependencies.
