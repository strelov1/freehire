# internal/job/wikicompany — Wikidata Company Matching

Resolves a company's display name against Wikidata/Wikipedia's free public APIs,
consumed by `cmd/backfill-company-info-wikipedia`. No API key, no billing, no
`go.mod` dependency — plain `net/http` over `*Client`'s three configurable base
URLs (`WikidataAPIURL`, `SPARQLURL`, `WikipediaURL`), pointed at the real hosts by
`New` and at an `httptest` server in every test.

## Lookup

`Client.Lookup(ctx, name)` returns `(*Match, error)`; `nil, nil` means "no
confident match", not an error — the expected outcome for most names, since this
package is a best-effort gap-filler, not a guaranteed resolver.

1. `wbsearchentities` search by name → top candidate QID + Wikidata's own short
   `description`.
2. **The type-confidence gate**: a SPARQL `ASK` walking `wdt:P31/wdt:P279*`
   against a curated business/organization anchor set (`organizationAnchorQIDs`
   in `query.go`) — never a keyword scan of the description. A same-named
   person, place, or concept fails this and yields no match; a real company
   subtype the anchor set doesn't name directly (`CACI` → "American defense
   contractor") still passes, because it's a `P279*` subclass of one of the six
   anchors. See `design.md`/`proposal.md` under
   `openspec/changes/company-info-wikipedia-backfill` for why: a spike found the
   keyword-scan approach produced both false positives and false negatives.
3. On a confident match, the entity's `enwiki` sitelink (if any) is fetched via
   `wbgetentities` and its Wikipedia summary extract pulled — `Match.Tagline`
   (the Wikidata description) and `Match.Summary` (the Wikipedia extract) are
   **independently optional**; either may be empty, but `Lookup` never returns a
   `Match` where both are.

## Robustness

- `doWithRetry` (`http.go`) retries 429/5xx up to `MaxAttempts` with `Backoff`
  between attempts. On the error path it closes the exhausted response's body
  itself and returns `(nil, err)` — no caller ever needs a body when an error
  comes back, so this is the one place that can reliably close it.
- A QID is validated (`isValidQID`) before it is ever interpolated into the
  SPARQL query text, so a malformed or unexpectedly-shaped search result
  surfaces as an explicit error instead of silently building a corrupted query.

## Convention

- The QID anchor set is intentionally narrow — six base classes, not an
  exhaustive subtype list — because the transitive `P279*` walk is what makes a
  narrower list unnecessary. Widen it only if a real company is confirmed
  rejected because its subclass chain genuinely doesn't reach any of the six.
- A caller that needs "checked, no confident match" bookkeeping (to avoid
  re-querying the same name forever) owns that state itself — this package is
  stateless and makes no assumption about how its caller persists a result.
