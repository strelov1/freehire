## Context

`cmd/harvest-boards <provider> <seed.json>` already validates and adds boards; see
proposal.md for why a converter is needed upstream of it. The seed shape it accepts
(`cmd/harvest-boards/seed.go`) is a JSON array of either bare strings or `{"board",
"company", "expect_id"}` objects — this change only ever produces the `{"board",
"company"}` form (no ATS-native posting id is available from a `name,slug,url` inventory).

`internal/ingest/atsboard.Recognize(rawURL string) (source, board, canonical string, ok
bool)` already owns every URL-shape-to-provider mapping freehire has, is network-free, and
is exactly what a `url` column needs to become a `(provider, board)` pair. `source` is
already guaranteed to be the provider key the catalogue and `cmd/harvest-boards` both use
(see `internal/ingest/atsboard/AGENTS.md`, "the source MUST be the provider key the
catalogue uses"), so no re-mapping is needed between what `Recognize` returns and what
`cmd/harvest-boards`'s positional `<provider>` argument expects.

## Goals / Non-Goals

**Goals:**
- Turn any `name,slug,url` CSV (the ats-scrapers `ats-companies/*.csv` shape) into
  provider-partitioned seed files `cmd/harvest-boards` can consume unmodified.
- Reuse `atsboard.Recognize` as the single source of truth for URL → (provider, board);
  add no parallel parsing logic for any ATS.

**Non-Goals:**
- Does not call `cmd/harvest-boards`, probe any platform, or touch the `boards` table —
  that remains a separate, explicit step run by an operator after reviewing this tool's
  output.
- Does not handle inventory formats other than the three-column `name,slug,url` CSV (no
  Parquet, no the ats-scrapers Python package's own `Job` schema).
- Does not attempt to resolve vanity-domain or custom-domain ATS boards (Taleo,
  SuccessFactors, Oracle on their own domains) — `atsboard` documents this as out of its
  scope, and duplicating a network fetch (as `internal/ingest/boardresolve` does) to close
  that gap is explicitly left for a later change if the unresolved-row count ever justifies
  it.

## Decisions

**CLI shape:** `go run ./cmd/seed-from-inventory -in <csv> -out <dir>`. `-in` is a single
CSV file (one provider inventory at a time, matching how the source repo ships them — one
file per ATS); `-out` is a directory the tool writes into. This mirrors the argument style
of `cmd/harvest-boards` and `cmd/add-board` (flags, not a config file) rather than
introducing a new CLI convention.

**Output filenames:** one file per distinct resolved provider, named `<provider>.json` in
`-out` (e.g. `-out=/tmp/seeds` with a CSV containing both greenhouse and lever rows produces
`/tmp/seeds/greenhouse.json` and `/tmp/seeds/lever.json`). A single input file can therefore
legitimately fan out to several providers — expected for a broad inventory, a no-op split
for a single-ATS inventory like `ats-companies/workday.csv`.

**Deduplication key:** the exact `(provider, board)` pair `Recognize` returns, compared
byte-for-byte. `Recognize` already normalizes case and structure per platform (see
`atsboard`'s extraction modes), so a second fold here would either be redundant or silently
diverge from what the catalogue's own dedupe key does — out of scope for this tool, whose
job is producing candidates, not judging them. `cmd/harvest-boards` performs the real
duplicate check against the live catalogue on its own next run.

**Company label:** the CSV's `name` column, verbatim, becomes `company` in the seed entry.
`cmd/harvest-boards`'s `chooseCompany` (`cmd/harvest-boards/seed.go`) already prefers the
platform's own reported name over a seed-supplied one wherever the prober returns one, so a
CSV's occasionally stale or reformatted company name is a fallback label only, never the
final one.

**Malformed input handling:** structural failures (unreadable file, wrong column count, a
missing required header) abort the whole run before writing anything — a half-written
seed set is worse than no output, since an operator diffing "boards found" against "boards
expected" would misread a truncated run as a small inventory. A row-level failure to
recognize a URL is not structural and is merely skipped and counted (see spec).

**Summary output:** the tool prints a one-line-per-provider count of boards written plus a
total unrecognized-row count to stdout on completion — the same "report what happened"
posture `cmd/harvest-boards` and `cmd/merge-companies` already use, so an operator can see
at a glance whether an inventory was mostly recognized or mostly not before feeding a
provider's file onward.

## Risks / Trade-offs

- **A vanity-domain-heavy inventory yields a low recognition rate.** Expected and
  documented (`atsboard`'s own limitation) rather than a bug in this tool; the summary
  output surfaces it so an operator isn't left guessing why a provider's seed file is
  smaller than the input CSV. Closing this gap (a `boardresolve`-style page fetch) is
  explicitly deferred, since it would also cost network access this tool otherwise avoids.
- **A future `atsboard` change alters recognition results.** Since this tool calls
  `Recognize` directly rather than vendoring any logic, its output always tracks the
  current recognizer — a improvement (or a regression) there is inherited automatically,
  with no separate maintenance burden here.
