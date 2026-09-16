## 1. Migration

- [ ] 1.1 Add `migrations/0165_hn_posts.sql` creating `hn_posts` per design.md's shape (PK
      `id` = the comment's own HN item id, `thread_id`, `text`, `posted_at`, `fetched_at`,
      `attempts`, `claimed_at`, `failed_at`, `last_error`, `extracted_at`).
- [ ] 1.2 Add `internal/platform/db/queries/hn_posts.sql`: `InsertHNPost` (idempotent,
      `ON CONFLICT (id) DO NOTHING`), `ClaimPendingHNPosts` (claim a batch, mirroring
      telegram_posts' claim query), `MarkHNPostExtracted`, `MarkHNPostFailed`. Run
      `make sqlc` and commit the generated diff.

## 2. Crawl half: fetch the thread, store comments

- [ ] 2.1 RED: write a test for `hiringThreadIDs` against a fixture Algolia
      `search_by_date` response (a fake `httpClient`/fixture, not a live call) — asserts it
      picks the "Who is hiring?" story and not a sibling thread from the same author (same
      title-filter care `scripts/harvest_boards.py`'s `select_hiring_threads` already takes
      — port the same filtering logic, not just the shape).
- [ ] 2.2 GREEN: implement `internal/ingest/hackernews/fetch.go`'s `hiringThreadIDs`.
- [ ] 2.3 RED: write a test for `commentsFor` against a fixture `items/<id>` response —
      asserts it returns each child comment's id, text, and posted-at, skipping any with no
      text (a deleted/flagged comment).
- [ ] 2.4 GREEN: implement `commentsFor`.
- [ ] 2.5 RED: write a test for the prefilter (`internal/ingest/hackernews/prefilter.go`) —
      a comment that is plainly not a vacancy (e.g. "Congrats on launching!") is rejected; a
      "Company | Role | Remote" shaped comment passes.
- [ ] 2.6 GREEN: implement the prefilter.
- [ ] 2.7 RED: write a test for `CrawlRunner.Run` (`internal/ingest/hackernews/crawl.go`)
      against fake `Fetcher`/`Store` — asserts it discovers the thread, fetches comments,
      prefilters, and calls `Store.Insert` once per surviving comment; a fetch failure is
      counted and does not abort the run.
- [ ] 2.8 GREEN: implement `CrawlRunner`.
- [ ] 2.9 Simplify pass over 2.1-2.8; re-run tests green.

## 3. ATS-link recognition and the board-contribution branch

- [ ] 3.1 RED: write a test for `recognizedBoard` (`internal/ingest/hackernews/atslink.go`)
      — a comment text containing a Greenhouse/Ashby/etc. URL resolves via
      `atsboard.Recognize`; a comment with only a company's own custom domain does not; a
      comment with two recognized links returns the first one deterministically.
- [ ] 3.2 GREEN: implement `recognizedBoard`.
- [ ] 3.3 RED: write a test for the company-name heuristic in
      `internal/ingest/hackernews/contribute.go` (port of `scripts/harvest_boards.py`'s
      `hn_company_name()`/`_slug_title()` to Go) — leading "Company | Role | ..." token
      extracted; a prose/role-looking leader falls back to a title-cased board slug; cover
      the same reject-prefix/reject-role-word cases the Python version already tests
      (`scripts/test_harvest_boards.py`'s `test_hn_company_name_*` — read them for the exact
      cases to port, do not re-derive from scratch).
- [ ] 3.4 GREEN: implement the company-name heuristic.
- [ ] 3.5 RED: write a test for `contribute()` wiring against a fake
      `boardcatalog.Repository` — asserts it calls `Insert` with `Surface: "cli"`,
      `SubmittedBy: nil`, the recognized `(provider, board)`, and the derived company name,
      at `StatusPending`.
- [ ] 3.6 GREEN: implement `contribute()`.
- [ ] 3.7 Simplify pass over 3.1-3.6; re-run tests green.

## 4. LLM extraction (the no-link branch)

- [ ] 4.1 Define `Extraction`/`ExtractedJob`/`(*Extraction).Validate()` in
      `internal/ingest/hackernews/extraction.go` (own copy, per design.md's Decision — do
      NOT import `internal/ingest/telegram`'s types).
- [ ] 4.2 RED: write a test for `Validate()` — a job missing `Title` or `Description` is
      dropped; an `Extraction` where every job is dropped returns an error.
- [ ] 4.3 GREEN: implement `Validate()`.
- [ ] 4.4 Write the extraction system prompt (`internal/ingest/hackernews/llm.go`) — one
      prompt, no `kind` split (see design.md). Wire `requestSchema()` via `llmschema.Of`,
      mirroring `internal/ingest/telegram/schema.go`.
- [ ] 4.5 RED: write a test for `job.New(job.Draft{...})` wiring — `source = "hackernews"`,
      `external_id` is the comment id (or `"<id>/<job-index>"` for the second+ job out of one
      comment), asserting the exact format.
- [ ] 4.6 GREEN: implement the draft-building function.
- [ ] 4.7 Simplify pass over 4.1-4.6; re-run tests green.

## 5. Extract runner: branch, claim, write

- [ ] 5.1 RED: write a test for `ExtractRunner.Run` (`internal/ingest/hackernews/extract.go`)
      against fake `Store`/`Extractor`/board-contribution collaborators — a claimed comment
      with a recognized link goes through `contribute()` only; one without goes through LLM
      extraction only; either path ends in `Store.Complete` (mark extracted); a failure ends
      in `Store.Fail` (recorded, not silently dropped) — this proves Requirement "A comment
      is graded once" and "A single comment's processing failure does not abort the run"
      from specs/hackernews-ingest/spec.md.
- [ ] 5.2 GREEN: implement `ExtractRunner`.
- [ ] 5.3 Write `internal/ingest/hackernews/store.go`: the real `Store` implementation over
      sqlc queries — `Complete` writes the job (direct `UpsertJob` call inside one
      transaction with `EnqueueJobEnrichment`/`EnqueueSearchOutbox`, mirroring
      `cmd/tg-extract/store.go`'s shape exactly) or the board contribution, then
      `MarkHNPostExtracted`, all in one transaction.
- [ ] 5.4 Simplify pass over 5.1-5.3; re-run tests green.

## 6. Worker binaries

- [ ] 6.1 `cmd/hn-ingest/main.go`: `worker.Main(run)` → `worker.Bootstrap` → load
      `CrawlRunner` → run → `worker.ExitCode(...)`. Mirror `cmd/tg-ingest/main.go`'s shape.
- [ ] 6.2 `cmd/hn-extract/main.go`: load LLM config, build `llm.Client` tagged `"hackernews"`
      (matching how `cmd/tg-extract` tags its client `"telegram"` — see
      `internal/ai/llmkey/AGENTS.md`'s feature-tag convention), `worker.Bootstrap`, build
      `ExtractRunner`, run, `worker.ExitCode(...)`. Mirror `cmd/tg-extract/main.go`'s shape.
- [ ] 6.3 Add both to `internal/platform/worker/AGENTS.md`'s roster if that doc enumerates
      binaries (check first — only edit if the doc's own convention expects it).

## 7. Registration and generated code

- [ ] 7.1 Add `internal/ingest/hackernews` to `internal/platform/arch/layering/blocks.go`'s
      `ingest` block table.
- [ ] 7.2 `go run ./internal/platform/arch/layering` (or however the layering guard is
      normally invoked — check its own AGENTS.md) to confirm the new package passes.
- [ ] 7.3 Add `"hackernews"` to `cmd/gen-contracts/main.go`'s hardcoded non-adapter source
      list; run `make gen-contracts` and commit the regenerated
      `web/src/lib/generated/contracts.ts` diff (never hand-edit it — see the
      `generated_contracts_conflicts` lesson from prior source-adapter PRs).
- [ ] 7.4 Add `"hackernews"` to `cmd/liveness/main.go`'s `unsignalledSources` slice.
- [ ] 7.5 RED/GREEN: if `cmd/liveness` has an existing test asserting the exact contents or
      length of `unsignalledSources`/the exclude-from-probe set, update it; otherwise add
      one asserting `"hackernews"` is now excluded from the probe candidate set — proves
      specs/hackernews-ingest/spec.md's "closed by age, not by a liveness signal"
      requirement end to end at the liveness-worker level.

## 8. Verify

- [ ] 8.1 `gofmt -l .` clean.
- [ ] 8.2 `CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./...`.
- [ ] 8.3 `CGO_ENABLED=0 go test ./...` — full suite green (note the one known unrelated
      pre-existing `cmd/billing-sync` env-dependent failure from harvest-githublists-boards
      if it still reproduces; do not treat it as caused by this change).
- [ ] 8.4 `go vet -tags=integration ./...` (per AGENTS.md: run before every push).
- [ ] 8.5 Manual smoke: run `cmd/hn-ingest` and `cmd/hn-extract` against the local dev DB
      with a real (or recorded) Algolia response, confirm at least one board contribution
      and one directly-extracted job land correctly, matching design.md's stated behavior.
