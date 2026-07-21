## 1. Data model & queries

- [x] 1.1 Add `migrations/NNNN_credits.sql`: `credit_ledger` and `credit_balances` tables with the two partial unique indexes (debit-ref, grant-period) and the `kind`/`feature` CHECKs from design.md
- [x] 1.2 Add `internal/db/queries/credits.sql`: `GetBalanceForUpdate`, `UpsertBalance`, `InsertLedger` (grant + debit), `GetLedgerDebit` (idempotency lookup by user/feature/ref), and a read-only `GetBalance`
- [x] 1.3 Run `make sqlc` and verify the generated `internal/db` compiles (`go build ./...`)

## 2. Config

- [x] 2.1 Add `CREDITS_MONTHLY_GRANT` (default 20), `CREDITS_COST_MATCH` (default 1), `CREDITS_COST_TAILOR` (default 3) to `internal/config`, with a unit test covering defaults and env overrides

## 3. Credits package

- [x] 3.1 Create `internal/credits`: `Store` type, `Balance` value (`Remaining`, `ResetsAt`), `Feature` constants (`match`, `tailor`), typed `ErrInsufficient`, and the period helper (`YYYY-MM` UTC + `resets_at` = first of next month)
- [x] 3.2 Implement `Store.Balance(ctx, userID)` with in-memory lazy reset for display; unit-test fresh user, mid-period, and rolled-over-period cases
- [x] 3.3 Implement `Store.Debit(ctx, userID, feature, ref)` as the atomic transaction (FOR UPDATE, lazy reset + grant, idempotency short-circuit, insufficient check, decrement + ledger append); unit-test sufficient debit, repeat-ref no-op, insufficient (402 path), and lazy-reset-then-debit
- [x] 3.4 Add a concurrency test proving two racing debits with points for only one do not oversell

## 4. Match integration (job-fit)

- [x] 4.1 Wire a `*credits.Store` into the handler `API` and inject it where `jobFitCache` is constructed
- [x] 4.2 Replace `enforceFitQuota` in `PostJobFit` and `StreamJobFit` with a pre-check (`Balance` ≥ match cost OR `(user, job)` already debited) before the LLM, returning 402 on insufficient
- [x] 4.3 Debit `match`/`job_id` only after successful analysis persistence in both endpoints; verify a failed/unpersisted analysis leaves the balance unchanged
- [x] 4.4 Swap the `quota` object on `GET /jobs/:slug/fit` for `credits {remaining, resets_at}`; update the handler DB-integration test

## 5. Tailor integration

- [x] 5.1 Debit `tailor`/`cv_id` inside `TailorCV` (`POST /me/cvs/tailor`) before minting the session; return 402 with `{error, remaining, resets_at}` on insufficient and create nothing
- [x] 5.2 Verify resuming/re-opening an existing tailored CV does not debit again (idempotent by `cv_id`)

## 6. Frontend

- [x] 6.1 Match page: read `credits {remaining, resets_at}`, render the balance, and handle a 402 out-of-credits state (replacing the 429 quota handling)
- [x] 6.2 Tailor surface: handle 402 on tailored-CV creation with an out-of-credits message showing `resets_at`

## 7. Verification

- [x] 7.1 `go build ./... && go vet ./... && go test ./...` green
- [x] 7.2 Run the queue/handler integration tests that touch credits (`go test -tags=integration ./internal/db/` and handler DB tests)
- [x] 7.3 End-to-end drive: exhaust the grant via repeated new-job matches, confirm 402 + `resets_at`, confirm recompute stays free, confirm a tailor create debits 3
