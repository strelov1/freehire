## Context

The `/submit` page (`web/src/lib/components/SubmitView.svelte`) posts to
`POST /api/v1/submissions`, which validates the body against the same
`moderation.CreateInput` contract a moderator uses, stores a `pending` row in
`job_submissions`, and — on moderator approval — mints a live job through
`moderation.Service.Create` (the "minter"). The minter runs `derive(...)` →
`job.New` → `jobderive.Derive`, which produces the deterministic dictionary facets
(countries/regions/cities/work_mode/skills/seniority/category) and slugs, writes the
job via `UpsertManualJob`, and enqueues it for LLM enrichment.

Three facts shape this design:

1. **`jobderive.Derive` already implements explicit-wins precedence** for `WorkMode`,
   `Skills` (`unionSkills(explicit, dictionary)`), `Seniority`, `Category`, and
   `ExperienceYearsMin` — the dictionary fills only when the caller left the field
   empty. But `jobderive.Input` has **no `Regions`/`Cities` inputs**; those come solely
   from `geo` (derived from `Location`).
2. **Salary is not a job facet.** It lives in the enrichment payload
   (`enrich.Enrichment.SalaryMin/Max/Currency/Period`), which the LLM enrichment pass
   owns and overwrites wholesale at its target version.
3. **The catalogue renders descriptions as sanitized HTML** (`JobDescription {@html}`);
   `moderation.Create` runs `sources.SanitizeHTML` on the description before persisting.

## Goals / Non-Goals

**Goals:**
- Let a submitter (and a moderator) supply `skills`, `regions`, `cities`, `work_mode`,
  and salary on the submit/create surfaces.
- Make `skills`/`regions`/`cities`/`work_mode` **hard facets** on the minted job:
  explicit values win over dictionary derivation; absent values still derive (unchanged).
- Persist the recruiter's salary structurally on the submission and surface it on the
  minted vacancy without fighting the enrichment subsystem's ownership of salary.
- Replace the plain description textarea with the tracker's markdown editor, emitting
  sanitized HTML that matches the catalogue contract.
- Polish the page within the existing design system (tokens, `$lib/ui`, pill/chip style).

**Non-Goals:**
- No "locked/user-authoritative field" concept in the enrichment subsystem.
- No reuse of the stateful filter facet components (`FacetStore`, `LocationPane`,
  `ChipFacet`) — they are built for multi-select faceted filtering, not single-value entry.
- No new visual language, fonts, or layout paradigm; no new npm dependencies.
- No change to who may submit or approve, the dedup rule, or the review lifecycle.

## Decisions

### D1. Extend the structured-facets seam to region & city (explicit wins)
Add `Regions []string` and `Cities []string` to `jobderive.Input`, and apply the same
precedence already used for `WorkMode`: when the caller supplies them, they win over
`geo.Regions`/`geo.Cities`; when empty, geo derivation fills them. Add the matching
`Regions`/`Cities` fields to `job.Draft` and pass them through `job.New`.
*Why:* the seam already exists for WorkMode/Skills — this is a two-field extension of an
established pattern, not a new mechanism. *Alternative rejected:* a separate override map
outside `jobderive` — needlessly duplicates the precedence logic.

### D2. Thread explicit facets through the create/submit contract
Add `Skills`, `Regions`, `Cities`, `WorkMode` (and the salary fields, see D3) to
`createJobRequest` and `moderation.CreateInput`. `moderation.Create`/`Update` pass them
into `derive(...)`. The moderator create path gains the same capability for free (shared
contract), which is desirable and consistent.

### D3. Recruiter salary is an authoritative manual override on the job, overlaid onto the enrichment payload every consumer reads
The recruiter's salary must **hold** — a recruiter may want to state it themselves and have it
survive. Every salary consumer in the app (search facets/sort in `internal/search`, filter
mapping in `query_filter.go`, insights rollups, the jobview wire shape) reads a single
projected source: `jobs.enrichment` JSONB `salary_*`, which the LLM enrichment pass overwrites
wholesale at its target version. So salary is made authoritative in three moves:

1. **Persist on the submission.** `job_submissions` gains `salary_min`, `salary_max`,
   `salary_currency`, `salary_period` — the recruiter's exact input, never lost, moderator-visible.
