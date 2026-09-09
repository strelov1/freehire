## Context

See `proposal.md` for the production evidence (`Lookup("Nissan")` → Q270195,
a French commune). This is a single-function fix in
`internal/job/wikicompany/query.go`'s `buildOrganizationCheckQuery`.

## Goals / Non-Goals

**Goals:** Reject a candidate that is transitively both an organization and a
geographic/administrative entity, in the same query round-trip.

**Non-Goals:** Auditing every other possible multi-parent collision in
Wikidata's class hierarchy — this fixes the one found in production, with
anchors broad enough to cover the general shape of the problem (any place),
not just the specific commune.

## Decisions

**One combined `ASK` query with `FILTER NOT EXISTS`, not a second HTTP
round-trip.** A second `isGeographic` check would double the request count per
candidate for no benefit — SPARQL's `FILTER NOT EXISTS` expresses "reaches an
organization anchor AND does not reach an excluded anchor" as a single
boolean, matching how the existing positive check already works.

**Anchor choice: `Q2221906` (geographic location) and `Q56061`
(administrative territorial entity), not an exhaustive list of place types.**
Both were confirmed live against `query.wikidata.org` to catch the commune
that caused the bug, and confirmed NOT to catch any of the spike's known-good
company matches (Paladin Energy, Hitachi Energy, Royal Bank of Canada, CACI).
These two anchors are themselves broad classes with many subclasses (city,
country, region, commune, province, ...), so they generalize past the one
instance found rather than special-casing "commune of France".

## Risks / Trade-offs

- **[Risk]** A genuine company whose Wikidata entity is ALSO modeled as
  administratively significant (rare, but Wikidata's graph has odd edges)
  gets wrongly excluded. → **[Mitigation]** Same shape as every other
  rejection: the company simply keeps `tagline = NULL`, no corruption — and
  this is far preferable to the alternative (a wrong, live tagline).

## Migration Plan

Query-only change, no schema/API impact. Redeploy picks it up automatically;
re-verify with the same live diagnostic used to find the bug (`Lookup` against
"Nissan", "Paladin Energy", "Hitachi Energy", "Royal Bank of Canada", "CACI")
before resuming the interrupted rollout (`company-info-wikipedia-backfill`
tasks 4.2/5.1/5.2).
