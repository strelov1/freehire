# internal/candidate

Everything about the person applying — the stored CV and its edits, the structured extraction, the experience bank, PII masking, ATS scoring, and the CV-vs-vacancy match.

**Layer 4 of 8.**

May import: `platform`, `dict`, `ai`, `identity` — and itself.

Must NOT import: `job`, `application`, `search`, `engage`, `ingest`, `api`.

Both directions are enforced. `depguard` in `.golangci.yml` fails on the
offending import line; `internal/platform/arch/layering` holds the same table and
reports the whole graph at once, including imports that exist only in test files.

## Profile vs CV

Two words that look alike and are different things; the agent-facing tools keep them apart.

- **Profile** is who the candidate is and what they want: roles, skills, location, languages, and the
  headline and summary shown on `/my/profile`. The text lives in `resume.Owned` (what a CV upload
  seeds and the candidate then owns), the preferences in `userprofile`. Agents read it with
  `get_profile` and change it only with `update_profile`.
- **CV** is a document sent to an employer: the base copy and each tailored copy. Written only by
  `cvedit` (`cv_edit`). A CV also has a `summary`; it is a separate string from the profile's.

`get_profile` reports the profile text as `background` (and, deprecated, as `cv`). It does not read any
CV document, so an edit made with `cv_edit` does not show there. The packages keep their old names
(`resume`, `resumeextract`): renaming them is churn without value.

## Packages

`atscheck` `cv` `cvedit` `cvmatch` `cvsection` `experience` `fitanalysis` `hardconstraint` `hardconstraint/credentials` `headshot` `jobmatch` `matchanalysis` `pii` `resume` `resumeextract`
