## 1. Board recognition

- [x] 1.1 Add `internal/contribution/board.go`: `recognizeBoard(url) → (source, board, canonical, ok)` — an `atsBoards` host table (greenhouse/lever/ashby/workable, exact or subdomain match) + first-path-segment board + tail canonicalization (query/fragment/`/apply`/trailing slash).
- [x] 1.2 Table test `recognizeBoard`: vacancy AND board-listing URLs → same `(source, board)`; single-tenant/unknown host/board-less/non-http → declined.
- [x] 1.3 (Reverted an earlier over-design: a `BoardKey` method threaded through the `linksource.Source` interface + 9 adapters. `linksource` is left untouched — the table is simpler and extends by one row.)

## 2. Schema & sqlc

- [x] 2.1 Add `migrations/0025_link_contributions.sql`: `link_contributions` (id, submitted_by fk, url, source, board, status CHECK, created_at, `unique(source, board)`) and `ALTER TABLE users ADD COLUMN points integer NOT NULL DEFAULT 0`.
- [x] 2.2 Queries `internal/db/queries/link_contributions.sql`: `CreateContribution`, `JobsExistForBoard` (board-tracked via `starts_with`), `ListContributionsByUser`, `IncrementUserPoints`. Add `points` to `GetUserByID`; add `GetUserIDByTelegramChat` reverse lookup.
- [x] 2.3 `make sqlc`; `go build ./...` clean.

## 3. contribution module

- [x] 3.1 `internal/contribution/contribution.go`: domain `Contribution`, `Repository`, sentinels (`ErrUnsupportedATS`, `ErrBoardAlreadyTracked`, `ErrBoardAlreadyContributed`), `Service`.
- [x] 3.2 TDD `Service.Submit`: recognize board (else `ErrUnsupportedATS`) → board-tracked (else `ErrBoardAlreadyTracked`) → record + increment points in one tx (unique → `ErrBoardAlreadyContributed`). Branch tests with a fake Repository.
- [x] 3.3 TDD `Service.ListMine` scoped to the caller, newest first.
- [x] 3.4 `repository.go` over `*db.Queries`: insert+increment in a pgx transaction; unique violation → `ErrBoardAlreadyContributed`.
- [x] 3.5 Integration test (build-tag) — PASSED on real Postgres: record+point, duplicate board rejected (no 2nd point), board-tracked query, concurrent duplicate credits once.

## 4. HTTP surface

- [x] 4.1 Wire `contribution.Service` into `handler.API` (`contribution.New(NewQueriesRepository(queries, pool))`).
- [x] 4.2 `POST /api/v1/me/contributions` + `GET /api/v1/me/contributions` (`RequireAuthOrKey`); sentinels → 422/409; 201 with the recorded board.
- [x] 4.3 Add `points` to `/api/v1/auth/me` (threaded `GetUserByID` → `accounts.User` → `userResponse`).
- [x] 4.4 Handler tests: unit mapping (422/409/409) + response shape; integration flow PASSED — 401/422/409 tracked/201 novel/409 duplicate (vacancy + listing)/`me` balance/my-list.

## 5. Telegram front door

- [x] 5.1 `handleTelegramContribution` on the webhook: extract first URL, resolve chat→user (`GetUserIDByTelegramChat`), `Submit`, reply with outcome; no-link ignored, unlinked chat prompted to link.
- [x] 5.2 Integration test PASSED on real Postgres: linked user's link recorded+rewarded+reply; second same-board link no point; non-link silent; unlinked chat prompted.

## 6. Frontend

- [x] 6.1 `ContributeView.svelte` + `/my/contributions`: paste-a-link form → `api.submitContribution`, distinct errors surfaced, points balance (refreshed via `invalidateAll`).
- [x] 6.2 Render the discovered-boards list (board, source, timeAgo) from `api.listMyContributions`.
- [x] 6.3 `accountNav` entry + `my/+layout` icon; `points` on the `User` type; `accountNav.test` count 9→10.
- [x] 6.4 Visual-verified (headless Chrome screenshot); `svelte-check` 0 errors, `eslint` clean, `vitest` green.

## 7. Verify & docs

- [x] 7.1 `go build ./... && go vet ./... && go test ./...` clean; integration-tagged contribution + handler (incl. Telegram) tests PASS on real Postgres; migration `0025` applies in the full glob.
- [x] 7.2 End-to-end drive on a live stack (Postgres + server + SvelteKit): novel board → 201 + balance; second vacancy/listing on same board → 409; unsupported → 422.
- [x] 7.3 Added `internal/contribution/AGENTS.md` (board model + two front doors); `linksource/AGENTS.md` left as original; manual `0025` migration noted.
