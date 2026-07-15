# Link source conventions

## Scope
Resolving a single outbound job-detail URL into a fully parsed vacancy under the destination's own identity.

## Always true
- `internal/linksource` turns one outbound job-detail URL into a fully parsed vacancy.
- `sources` adapts a whole ATS board by id; a `LinkSource` adapts a single detail page — it matches the link's host and resolves that one page.
- Adding a destination is a new adapter plus one line in `linksource.All` — the same shape as `sources.All`.
- The resolved job is stored under the destination's identity, not Telegram's, so it dedups against the same posting if another source also has it.

## How it works
A Telegram post often just links to a real vacancy elsewhere. Rather than treating the Telegram post itself as the job, `internal/linksource` follows the outbound URL and resolves the actual detail page at the destination ATS. This reuses the same adapter pattern as `internal/sources` but at the granularity of a single page: a `LinkSource` matches the link's host and parses that one detail page into a normalized job. The job is then stored under the destination source's identity (e.g. greenhouse, lever), not under telegram, so the dedup key `(source, external_id)` naturally prevents duplication if another source also carries the same posting.

## Limitations
None currently listed.
