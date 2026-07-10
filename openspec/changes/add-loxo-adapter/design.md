## Context

Loxo is an ATS/CRM for recruiting agencies; each agency gets a public careers
board at `<host>/<slug>` where `host` is one of an agency subdomain
(`fitnext.app.loxo.co`), the bare `app.loxo.co`, or a regional pod
(`pod4.app.loxo.co`). A spike over 6 agencies across all three host types
VALIDATED the shape: the careers page is server-rendered HTML listing every
posting as `/job/<base64>`; each detail page carries the role name in `<title>`
(`Role | Agency`) and the HTML description in an embedded
`<script type="application/json">{"description": …}` blob; the base64 job id
decodes to a stable `<agency_id>-<slug>` pair. The real employer is never exposed
— boards show only the agency name.

freehire already has the seams this needs: the careerspage adapter (listing →
detail fan-out over an `HTMLGetter`), the `CompanyEntry.Hub` flag honored by
huntflow for agency/hub boards, and the `harvest-boards` live-validation pattern.

## Goals / Non-Goals

**Goals:**
- One `loxo` adapter that crawls any Loxo agency board regardless of host variant.
- Employer attribution that uses the real client when the posting reveals it and
  falls back to the agency otherwise — never guessed.
- A discovery prober that turns Loxo's search-engine footprint into curated
  `sources/loxo.yml` entries.

**Non-Goals:**
- The paid Loxo Open API (bearer-token gated) — the keyless careers page is the
  source of truth here.
- Cross-source deduplication of agency reposts vs. clients' own ATS postings
  (accepted trade-off; see Risks).
- A `linksource` for one-off Loxo links (out of scope; the seam is a one-liner
  later if TG links need it).
- Automatic committing of discovered boards — the harvester proposes, a human
  curates.

## Decisions

- **Board id = careers URL without scheme (`<host>/<slug>`).** Alternatives: store
  slug alone (breaks on pods/subdomains — the host is not derivable from the slug)
  or a per-entry `Region` host hint (overloads a field meant for API data
  residency). Carrying the full host+slug keeps one code path across all variants;
  the adapter splits on the first `/` to get origin and slug.
- **Detail parse = title + embedded JSON, not schema.org.** Loxo detail pages have
  no `JobPosting` ld+json (verified), so unlike careerspage we read the `<title>`
  (strip ` | <agency>`) and the embedded `application/json` blob's `description`.
  Location/remote are best-effort DOM reads; absent → empty (the location
  dictionary decides downstream, never a guess).
- **ExternalID = decoded `<agency_id>-<slug>`.** Stable and human-legible; the
  agency-id prefix keeps ids unique across boards even before the pipeline
  namespaces by board.
- **Employer via `Hub` (reuse, not new).** `hub: true` boards resolve the client
  only on an explicit title delimiter (`— Client` / `@ Client`); else the agency
  name. Mirrors huntflow's division-breadcrumb resolution. In practice fallback is
  the common case, which is honest (the agency is who posts).
- **Discovery = search footprint, not subdomain brute force.** Loxo has no public
  agency directory and blind-guessing `*.app.loxo.co` is a huge, low-yield space.
  One `site:app.loxo.co` query already surfaced ~10 valid agencies; the prober
  paginates the footprint, extracts `(host, slug)`, and live-validates.
- **Harvester proposes, human curates.** Same philosophy as `harvest-boards`: emit
  draft entries with tech-job counts so the operator seeds only live, tech-bearing
  boards; the committed `sources/loxo.yml` stays the project's validated fact set.

## Risks / Trade-offs

- **Cross-source duplicates** → accepted, no mitigation now. An agency repost and
  the client's own ATS posting are separate rows (freehire dedups only within a
  `source`). Treated as a legitimately distinct listing; a future cross-source
  fuzzy dedup on title+company is a noted seam.
- **Mostly non-tech agencies** → the harvester's tech-count lets the operator skip
  boards with no tech vacancies; the per-job non-tech gate filters the rest
  downstream, so no adapter-level gate is needed.
- **HTML shape drift** (title suffix / JSON blob format could change) → covered by
  fixture-based unit tests that fail loudly if the parse breaks; the spike shows
  the shape is uniform across agencies today.
- **Employer under-attribution** → by design we accept agency-as-company rather
  than risk a wrong client guess (consistent with freehire's "never guess" bias).

## Migration Plan

- Pure additive: new adapter file + one `sources.All` line + new board file + new
  `cmd/harvest-loxo`. No schema or migration.
- Deploy: ship the binary, add a staggered `freehire-ingest@loxo` timer (as for
  other board files); seed `sources/loxo.yml` from the spike agencies, expand via
  `harvest-loxo` output.

## Open Questions

- None blocking. The employer-delimiter heuristic set (`—`, `@`) can be tuned as
  real titles are observed, without changing the design.
