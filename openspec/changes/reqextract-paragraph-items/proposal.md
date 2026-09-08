## Why

`reqextract.Derive` only reads requirement items out of a `<ul>`/`<ol>` list under a
recognized heading; a section stated as a run of `<p>` items closes as prose before any
list is found and yields nothing. This is not a hypothetical shape: every one of 683 real
`Требования` headings sampled from `tbank.ru` is followed by a run of `<p>` paragraphs,
never a list, so the Russian heading vocabulary added in `reqextract-russian-headings`
matches the heading on that source today and still extracts nothing from it.

## What Changes

- `Derive`'s walk gains a deferred buffer (`pending`/`pendingHasProse`/`flushPending`):
  a text block too long to be a heading candidate is buffered instead of closing its
  section immediately, since it may be the section's own first stated item rather than
  prose explaining the section away — the two cannot be told apart from one paragraph
  alone. The buffer is discarded (and the section closed) if a list is found afterward
  (real prose before a list means the list belongs to what follows, not this heading);
  it is committed as the section's own items — but only when there are two or more of
  them — when a real heading transition, a table, or the document's end resolves it. A
  short unrecognized inline line is buffered the same way but never marks the buffer as
  prose, and is silently cleared the moment a list appears.
- `internal/job/reqextract/AGENTS.md` updated: the "How it works" section describes the
  new buffering mechanism, the "Limitations" bullet documenting this exact gap is
  removed, and the "Measuring coverage" figures (28.0% / 29.3%) are flagged as
  predating this fix.

## Capabilities

### Modified Capabilities

- `posting-requirements-derivation`: a requirements section stated as a run of `<p>`
  items (no enclosing list) is now derived, under the same heading vocabulary and
  priority rules as the list-shaped case, instead of yielding nothing.

## Impact

- `internal/job/reqextract/reqextract.go` — `Derive`'s walk.
- `internal/job/reqextract/reqextract_test.go` — new boundary-case coverage.
- `internal/job/reqextract/AGENTS.md` — mechanism + limitation + coverage-figure notes.
- No schema change, no new worker: the existing ingest write path and the existing
  `backfill-requirements` one-off both call `Derive` unchanged, so a re-run of the
  backfill (not part of this change) is what would reach the pre-existing catalogue.
