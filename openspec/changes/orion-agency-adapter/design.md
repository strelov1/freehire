## Context

Full batch rationale lives in
`docs/superpowers/specs/2026-10-08-oilgas-agency-adapters-design.md`; this covers only
Orion's own API shape, verified live (2026-10-08):

`GET https://www.orionjobs.com/api/recruitment/job/data/?folder=uk&hasexpired=false&page=N`
returns:

```json
{
  "items": [{
    "id": 4398,
    "url": "/job/biomarker-sample-operations-manager-iii-.../",
    "slug": {"value": "biomarker-sample-operations-manager-iii-..."},
    "title": {"value": "Biomarker Sample Operations Manager III - ..."},
    "postdate": {"value": "08/10/2026"},
    "description": {"value": "<strong>...</strong><br>..."},
    "locationtext": {"value": "United States, Illinois, North Chicago"},
    "employment_type": [{"value": "Contract"}]
  }],
  "pagination": {"total": 220, "to": 10, "page": 1}
}
```

Every leaf field is wrapped in an object carrying `.value` (and sometimes
`.originalValue`, `.predefinedId`) — only `.value` is used. `id`/`url` are bare, not
wrapped. `postdate.value` is `DD/MM/YYYY`. No authentication, no UA requirement, no
pagination token — `page` is a plain query param, and `pagination.to >= pagination.total`
(or an empty `items` array) is the stop condition.

## Goals / Non-Goals

**Goals:**
- Page the API with `hasexpired=false` into the catalogue, one `Job` per item, no
  separate detail fetch (the listing already carries the full description).
- Reuse `GetJSON` (`JSONGetter`) — no new transport, no auth, no pacing.

**Non-Goals:**
- The sitemap (`/api/sitemap.xml`) — weaker and partially stale (some entries 410); the
  API's own `hasexpired=false` filter is the correct "currently open" source, so the
  sitemap is not read at all.
- Any aggregator/hiringOrganization-based company resolution — confirmed live that the
  API never names a real end client; `Company` is the constant `"Orion Group"`.

## Decisions

**Pagination: query-param `page`, not a token.** Unlike `emagine.go`'s
`PostJSON`+`skipCount` body, Orion's API is `GET`-only with `page=N` in the URL and
returns its own `pagination.total`/`pagination.to`. `GetJSON` against
`fmt.Sprintf("%s?folder=uk&hasexpired=false&page=%d", orionAPIURL, page)`, looping while
`len(resp.Items) > 0` and capped by a `orionMaxPages` backstop (mirroring
`emagineMaxPages`'s role — a defense against a `pagination.total` that never actually
stops the loop).

**No detail fetch.** `emagine.go` hydrates per-posting because its listing omits the
description; Orion's listing already includes full HTML description, so `Fetch` maps
every item straight from the listing response — no `fetchDetails`/`HydratingSource`
needed.

**`id` as `ExternalID` (not the `slug`/`url`).** `id` is a stable integer per posting;
`url`'s slug can in principle be edited (title changes reflow the slug) while `id` stays
fixed — the safer dedup key.

**Description sanitization.** The field is already plain HTML with no entity-escaping
observed (no `&amp;`-style double-encoding like energyjobline's), so `sanitizeHTML`
alone, no `html.UnescapeString` step (confirm against the fixture captured from the live
response during implementation; add the unescape step only if a real posting needs it).

## Risks / Trade-offs

- **[`pagination.total` could be inaccurate or the loop could undercount]** →
  `orionMaxPages` backstop bounds worst case the same way `emagineMaxPages` does.
- **[API shape could change without notice (no SLA, no API docs found)]** → accepted;
  this is the same risk every scraped vendor API in this codebase carries, not unique to
  Orion.

## Migration Plan

1. Ship adapter + registry line + tests through normal review.
2. After merge, run a manual crawl and sanity-check the job count (~220 expected) before
   adding the live board.
3. `cmd/add-board --provider=orion --company='Orion Group' --apply` (boardless, no
   `--board` value).
4. Rollback: remove the registry line, retire the board.

## Open Questions

None — the API shape was captured directly from a live response, not inferred.
