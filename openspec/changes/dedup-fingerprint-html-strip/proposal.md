## Why

`role_fingerprint` keys the role cluster that collapses reposts and per-city variants of one job into a single card. It is a hash of `company_slug` + city-stripped title + normalized description, but the text normalization only lowercases and collapses whitespace — so any HTML markup difference splits the fingerprint. The same posting re-emitted with a stray `<br>`, a different entity encoding (`&amp;` vs `&`), or a different wrapper structure (e.g. the same job surfaced by a second source) resolves to a different fingerprint and shows as a duplicate card. Measured on prod (2026-07-21): **~29,000 duplicate fingerprints across ~18,000 `(company, title)` groups differ ONLY by markup** and would collapse if the hash compared visible text instead of raw markup.

## What Changes

- The `role_fingerprint` text normalization strips HTML tags and decodes HTML entities before hashing, so the fingerprint compares the **visible text** of a posting rather than its markup. Two postings whose rendered title and description are identical now collapse even if their HTML differs.
- Normalization strips HTML tags (a `<[^>]*>` regex, tags replaced by a space to preserve word boundaries) then decodes HTML entities (`html.UnescapeString`), applied to both the title and the description via the shared `normalizeRoleText`, before the existing lowercase + whitespace fold. No new dependency (`html` is stdlib).
- A one-shot `cmd/backfill-role-fingerprint` run followed by `make reindex` re-applies the new fingerprint to the live catalogue, collapsing the newly-clustered reposts and unioning their geography onto each canon.
- **Out of scope (separate future change):** fuzzy / Jaccard matching of near-identical-but-unequal descriptions (e.g. an Austrian Kollektivvertrag legal clause naming one city that another city's posting lacks). This change keeps the fingerprint an exact-match on normalized visible text; it does not introduce a similarity threshold.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `ingest-content-dedup`: the role-fingerprint normalization requirement is strengthened — the title and description are reduced to visible text (HTML tags stripped, entities decoded) before the lowercase/whitespace fold, so postings that differ only by markup share a fingerprint. The over-merge guard is unchanged: the description stays in the key, so postings with different visible text still stay separate.

## Impact

- **Code:** `internal/jobhash/rolefingerprint.go` (`normalizeRoleText`) — the only behavioral change. `cmd/backfill-role-fingerprint` and `cmd/ingest`/`UpsertJob` pick it up automatically because they call `jobhash.RoleFingerprint`.
- **Data:** every job's `role_fingerprint` is recomputed. All markup-only duplicate clusters merge; downstream `duplicate_of` / `job_cluster_copies` and repost counts shift accordingly. Requires the backfill + reindex rollout (deploy code → `cmd/backfill-role-fingerprint` → `make reindex`, own flock, low-traffic window) — until the backfill runs, existing rows keep their old fingerprint and only re-cluster as they re-crawl.
- **Dependencies:** no new dependencies — the strip uses only the standard library (`regexp`, `html`). (The ingest-time sanitizer that guarantees well-formed stored HTML is `bluemonday`, unchanged by this change.)
- **Risk:** merging direction only (never splits a currently-clustered role). Over-merge is bounded to postings with byte-identical visible text, which are genuine duplicates.
