## Context

See proposal.md - Why/What Changes for motivation and scope. Two behaviors matter here:

- `internal/dict/skilltag.Parse(text string, opts ...Option) []string` already takes free
  text and returns canonical skill slugs — a raw `enrichment.skills` phrase can be fed to it
  directly to test whether the dictionary would have caught it.
- `internal/dict/classify.Parse(title string) Classification` already takes a title and
  returns `{Seniority, Category}` — the same function the ingest write path uses, so calling
  it again against a stored title reproduces today's dictionary answer with no extra state.

`jobs.enrichment` is a JSONB blob; the LLM's own `seniority`/`category` guess is stored there
independently of the `jobs.seniority`/`jobs.category` columns the dictionary computed at
write time (`internal/ai/enrich/AGENTS.md`: `jobview.FromRow` overlays the dictionary value
onto the *served* shape, it never rewrites the stored JSONB). Both facts are already sitting
in the table; nothing here changes what gets stored, only what gets read back and compared.

The existing one-off `cmd/backfill-requirements` (and siblings `backfill-slug-folded`,
`backfill-clearance`) establish the id-range-chunked, resumable read shape this change
reuses: `MIN/MAX(id)` bounds query, a chunk query bounded by both an id range and a row
`LIMIT`, resuming from the last id seen when a chunk comes back full, paced with a sleep
between chunks, knobs read through `worker.EnvInt64` before touching the database.

## Goals / Non-Goals

**Goals:**
- Turn accumulated enrichment data into a ranked, human-readable list of dictionary gaps for
  `skilltag` and for `classify`, cheaply enough to re-run whenever a curator wants a fresh
  read.
- Reuse the established chunked-read shape so the two new commands behave like every other
  one-off report/backfill tool operationally (env knobs, resumability, pacing).

**Non-Goals:**
- Deciding *which* candidates are worth adding — that is a curated, reviewed edit to
  `dictionaries.go`/`labels.go`/`descriptions.tsv` or the `classify` title table, done by a
  person reading the report.
- Any semantic clustering of skill phrases beyond trivial case/punctuation/whitespace
  variants (e.g. recognizing "React.js" and "reactjs" as the same idea is a human judgment
  call, consistent with every other dict-only design in this repo: never guess).
- Scheduling, alerting, or a Prometheus metric — this is a manual curator tool, not a
  monitored worker (see `cmd/merge-companies`, `cmd/linkedin-auth` for the same shape).
- Touching `cmd/enrich`, `cmd/ingest`, or any serving path.

## Decisions

**One pure package, `internal/job/dictgap`, holds both candidate-ranking functions.**
Both take already-fetched Go values (no `*pgxpool.Pool`, no context) and return a ranked
slice, so they are unit-testable with plain table tests and require no database in CI. They
live together because they are the same *kind* of function (enrichment fact in, ranked
dictionary-gap candidates out) even though they serve two different commands — splitting
them into two packages would buy no isolation, since neither imports the other.
Alternative considered: put the logic directly in each `cmd/`'s `main.go`. Rejected because
`cmd/backfill-requirements` already shows the pattern of keeping the derivation logic
testable independent of the DB loop (`derive` there isn't in its own package, but it *is*
a pure function covered by `main_test.go`; `dictgap` follows the same spirit but as an
importable package since two commands need to share nothing — each has one function).

**`SkillGapCandidates` takes pre-aggregated frequencies, not raw rows.**
Signature: `SkillGapCandidates(counts map[string]int) []SkillGapCandidate`. The command layer
owns the chunked scan and folds every `enrichment.skills` element into a
`map[string]int` (keyed on the phrase exactly as enrichment stored it) as it pages through
`jobs`; `dictgap` never sees a row or a chunk boundary, only the final tally. This keeps the
memory/pacing concerns (chunk size, resume id) entirely in `cmd/report-skill-gaps`, where the
rest of that concern already lives, and keeps the package trivial to test with a literal map.
Collapsing case/punctuation/whitespace variants happens inside `SkillGapCandidates` (it
normalizes each key, sums colliding buckets, and keeps one display form — the most frequent
original spelling) rather than in the command, so the collapsing rule has one home and one
test suite.

