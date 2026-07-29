## Context

The intake writes an imported posting under the identity its resolver produced. The last
resolver in the registry is generic: its `Match` is true for every http(s) page, and it files
what it parses under `(weblink, <the URL>)`.

That identity is correct when nothing better is known and wrong when the page is a storefront —
a careers site on a company's own domain fronting an ATS board we crawl. The crawl writes
`(greenhouse, <board>:<id>)`; the link writes a second row for the same vacancy.

Board-level defences exist but are narrow: the URL recogniser does not know vanity hosts by
design, the page-fetch resolver needs an ATS link in the markup (a JS-rendered or 403-guarded
storefront has none), and the id-in-the-URL lookup is written per ATS — greenhouse and ashby
only. Of the three duplicate pairs in prod, two front teamtailor.

A batch pass already collapses such pairs by role cluster, but only when a reindex runs, and
the row remains visible until it does.

## Goals / Non-Goals

**Goals:**

- No second searchable posting for a vacancy the catalogue already carries, whatever ATS the
  storefront fronts.
- The submitted storefront URL still resolves — to the canonical posting.
- One mechanism covering every ATS, rather than one id-lookup per ATS.

**Non-Goals:**

- Pairs whose `role_fingerprint` has diverged. Prod holds these: aggregator copies of a Dropbox
  posting carry a different fingerprint under a title that looks identical. Those are marked by
  the aggregator-suppression pass on a different criterion; why the fingerprint diverges is its
  own investigation.
- Reworking the two batch dedup passes. They write to one `duplicate_of` column by different
  criteria and must run in the order `cmd/reindex` runs them (role, then aggregator) — running
  either alone unmarks the other's work.
- Removing the existing duplicate rows in prod. Done by hand, out of band.

## Decisions

**The check lives in `linkimport.write`,** after `job.New` derives `company_slug` and
`role_fingerprint`, before `UpsertJob`. Rejected: in `intake` (misses `cmd/resolve-url`, and
marking after the write is a second statement outside the transaction); in `UpsertJob` (the
write path of the whole ingest pipeline — dedup policy there risks the catalogue to fix an
import-only problem).

**The canon is `MIN(id)` among the open, non-duplicate rows of the cluster** — the same choice
`RecomputeRoleDuplicatesForCompany` makes, so this answer and the one a later reindex computes
agree rather than fight.

**The row is written and marked, not skipped.** `FindOpenJobByURL` matches duplicates and
answers with the posting they duplicate, so the written row is what lets the storefront URL
resolve at all. Skipping the write would leave nothing to match and re-fetch the page on every
resubmission.

**A collapsed row is not enqueued for enrichment.** It never reaches search, so enriching it
pays an LLM for an invisible posting.

**The answer is `found`, given after the contribution is recorded.** The existing early `found`
(a catalogue hit by URL, before any fetch) keeps its early return. The new one cannot: the board
behind an unrecognised storefront may still be new and worth onboarding.

**Only the generic identity is checked.** Every board identity is already deduplicated by
`(source, external_id)` uniqueness, and checking it would spend a query per import to learn
nothing.

## Risks / Trade-offs

**A wrong collapse hides a real vacancy.** The role cluster is `company_slug` +
`role_fingerprint`, the same key the batch pass uses to collapse reposts, so a false positive
here is a false positive there too — the failure mode is pre-existing, not new. The blast radius
is one imported row, and it stays readable by its own slug.

**One extra query per generic import.** Served by the existing `(company_slug,
role_fingerprint)` index, and only on the generic path — a few writes a week at current volume.

**A duplicate row still accumulates.** The catalogue keeps a row per submitted storefront URL.
That is the price of keeping the URL resolvable; at nine such rows ever, it is not pressing.

**The lookup can fail.** Then the row is written unmarked — today's behaviour. Dedup is an
improvement, not a condition of keeping the vacancy.
