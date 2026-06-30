## Why

getmatch.ru exposes a posting's grade, required experience, skills, and specialization as **structured** fields in its detail API, but our adapter ignores them — it only reads the HTML description. freehire then derives `seniority`/`category`/`skills`/`experience_years_min` purely from the title/description dictionaries, which stay silent on getmatch's marketing/Russian titles (e.g. "Приглашаем бизнес-аналитиков", "AI Engineer (AI-агенты)"). Result: ~60% of getmatch jobs carry no seniority facet, so a user filtering `remote + senior/lead/c_level` sees only 73 of the ~204 jobs getmatch.ru itself returns for the same filter.

## What Changes

- Adapters can emit **structured facet signals** (`Seniority`, `Category`, `Skills`, `ExperienceYearsMin`) on `sources.Job`, mirroring the existing `WorkMode` field's "structured signal only, never a heuristic" contract.
- Derivation precedence per facet becomes **structured source signal → dictionary → description**, generalizing the rule `work_mode` already follows. The dictionary remains the fallback when the source is silent; a source value never invents an out-of-vocabulary facet.
- The getmatch adapter populates these fields from the detail response it already fetches, mapping getmatch's values into freehire's controlled vocabularies and **dropping anything unmappable** (never guessing): grade → `SeniorityValues`; `skills_objects` canonicalized through the existing `skilltag` dictionary; `required_years_of_experience` → `experience_years_min`; `specializations` → `CategoryValues` via an explicit subset map (a mixed-category offer resolves to empty, like WorkMode on conflict).
- Salary is explicitly out of scope (needs schema work and crosses the LLM-owned enrichment boundary) — noted as a follow-up seam.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `source-ingest`: source adapters gain optional structured facet fields on the normalized `Job`, and the getmatch adapter maps its detail data into them under freehire's vocabularies.
- `deterministic-facets`: the `seniority`, `category`, `skills`, and `experience_years_min` facets accept a structured source signal that takes precedence over the deterministic dictionary, extending the precedence rule already in force for `work_mode`.

## Impact

- `internal/sources/source.go` (`Job` struct gains structured facet fields), `internal/sources/getmatch.go` (decode + map detail fields).
- `internal/jobderive/jobderive.go` (per-facet precedence: source → dictionary → description) and `internal/pipeline/pipeline.go` (pass the new `Job` fields into `jobderive.Input`).
- No schema/migration changes (columns `seniority`, `category`, `skills`, `experience_years_min` already exist).
- Data refresh: getmatch is boardless, so its next full crawl re-upserts every offer with the structured fields; `backfill-derive` cannot recover them (the structured signal exists only at ingest from the live API). A `reindex` surfaces the refreshed facets to search.
