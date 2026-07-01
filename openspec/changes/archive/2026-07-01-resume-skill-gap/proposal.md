## Why

Job seekers don't know which market-relevant skills they're missing for a role. Inspired by
[hirable.pro](https://www.hirable.pro/), we can let a user upload their resume, extract skills
from it, save those into their existing search profile, and show — from our own live
job-market data — how many of the skills expected for the profile's specialization(s) they
lack. We already own every building block (`skilltag` extraction, the `search_profiles` table,
and the `/jobs/facets` market-count endpoint), so this is high value at low cost.

## What Changes

- Add `POST /api/v1/me/resume/extract` (cookie-only, `RequireAuth`): accepts a PDF
  (`multipart/form-data` `file`) or pasted text (`application/json {text}`), extracts text and
  runs `internal/skilltag.Parse`, returns `{data:{skills:[...]}}`. The resume is **never
  stored** — it lives only in request memory.
- New dependency `github.com/ledongthuc/pdf` for server-side PDF text extraction.
- `SearchProfilesView` gains an "Upload resume" control that **merges** extracted skills
  (union, dedup) into the profile form's skills field — never wiping existing entries.
- Each profile card gains a **skill-gap block**: for the profile's specialization(s) the
  frontend queries `GET /jobs/facets?category=<each spec>` (OR), sorts the `skills` facet
  descending, takes the top N=20, and shows coverage `X/N` plus the missing skills as chips.
- Gap is computed on the frontend via a pure `computeGap(marketSkills, profileSkills, n)` — no
  new backend surface beyond extraction.

## Capabilities

### New Capabilities
- `resume-skill-extraction`: a stateless endpoint that turns an uploaded resume (PDF or text)
  into canonical skill slugs via the existing deterministic dictionary, without persisting the
  resume.

### Modified Capabilities
- `search-profiles`: profiles can now be populated from a resume and display a live skill-gap
  analysis (coverage + missing market skills) against their specialization(s).

## Impact

- **Backend:** new `internal/handler/resume.go` + route in `handler.Register`; new go.mod
  dependency `github.com/ledongthuc/pdf`. Reuses `internal/skilltag`. No DB schema change.
- **Frontend:** `web/src/lib/components/SearchProfilesView.svelte` (upload control + gap block),
  new `extractResumeSkills` in `web/src/lib/api.ts`, a pure `computeGap` helper. Reuses the
  existing `/jobs/facets` client.
- **Privacy:** resume file/text passes through the server but is not logged or persisted; only
  canonical slugs are returned.
