## Why

`internal/dict/classify` resolves a job's `category` by whole-word match, and
`search.CategoryUnresolved` excludes a job from the Meilisearch index when no
category resolves and `is_tech` isn't otherwise confirmed. German compounds
IT job titles into a single word with no separator (`Systemadministrator`,
`Fachinformatiker`), so the word-boundary matcher can never reach them the way
it reaches a hyphenated Russian title or a spaced English one — the dictionary
has no fused-compound alias for them at all, unlike `entwickler`, which already
enumerates its fused/hyphen/space spellings. A DB check on the local dev
catalogue found ~400 open postings from `arbeitsagentur` alone — plain IT
administrator, technician and tester titles — silently excluded from search
for this reason.

## What Changes

- Add fused-compound German aliases to the category title dictionary for the
  IT administrator/technician/tester/developer family that has no word for
  them today: `Fachinformatiker` (+ `für Systemintegration` /
  `Anwendungsentwicklung` qualifiers), `Systemadministrator`,
  `Netzwerkadministrator`, `Datenbankadministrator`, `Netzwerktechniker`,
  `Softwaretester`, `Anwendungsentwickler` — each resolves bare, the same
  precedent the existing `entwickler`/Russian clusters set (a hyphen or space
  before the compound is itself a word boundary, so the bare alias already
  reaches an `IT-`/`IT `-prefixed spelling without a separate entry).
- Add `IT Systemtechniker` / `IT-Systemtechniker` and `IT Systemelektroniker` /
  `IT-Systemelektroniker` as **qualified-only** aliases (no bare
  `Systemtechniker`/`Systemelektroniker` fallback): the local DB check found
  the bare German words also naming non-IT disciplines ("Systemtechniker
  Elektrotechnik", "Systemtechniker Sicherheitstechnik"), the same
  cross-domain trap `it-title-coverage` already documents for bare "Systems
  Engineer" lookalikes — an unqualified title stays unresolved rather than
  guessed.
- Each new alias resolves to the same category its already-covered
  spaced/English counterpart resolves to (e.g. `Systemadministrator` joins
  `system administrator` → `devops`), so no new category is introduced.
- **Not in scope**: `SPS-Programmierer` (PLC/industrial-controller
  programming) is already deliberately excluded from `software_engineering`
  in the dictionary, on the same reasoning as "CNC Programmer" — this change
  does not touch that.

## Capabilities

### Modified Capabilities
- `it-title-coverage`: adds a requirement that the German fused-compound IT
  titles above resolve to a category, matching the coverage doctrine this spec
  already states for the Russian and English title families it documents.

## Impact

- Code: `internal/dict/classify/dictionaries.go` (new alias entries),
  `internal/dict/classify/*_test.go` (regression coverage).
- No column, index, or API shape changes — a resolved `category` already flows
  through `is_tech` derivation (tech-classification's "Recognized tech
  category yields true") and into `search.CategoryUnresolved` with no further
  code change.
- Operational, out of scope for this change: postings ingested before this
  dictionary update carry a stale empty `category`/`is_tech` (this also
  affects some already-covered titles, e.g. Russian "Go-разработчик", found
  stale in the same DB check). Reaching those existing rows needs a
  `cmd/backfill-derive` run followed by a full `make reindex`, per the
  existing pattern documented in `AGENTS.md` for prior dictionary-coverage
  backfills (e.g. `backfill-profession-it-tech`) — a production operations
  step, not a code change.