2. **Store as manual-override columns on the job.** On mint these become nullable authoritative
   columns on `jobs` (`salary_min_manual`, `salary_max_manual`, `salary_currency_manual`,
   `salary_period_manual`). The mint also seeds the `enrichment` JSONB `salary_*` from them so
   salary displays immediately, before any LLM pass.
3. **Overlay at enrich-apply.** When the enrichment worker writes a job's `enrichment` payload,
   it coalesces the manual salary columns **over** the LLM's salary, so the LLM can compute its
   own figure but never displaces a recruiter-provided one. `enrichment.salary_*` therefore
   stays the effective value — all existing consumers are unchanged.

*Why:* this makes the recruiter salary durable and fully decoupled from the LLM (the chosen
mechanism) while reusing every existing salary consumer via the `enrichment.salary_*` projection.
*Alternatives rejected:* (a) fold salary into the description and let the LLM extract it — the LLM
could then adjust the recruiter's figure, which violates "recruiter states it and it holds";
(b) parallel `manual_salary_*` columns that every consumer (search, insights, jobview) must learn
to prefer — broad consumer churn for no gain over the single-projection overlay; (c) a general
"locked fields" concept across enrichment — out of scope.

### D4. Reuse the tracker's markdown editor; convert to HTML on submit
The description field mounts the existing `NoteEditor` (EasyMDE). Its value is markdown; on
submit, convert to HTML with the already-bundled `marked`, then send it as `description`.
`moderation.Create` already runs `SanitizeHTML`, so the server remains the sanitization
authority. *Why:* "our editor from the tracker" reuse with zero new deps; the markdown→HTML
step bridges the editor's output to the catalogue's HTML description contract.
*Alternative rejected:* store markdown and render markdown in the catalogue — diverges from
every other source, which is HTML.

### D5. Form inputs reuse filter vocabularies, not filter components
New single-value inputs (region select, city input, work-mode segmented control, skills
chip input, salary min/max + currency/period) are built on `$lib/ui` primitives and reuse
the shared vocabularies/labels (`REGION_LABELS`, `COUNTRY_REGION_MAP`, the work-mode vocab,
the currency list) and the existing pill/chip visual style (`TokenInput` for skills).
*Why:* the filter components depend on `FacetStore`, faceted counts, and include/exclude
cycling — semantics irrelevant to authoring one value. Sharing the vocabularies keeps the
form consistent with the filter without importing filter state machinery.

## Risks / Trade-offs

- **Enrich-apply overlay is a new cross-subsystem touch.** Making manual salary authoritative
  requires the enrichment worker to coalesce manual columns over the LLM payload on every apply.
  → Bounded and covered by tests: a job with a manual salary keeps it after an enrichment pass;
  a job without one is unchanged. This is the cost of the chosen (robust) salary mechanism.
- **Explicit region/city that the dictionary doesn't recognize.** Values are controlled by
  the shared vocabulary in the UI (region select, country/city from `COUNTRY_REGION_MAP`), so
  free-text drift is avoided; the backend stores what it is given. → Mitigate by validating
  `work_mode` and `regions` against the known vocab server-side and dropping unknowns, mirroring
  how dictionary facets already reject unknowns.
- **Migration in prod is manual** (initdb single-run; see prod manual-migration ownership). →
  New numbered migration; columns are nullable with no backfill, so it is additive and safe.
- **Wider create contract.** Moderators can now set explicit facets too. → Intended; it is the
  same contract and improves the moderator create path symmetrically.

## Migration Plan

1. Add a new numbered migration: nullable `skills text[]`, `regions text[]`, `cities text[]`,
   `work_mode text`, `salary_min int`, `salary_max int`, `salary_currency text`,
   `salary_period text` on `job_submissions`; and nullable `salary_min_manual int`,
   `salary_max_manual int`, `salary_currency_manual text`, `salary_period_manual text` on
   `jobs`. Regenerate sqlc.
2. Ship backend (contract + derive seam + minter fold) behind the same endpoints; the new
   body fields are optional, so existing clients are unaffected.
3. Ship the redesigned form.
4. Rollback: the columns are additive and nullable; reverting the app code leaves them unused
   and harmless. No data migration to undo.

## Open Questions

- Salary period vocabulary on the form: expose the full `SalaryPeriodValues` enum, or just
  `year`/`month`/`hour`? (Leaning to the enum, defaulting to `year`.)
