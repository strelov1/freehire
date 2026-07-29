# Synchronous dedup at intake

Date: 2026-07-29
Status: designed, not implemented

## Problem

A link pasted from a **storefront** — a careers site on the company's own domain, fronting an
ATS board we already crawl — is imported a second time under `(weblink, <the URL>)`, beside the
row the crawl already wrote.

Prod holds three such pairs out of nine `weblink` rows ever written. Two of them front
**teamtailor**, one **greenhouse**. The greenhouse case is now covered: PR #1244 hands the board
that `contribution.Inspect` resolved to the import, and the id-in-the-URL lookup can name a
greenhouse board from a storefront path. Teamtailor has no such lookup, and neither do lever,
workable, smartrecruiters or any other ATS — an id-lookup exists only for greenhouse and ashby.

Writing one lookup per ATS does not scale: each needs its own id format and its own catalogue
query. A single check on what the page actually parsed to covers every ATS at once.

## Approach

Before writing a `weblink` row, ask the catalogue whether it already holds this vacancy. If it
does, still write the row — but mark it `duplicate_of` the row we already have.

Writing-and-marking rather than skipping the write is deliberate. The row is what lets the
storefront URL resolve at all: `FindOpenJobByURL` matches duplicates and answers with the
posting they duplicate, so the storefront link lands on the canonical card. Skipping the write
would leave nothing to match, and every resubmission of that URL would re-fetch the page.

A marked row costs nothing beyond a row: `linkimport.index` never pushes a duplicate to search,
and `splitJobs` deletes one that was indexed before it was demoted.

### Where the check lives

In `linkimport.write`, after `job.New` (which derives `company_slug` and `role_fingerprint`) and
before `UpsertJob`.

Rejected alternatives:

- **In `intake`** — does not cover `cmd/resolve-url`, and marking after the write is a second
  statement outside the write's transaction.
- **In `UpsertJob`** — that is the write path of the whole ingest pipeline. Dedup policy there
  risks the entire catalogue to fix an import-only problem.

### What counts as already held

A new query, `CanonicalJobForRole(company_slug, role_fingerprint, source, external_id)`:

- same `company_slug` and `role_fingerprint` — the role-cluster key the dedup passes use;
- `closed_at IS NULL` and `duplicate_of IS NULL` — a canon must be open and not itself a
  duplicate, or the marking builds a chain the readers do not expect;
- excluding the row being imported, by its `(source, external_id)` dedup identity, because a
  re-import of the same URL would otherwise find itself;
- `MIN(id)` as the canon — the same choice `RecomputeRoleDuplicatesForCompany` makes, so this
  answer and the one reindex computes later agree.

Served by the existing `(company_slug, role_fingerprint)` index.

The check runs **only** when the identity being written is `weblink`
(`linksource.GenericSource`). Every other identity is a board's own, and `UpsertJob` already
dedups it on `(source, external_id)`. An empty `role_fingerprint` clusters with nothing, so it
is skipped.

### What happens on a hit

Inside the existing transaction: `UpsertJob`, then a new `MarkJobDuplicateOf`, and **skip**
`EnqueueJobEnrichment`. Enriching a row that will never reach search is paying an LLM for an
invisible posting.

`linkimport.Result` gains the canonical `PublicSlug` and a flag saying the import collapsed onto
an existing posting.

### What the caller answers

`intake` answers `found` — the catalogue does carry this posting. This is a **second** route to
that status, reached after the import, and it must record the contribution first: a storefront's
board may still be new and worth onboarding.

The existing early `found` (the `catalogSlugForURL` hit at the top of `Resolve`) is untouched.
That one fires before anything is fetched, on a URL the catalogue already stores verbatim, and
its early return stays as it is.

### Failure

A failed canon lookup does not block the import: the row is written unmarked, exactly as today.
Dedup is an improvement, not a condition of keeping the vacancy.

## Testing

- `linkimport` (integration): a catalogue holding a greenhouse row for the same
  company + fingerprint → importing the storefront page writes a row carrying `duplicate_of`,
  and enqueues no enrichment for it.
- `linkimport` (integration): no canon → unchanged behaviour, row written unmarked and enqueued.
- `handler` (integration): the intake answers `found` with the canonical slug, and the
  contribution row is still recorded.
- `handler` (integration): a board-identity import (recruitee) is untouched.

## Out of scope

Pairs whose `role_fingerprint` has diverged. Prod holds these: the weworkremotely copies of a
Dropbox posting carry `1afd65e8…` where the ATS row carries `48cf750d…`, under titles that look
identical. Those are marked by the aggregator-suppression pass on a different criterion, and
whatever makes the fingerprint diverge is its own investigation.

Also out of scope: the two dedup passes writing to one `duplicate_of` column by different
criteria, so that running either alone unmarks the other's work. Worth knowing before touching
them (`cmd/reindex/main.go` runs role → aggregator, in that order, always).
