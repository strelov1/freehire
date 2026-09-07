## 1. jobderive: structured is_tech signal

- [x] 1.1 Add `IsTechHint bool` to `jobderive.Input` (doc comment following the existing structured-signal contract) and thread it into `deriveIsTech` as a new parameter checked before `TechEvidence` and the non-tech detector.
- [x] 1.2 Unit tests in `internal/job/jobderive`: hint yields `true` when category and title dictionaries resolve nothing; hint yields `true` even when the non-tech title detector would otherwise fire; hint absent falls back to the existing precedence unchanged (regression coverage for the current scenarios).

## 2. Ingest wiring: Profession asserts the signal

- [x] 2.1 Add `IsTechHint bool` to `sources.Job` (`internal/ingest/sources/source.go`), documented like the other structured fields.
- [x] 2.2 Set `IsTechHint: true` on the `Job` returned by `profession.detail()`, with a comment cross-referencing the existing `professionITBoards` rationale (evidence already measured: zero non-tech rejections over the sampled postings from these two boards).
- [x] 2.3 Wire `j.IsTechHint` into the `jobderive.Input{...}` literal in `internal/ingest/pipeline/pipeline.go`'s `normalizeJob`.
- [x] 2.4 Unit test on `profession.detail()` (or the existing adapter test file): a successfully parsed posting carries `IsTechHint = true`.
- [x] 2.5 Unit/integration test on `normalizeJob` (or the pipeline's existing coverage) confirming a Profession job's `IsTechHint` reaches `jobderive.Input.IsTechHint` and therefore `is_tech = true` end to end, even when the title matches neither dictionary.

## 3. Search: confirmed-technical carve-out

- [x] 3.1 Update `search.CategoryUnresolved` (`internal/search/search/document.go`) to return `false` when `j.IsTech` is valid and `true`, before falling through to the existing enrichment-category check. Update its doc comment to state the carve-out and why.
- [x] 3.2 Unit tests in `internal/search/search`: category empty + enrichment empty/`"other"` + `is_tech` unknown/false → still excluded (regression); same inputs with `is_tech = true` → included; a resolved category still includes regardless of `is_tech` (regression).
- [x] 3.3 Confirm (by reading, not changing) that both call sites — `cmd/search-drain/indexer.go:110` and `cmd/reindex/main.go:514` — need no code change since they call the shared function; update their surrounding comments only if they now read as inaccurate given the carve-out.

## 4. Backfill for already-stored rows

- [x] 4.1 Add a `BackfillProfessionITTech :execrows` query to `internal/platform/db/queries/jobs.sql`: `UPDATE jobs SET is_tech = true WHERE source = 'profession' AND (external_id LIKE 'itdev:%' OR external_id LIKE 'itops:%') AND is_tech IS DISTINCT FROM true`, with a comment explaining the `external_id` prefix convention it relies on (`internal/ingest/pipeline`'s `externalid.Namespace`) and why this is a direct column set rather than a `jobderive` re-derivation (see design.md Decision 3).
- [x] 4.2 Run `make sqlc` to regenerate; confirm the diff is limited to the new query.
- [x] 4.3 Add `cmd/backfill-profession-it-tech/main.go`: a `DATABASE_URL`-only one-off worker calling the new query once and logging the row count affected, following the shape of `cmd/backfill-clearance` (idempotent, safe to re-run, no chunking needed given the bounded row count for two boards).
- [x] 4.4 Add a short test (or reuse the `internal/platform/db` integration test pattern) verifying the query only touches Profession `itdev`/`itops` rows and is a no-op on a second run.

## 5. Documentation

- [x] 5.1 Add `backfill-profession-it-tech` to the root `AGENTS.md` "Worker gotchas" list, following the existing one-off backfill entries' shape (what it does, when to run it once, the required follow-up `make reindex`).
- [x] 5.2 Confirm `internal/ingest/sources/AGENTS.md`'s existing Profession section still reads accurately given the new `IsTechHint` assertion; extend its existing note if it now undersells what board membership implies.

## 6. Verification

- [x] 6.1 `gofmt -l .`, `go vet ./...`, `go test ./...` clean.
- [x] 6.2 `go vet -tags=integration ./...` clean.
- [x] 6.3 `openspec validate --strict` for this change passes.

## 7. Independent review follow-ups

Found by an independent code review of the finished diff; fixed test-first before merge.

- [x] 7.1 `cmd/backfill-derive`'s `deriveRow` re-derives `is_tech` via a bare `jobderive.Input` with no `IsTechHint`, so its routine ~15h pass would silently rewrite a fixed Profession itdev/itops row back to unknown, undoing the fix. Added `sources.ProfessionConfirmsTech(source, externalID)` (recovers the board from the stored `external_id` namespace prefix) and wired it into `deriveRow`, with a regression test (`TestDeriveRow_PreservesProfessionITTechHint`) plus the negative case (`TestDeriveRow_DoesNotConfuseAnotherSourceForProfessionsITBoards`).
- [x] 7.2 `BackfillProfessionITBoardTech`'s SQL hardcoded `LIKE 'itdev:%' OR LIKE 'itops:%'` as a literal instead of the existing `externalid.BoardPattern` convention (`ExistingExternalIDsByBoard`/`BackfillBoardCompany`), and was case-sensitive against a board whose stored casing isn't guaranteed. Added `sources.ProfessionITBoardNames()`, switched the query to `external_id ILIKE ANY(sqlc.arg(board_patterns)::text[])` built from that list via `externalid.BoardPattern`, and added a mixed-case regression row to the integration test.
- [x] 7.3 Fixed a stale comment in `internal/ingest/linkimport.go`'s `index` (a third `search.CategoryUnresolved` caller) that didn't mention the `is_tech` carve-out.
- [x] 7.4 Fixed `TestUpsertParams_CheapWriteMatchKeyCoversEveryColumnItWrites`'s failure message, which still listed only the pre-`is_tech` match-key columns.
