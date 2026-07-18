## Why

The `/submit` (recruiter contribution) page is a bare textarea form: URL, title, company,
location, source, a remote checkbox, and a plain description box. Recruiters routinely know
the structured facts that make a vacancy useful — required skills, region/city, work mode,
salary — but the form has nowhere to put them, so every submission arrives thin and the
minted job depends entirely on dictionary/LLM derivation to reconstruct facts the submitter
already had in hand. The description box is also plain text, while the catalogue renders
descriptions as formatted HTML, so submitters cannot preview or control formatting.

## What Changes

- **Enrich the submission surface with structured facet inputs.** The submit form and the
  `POST /api/v1/submissions` body gain optional `skills`, `regions`, `cities`, `work_mode`,
  and salary (`salary_min`, `salary_max`, `salary_currency`, `salary_period`) fields. These
  are stored on the submission and carried through to the minted job on approval.
- **Explicit structured facets override dictionary derivation on the job.** When a submitter
  (or moderator) supplies `regions`/`cities`/`work_mode`/`skills` explicitly, those values
  win over what the `location`/`skilltag` dictionaries would derive from location+description.
  Absent an explicit value, derivation still fills the gap (unchanged behaviour).
- **Recruiter salary is an authoritative manual override.** Salary lives in the enrichment
  payload, which the LLM overwrites wholesale. A supplied salary is stored as manual-override
  columns on the job, seeded into the enrichment payload at mint (shows immediately), and
  overlaid over the LLM payload on every enrichment apply — so a recruiter's figure holds and
  is never displaced by the LLM.
- **Reuse the tracker's WYSIWYG editor for the description.** The description field adopts the
  markdown editor already used in the tracking board (`NoteEditor`/EasyMDE), converting its
  markdown to sanitized HTML (via the already-bundled `marked`) on submit so it matches the
  catalogue's HTML description contract.
- **Polish the page within the existing design system.** Section the form (Basics / Details /
  Description), add field icons and clearer spacing, and give it an explicit success state —
  all on the current design tokens and `$lib/ui` primitives, no new visual language.

## Capabilities

### New Capabilities
<!-- None — this extends existing capabilities. -->

### Modified Capabilities
- `job-submission`: the submission accepts and stores optional structured facets (skills,
  regions, cities, work_mode, salary_*); on approval they seed the minted job. The submit UI
  gains the facet inputs and the markdown description editor.
- `job-authoring`: the moderator create/edit body accepts the same explicit structured
  facets, applied as overrides over dictionary derivation.
- `job-aggregate`: `job.New` accepts explicit `regions`/`cities`/`work_mode`/`skills`
  overrides that win over dictionary derivation; supplied salary seeds the enrichment payload.

## Impact

- **API:** `POST /api/v1/submissions` and `POST /api/v1/jobs` request bodies gain optional
  structured facet fields; the submission response echoes them back.
- **Schema:** new nullable columns on `job_submissions` (`skills`, `regions`, `cities`,
  `work_mode`, `salary_min`, `salary_max`, `salary_currency`, `salary_period`) and on `jobs`
  (`salary_min_manual`, `salary_max_manual`, `salary_currency_manual`, `salary_period_manual`);
  a new numbered migration; sqlc regeneration.
- **Backend:** `createJobRequest`, `moderation.CreateInput`, `jobderive.Input`,
  `job.Draft`/`job.New`, the `submission` service/repository, the minter (approve path), and
  the enrichment apply (manual-salary overlay) all carry the new fields.
- **Frontend:** `SubmitView.svelte` redesigned; the description adopts the shared markdown
  editor; new single-value facet inputs reuse the filter's vocabularies (`REGION_LABELS`,
  `COUNTRY_REGION_MAP`, work-mode vocab, currency list) and pill/chip visual style — not the
  stateful `FacetStore` filter components.
- **Dependencies:** none added (`marked`, `easymde` already bundled).
