## Context

`GET /api/v1/jobs/find?url=` answers one question for the browser extension: *is the page
in front of the user a posting we carry, and which one?* Today it answers by recovering
the catalog's dedup identity `(source, external_id)` from the URL (`sources.RefFromURL`)
and loading that row by its unique index. That path is exact and cheap, and it replaced a
company+title `ILIKE` match that could not use an index and timed out in production on
millions of rows. Keeping it first is not negotiable.

Its coverage, though, is one ATS out of 175 adapters. The identity of a posting is
provider-specific (Greenhouse numbers jobs, Ashby uses UUIDs, aggregators key on their own
page URL), so every additional provider is a hand-written parser that has to agree with
what that provider's ingest adapter writes into `external_id`.

Meanwhile the catalog already stores each posting's detail URL in `jobs.url`, written by
the source adapter from the feed. For aggregators and most ATS boards that is the same
page the user is standing on — the same string, modulo a tracking query and the usual
scheme/host noise.

## Goals / Non-Goals

**Goals:**
- Resolve a page URL to its catalog posting for any source whose stored `url` is the
  posting's own detail page, without a parser per provider.
- Keep the exact-identity path first and unchanged.
- One definition of URL normalization, shared by the index and the query, so the lookup
  cannot silently stop using its index.
- Stay a single indexed equality lookup — no scan, no fetch, no LIKE.

**Non-Goals:**
- Resolving a page whose posting we do not carry (that is the contribute/import change).
- Resolving to a closed or duplicate posting.
- Fuzzy URL matching (path prefixes, id extraction from arbitrary paths).

## Decisions

### Normalization lives in SQL, as one IMMUTABLE function

An expression index is only used when the query's expression matches the index's
*exactly*. If normalization were written in Go and the index in SQL, the two would be
independent definitions of the same rule — and the failure mode of a drift is silent: the
query still returns correct rows, it just stops using the index and degrades into the
sequential scan this endpoint was rewritten to escape.

So the migration defines:

```sql
CREATE FUNCTION normalize_job_url(text) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT
AS $$ SELECT regexp_replace(
                 regexp_replace(
                     regexp_replace(lower($1), '^https?://(www\.)?', ''),
                 '[?#].*$', ''),
             '/+$', '') $$;
```

and the query applies the same function to the column and to the parameter. The rule is
therefore stated once: lowercase, drop the scheme and a leading `www.`, drop query and
fragment, drop trailing slashes.

`lower`, `regexp_replace` and `SELECT` over them are immutable, which is what makes the
function indexable. `STRICT` keeps a NULL url out of the index.

Dropping the query string is what makes the freehire-tagged link the user followed
(`?utm_source=freehire.me`) resolve to the row it came from — and it is safe here because
no source in the catalog distinguishes two postings by query parameters alone. The
Greenhouse embed form does exactly that (`?for=…&token=…`), which is why it is served by
`RefFromURL` — the first tier — and never reaches this one.

### The index is partial over open, canonical rows

```sql
CREATE INDEX jobs_normalized_url_idx
    ON public.jobs (normalize_job_url(url))
    WHERE closed_at IS NULL AND duplicate_of IS NULL;
```

`jobs` holds several million rows; a full expression index over `text` is a few hundred
megabytes of it we would never query. Restricting to open canonical rows is not only
cheaper — it is the correct semantic: a curated match card exists to tell the user how
they fit a vacancy they can still apply to, and a duplicate row is by definition not the
one we serve. When the posting is closed or suppressed, `find` answers `null` and the
extension's text-match fallback takes over, exactly as it does for an unknown page.

Following `0027_jobs_greenhouse_jobid_idx.sql`, the migration ships the plain
`CREATE INDEX` (correct on a fresh initdb volume) and notes that prod takes it as
`CREATE INDEX CONCURRENTLY` by hand.

### Ties are broken by recency

Two open canonical rows can share a normalized URL — an aggregator and an ATS row that
link to the same page, say. The query orders by `last_seen_at DESC, id DESC` and takes
one: the most recently confirmed row, deterministically. This is a display choice, not a
dedup one; collapsing such pairs stays the ingest passes' job.

### RefFromURL stays, and stays Greenhouse-only

The two tiers answer different questions. `RefFromURL` reads the *identity out of the
URL*, which works even when the page the user is on is not what `jobs.url` holds — the
Greenhouse embedded application form is a different URL from the board's job page, and
only a parser can bridge them. The URL fallback reads the *URL out of the catalog*, which
works for every source that stores the page it scraped. Adding a provider to the first
tier is worth it only when its pages are addressed differently from what its adapter
stores; for himalayas they are identical, so the fallback covers it and no adapter is
written.

## Risks / Trade-offs

- **Sources whose `url` is not the page the user sees.** Some adapters store an apply-form
  or external URL, so their postings stay unresolved by this tier. The outcome is today's
  outcome (`null` → text match), and the first tier remains the escape hatch for those
  providers.
- **Index write cost.** One more expression index on the hottest write table (`ingest`
  upserts). It is partial and narrow; the same shape already exists on `jobs` twice
  (`0026`, `0027`, `0035`).
- **A URL is user-controlled input.** It is a query parameter compared as a bound
  parameter through sqlc — no interpolation, and the normalization is pure string
  rewriting.
- **Query-string-significant hosts.** Any future source that distinguishes postings by
  query alone would resolve to the wrong sibling. No such source exists in the catalog
  today; the Greenhouse case that does is handled a tier earlier.

## Migration Plan

One forward migration; no backfill (the index is built from existing rows). Nothing reads
the function besides the new query, so a rollback is dropping the index and the function.

## Open Questions

None.
