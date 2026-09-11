# internal/candidate

Everything about the person applying — the stored CV and its edits, the structured extraction, the experience bank, PII masking, ATS scoring, and the CV-vs-vacancy match.

**Layer 4 of 8.**

May import: `platform`, `dict`, `ai`, `identity` — and itself.

Must NOT import: `job`, `application`, `search`, `engage`, `ingest`, `api`.

Both directions are enforced. `depguard` in `.golangci.yml` fails on the
offending import line; `internal/platform/arch/layering` holds the same table and
reports the whole graph at once, including imports that exist only in test files.

## Packages

`atscheck` `cv` `cvedit` `cvmatch` `cvsection` `experience` `fitanalysis` `hardconstraint` `hardconstraint/credentials` `headshot` `jobmatch` `matchanalysis` `pii` `resume` `resumeextract`
