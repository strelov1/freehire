# Job-hash conventions

## Scope
Three complementary fingerprints/signatures over a job: `Of` — the CHANGE signal behind
`jobs.content_hash`, deciding whether a re-ingest needs a re-push to the live search index;
`RoleFingerprint` — the IDENTITY signal that clusters reposts of one role;
`RoleKey` + `DescriptionSignature`/`DescriptionSimilarity` — CROSS-SOURCE role matching.
Pure functions over `db.UpsertJobParams` / `db.Job`; no I/O.

## Always true
- **`Of` and `RoleFingerprint` are opposites on purpose** (rolefingerprint.go:19-22). `Of`
  (jobhash.go:23) includes `posted_at`, so a repost with a bumped date is "changed" and
  re-indexed. `RoleFingerprint` (rolefingerprint.go:23) excludes every volatile field
  (`posted_at`, `url`, `public_slug`, `source`, `external_id`, `location`), so a repost
  under a new external_id collapses onto the same role. **Never cluster reposts by
  `content_hash`** — it is built to move on exactly the changes clustering must ignore.
- **`Of` hashes exactly the fields that end up in the Meilisearch document** (see
  `internal/search.FromJob`), excluding the identity columns (`source`, `external_id`) —
  constant for a row, not searchable content. Fields are record-separated (`\x1e`, slices
  nested `\x1f`) so content cannot shift across a field boundary and collide.
- **`OfRow` must mirror `Of`'s field list exactly** (jobhash.go:55-64). It is the stored-row
  half of the same decision — a backfill rewriting rows holds a `db.Job`, not params. Kept
  apart, a field added to `Of` leaves the mapping behind and the next crawl reports every
  backfilled row `changed` once for nothing. Not hypothetical: the mapping existed twice,
  byte-identical, in two backfill commands; `TestOfRow_CarriesEveryFieldTheHashReads` is
  what fails now instead.
- **`RoleKey` returns "" for a blank title, and that means "no key"** (rolekey.go:27-29).
  A caller MUST treat it as unmatchable — every blank title would otherwise match every
  other one. It is company+title only, deliberately weaker than `RoleFingerprint`
  (rolekey.go:9-19): across sources, descriptions are truncated and rewritten, so a
  description hash would be a coincidence rather than a rule. Parentheses are unwrapped,
  not dropped — measured on prod, keeping them inflated the cross-check's absent count by
  13% ("Data Engineer (Semi Senior)" vs "Data Engineer Semi Senior").
- **Both text paths share one normalization** (`normalizeRoleText`, rolefingerprint.go:50):
  strip HTML tags (replaced with a space, so block boundaries survive), decode entities,
  lower-case, collapse whitespace. `stripTrailingClause` (rolefingerprint.go:69) removes a
  trailing location/qualifier clause so per-city variants cluster — but never below two
  words, so a too-generic token cannot become a cluster key.
- **`DescriptionSimilarity` is only sound INSIDE a company+title bucket** (descsimilarity.go:
  30-41). It is the Jaccard index of distinct meaningful words (>2 chars) in the visible
  description text — per-city variants of one role score ≥0.95, distinct roles in the same
  bucket ≤0.5, but two boilerplate-heavy postings of unrelated roles at one company can
  score 0.98. An EMPTY signature is never similar to anything, itself included — Jaccard of
  two empty sets is undefined, and treating it as 1 would merge every posting whose
  description failed to normalize.

## Consumers
- `internal/job` — the ingest write path: `Of` decides changed-vs-unchanged on re-ingest,
  `RoleFingerprint` feeds repost clustering.
- `internal/ghost` (crosscheck.go) — `RoleKey` matches aggregator postings against the
  employer's own board titles.
- `cmd/reindex` (fuzzy.go) — description-signature similarity for fuzzy role matching.
- `cmd/hydrate-adzuna-description`, `cmd/backfill-descriptions`, `cmd/backfill-echojobs` —
  `OfRow`/`RoleKey` for rewriting stored rows without breaking the change signal.
