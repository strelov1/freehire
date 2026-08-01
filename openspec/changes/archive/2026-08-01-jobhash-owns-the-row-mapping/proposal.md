## Why

Two backfill commands carried a byte-identical `hashParams(j db.Job, description string)
db.UpsertJobParams` — 19 fields listed by hand, with the same doc comment ("the exact indexed
fields `jobhash.Of` fingerprints").

The cost is not the duplication itself but what it makes cheap. `Of` names the fields the
fingerprint covers; the mapping names where each comes from on a stored row. They are **one
decision in two halves**, and the halves lived in three files. Add a field to `Of` and the copies
keep computing the old fingerprint — so a backfill rewrites rows carrying a hash the ingest path
will not reproduce, and the next crawl of each reports `changed` once for nothing.

## What Changes

- `jobhash.OfRow(j db.Job, description string) string` — the mapping moves beside the hash it
  feeds, which is the only place a reader can see both halves at once. `jobhash` already imports
  `db`, so this adds no dependency.
- Both `hashParams` copies are deleted; the two call sites and four test call sites go through
  `OfRow`.
- **A test replaces the reviewer.** `TestOfRow_CarriesEveryFieldTheHashReads` mutates one field
  of a fully-populated row at a time and asserts the fingerprint moves. A field the mapping drops
  cannot move it, so the case fails naming that field. Verified by removing `Seniority` from the
  mapping: the test fails with `changing seniority left the fingerprint unchanged`.
- Deliberately NOT routed through `job.FromRow → Fields → UpsertParams`: `FromRow` decodes the
  enrichment JSONB and returns a per-row error, which is real work and a new failure path inside
  two throwaway backfill loops.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none) — no requirement-level behaviour change. The fingerprint is byte-identical before and
after; `tasks.md` is the real artifact and the change archives with `--skip-specs`.

## Impact

- `internal/jobhash/jobhash.go` (+ its test), `cmd/backfill-descriptions`, `cmd/backfill-justjoin`.
- **Left alone, on purpose:** `Of` omits `Cities`, `EnglishLevel` and `IsTech`, which the search
  document does carry. That is a real gap the finding surfaced as evidence, but closing it changes
  every stored hash — every job would report `changed` once on its next crawl and force a full
  re-push. That is an operational decision with a cost, not a cleanup to fold into a refactor.
