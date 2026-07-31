## Why

A subscription digest went out listing the same vacancy thirty times — "Senior Full Stack
Engineer ID78855 — AgileEngine", once per LATAM city, all thirty in one message.

The rows were genuine duplicates and the catalogue knows how to collapse them: their titles
and descriptions are identical, so they share a `role_fingerprint`, and
`RecomputeRoleDuplicatesForCompany` would have marked twenty-nine of them non-canonical. But
that recompute runs only inside `cmd/reindex`, every three hours, while `cmd/ingest` pushes
each freshly written row straight into the live index and `cmd/notify` reads that index every
five minutes. Between the crawl and the next reindex, every copy is a searchable vacancy of
its own — to the digest, to the jobs list, and to the company's open count.

The existing spec already requires that reposts stay out of search and out of enrichment. It
never says *when* a row is judged a repost, and the implementation satisfied the letter of it:
`duplicate_of` was still NULL at index time, so nothing was excluded. This change closes that
window by asking the question at write time, the way the URL-import path already does.

## What Changes

- `cmd/ingest` asks, inside the same transaction as the upsert, whether the row it just
  inserted has an older canonical sibling in its role cluster, and marks it `duplicate_of`
  when so. A marked row is neither pushed to the live index nor enqueued for enrichment.
- The lookup is gated to genuinely new rows (`Inserted`). A posting that becomes a duplicate
  later — an edit making its title and description match a sibling's — stays the batch
  recompute's job, as does releasing a row whose canon closes.
- The canon lookup moves from `internal/linkimport` into a new `internal/jobdedup`, shared by
  both write paths. Its rule — the canon must be OLDER than the row just written, matching the
  `min(id)` the batch would pick — is what keeps the synchronous answer from being inverted by
  the next reindex, and it must not exist in two copies.
- `internal/linkimport` keeps its own gate (only a URL-keyed generic import can shadow a
  crawled posting) at the call site, since it describes that caller rather than the rule.

Unchanged: the batch recompute, the canon choice (`min(id)`), the geography union at reindex,
and the fact that duplicate rows are kept rather than dropped.

## Capabilities

### Modified Capabilities
- `ingest-content-dedup`: the repost marker is assigned when a posting is first written, not
  only by the periodic recompute, so a per-city fan-out never reaches search, the subscription
  digests, or the enrichment queue as separate vacancies.