**`ClassifyDriftCandidates` takes one row per distinct title already aggregated by the SQL layer.**
Signature: `ClassifyDriftCandidates(rows []TitleClassification) DriftReport` where each
`TitleClassification` is `{Title string; Count int; EnrichmentSeniority, EnrichmentCategory string}`
— note it does NOT take the stored `jobs.seniority`/`jobs.category` columns as input. The
function recomputes `classify.Parse(row.Title)` itself rather than trusting the stored
column. Two reasons: (1) the whole point is to catch drift the *current* dictionary has from
the model, and the stored column reflects whatever dictionary version was live when that job
was last derived — recomputing is the only way a report taken today reflects today's
dictionary; (2) it removes one input, simplifying both the SQL (`GROUP BY title` picks any
one `enrichment` blob per title via `MIN`/an arbitrary aggregate, since seniority/category
recomputation depends only on `title`) and the test fixtures.
Alternative considered: compare against the stored `jobs.seniority`/`jobs.category` instead
of recomputing. Rejected — that only ever shows drift the dictionary already had baked in at
ingest time, which `cmd/backfill-derive` already keeps current; it would never surface a gap
introduced by an *upcoming* dictionary change a curator is testing locally before committing.

**The SQL aggregates by title server-side, not in Go.** `ListTitlesForClassifyDrift` groups
by `title` and returns one row per distinct title with a count and one representative
enrichment seniority/category pair (`MIN` is enough — the requirement only needs *a*
disagreement signal per title, not a distribution across postings that share a title but
disagree with each other, which the drift-per-title framing in the spec doesn't ask for).
Grouping in SQL rather than pulling every row and grouping in Go bounds each chunk's memory
by its distinct-title count, not its raw row count, and avoids materializing millions of
individual job rows for a report that only needs title-level counts. This query carries no
row `LIMIT` (unlike `ListJobSkillsForGapReport`): an aggregated row has no single id to
resume a truncated chunk from, so the id-range width alone — the caller's chunk-size knob —
is what bounds one statement's cost here.

**Each command gets its own bounds + chunk query pair**, matching the existing
`RequirementsDerivedBackfillBounds`/`CompanySlugFoldedBackfillBounds`/
`DuplicateMarkerOwnerBackfillBounds` precedent — every existing chunked one-off tool defines
its own identical-shaped `MIN/MAX(id)` bounds query rather than sharing one, and the report
tools follow that convention rather than introducing the first shared helper.

**Output is plain stdout, not a file.** Both commands print a simple tab-separated table
(count, then the candidate fields) to stdout, ordered by count descending, capped at a
`-top`/`TOP_N`-style env-configurable N (default 200, matching the proposal). A curator
redirects to a file themselves (`go run ./cmd/report-skill-gaps > gaps.tsv`) if they want one
— piping is a shell-native concern, not something the command needs to own.

## Risks / Trade-offs

- [Risk] `enrichment.skills` values are LLM-freeform text, so most unresolved phrases will be
  noise (job-specific phrasing, non-technical soft skills, junk) rather than real dictionary
  gaps. → Mitigation: none needed in the tool — this is explicitly a curator-filtered report,
  not an auto-apply pipeline; the frequency ranking already pushes real signal (a widely
  repeated technical term) above one-off noise.
- [Risk] Recomputing `classify.Parse` per distinct title, potentially over a very large number
  of distinct titles (the catalogue has millions of postings but titles repeat heavily), could
  make the chunk query's `GROUP BY` expensive on a huge id range. → Mitigation: same chunk-size
  knob pattern as `backfill-requirements` (`REPORT_CLASSIFY_DRIFT_CHUNK`), so an operator narrows
  the id span per statement if a chunk proves slow; report tools are run by hand and can be
  stopped/resumed exactly like the backfills they copy the shape from.
- [Trade-off] Two near-identical chunked-read command skeletons instead of one shared runner.
  Accepted deliberately — the codebase's own convention (three separate near-identical bounds
  queries already exist) treats a shared chunked-scan abstraction as premature; each tool stays
  independently readable and independently disposable.

## Open Questions

None — scope, output shape, and the recompute-vs-stored-column decision are all settled above.
