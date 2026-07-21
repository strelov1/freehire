## 1. Remove the legacy `users.points` system (backend)

- [x] 1.1 Add migration `migrations/0034_drop_users_points.sql` — `ALTER TABLE users DROP COLUMN points` with the standard header comment (fresh-volume initdb after 0033; manual `SET ROLE hire` on prod before deploy).
- [x] 1.2 Delete the `IncrementUserPoints` query from `internal/db/queries/link_contributions.sql`; remove the `points` column from the `GetUserByID`/auth-me select in `internal/db/queries/users.sql`.
- [x] 1.3 Remove the `IncrementUserPoints` call from `internal/contribution/repository.go` `Record` (users row no longer touched in the tx); update the doc comment referencing the point increment.
- [x] 1.4 Regenerate sqlc (`make sqlc`); update any Go references to the removed `points` field in `internal/db/models.go` consumers and the auth/me response so `go build ./...` passes.
- [x] 1.5 Update `internal/contribution/repository_test.go` (and any handler/auth test asserting `points`) to drop points expectations; assert `Record` no longer increments a user counter.

## 2. Credit transaction history endpoint (backend)

- [x] 2.1 Add sqlc query `ListCreditLedger` to `internal/db/queries/credits.sql` — a user's ledger rows (kind, feature, delta, ref, created_at) newest first with a `LIMIT`, using `credit_ledger_user_id_created_at_idx`.
- [x] 2.2 Add resolver queries: `ListJobLabelsByIDs` (`id, title, public_slug FROM jobs WHERE id = ANY`) and `ListTailoredCVJobLabelsByIDs` (`c.id, j.title, j.public_slug FROM cvs c JOIN jobs j ON j.id=c.job_id WHERE c.id = ANY`); regenerate sqlc.
- [x] 2.3 Add a history label mapper (in `internal/credits` or the handler) that projects a ledger row + resolved subject into `{kind, feature, delta, label, subtitle, created_at}`: `grant`→"Monthly grant"; `reward`→"Board contribution"; `debit`+`match`→"Match analysis" / job title; `debit`+`tailor`→"CV tailoring" / job title; unknown kind→feature or generic label with delta. Missing subject → bare feature label. Unit-test the mapper including the fallback and unknown-kind branches.
- [x] 2.4 Add handler `GetMyCreditsHistory` in `internal/handler/me_credits.go`: require user, fetch the ledger page, batch-resolve match refs (job ids) and tailor refs (cv ids), build the labelled DTO list, return `{"data": [...]}`. Register `GET /api/v1/me/credits/history` in `internal/handler/handler.go` (cookie or API key, like `GetMyCredits`).
- [x] 2.5 Add a handler test (build-tagged DB test per project convention) covering: newest-first ordering, caller scoping (no other user's rows), match/tailor/grant/reward labelling, and a deleted-subject fallback.

## 3. Credits page, API, and navigation (frontend)

- [x] 3.1 Add the history type and `api.myCreditsHistory()` to `web/src/lib/types.ts` + `web/src/lib/api.ts` (fetch `/api/v1/me/credits/history`, return the DTO list).
- [x] 3.2 Create `web/src/lib/components/CreditsView.svelte`: headline balance (reuse `api.myCredits()`), then the transaction history list (label, subtitle, signed delta with +/− styling, date). Empty-state when no history.
- [x] 3.3 Add route `web/src/routes/my/credits/+page.svelte` (title "Credits", renders `CreditsView`), mirroring the existing thin `my/*` route wrappers.
- [x] 3.4 Add the "Credits" nav item to `web/src/routes/my/+layout.svelte` and `web/src/lib/components/AccountNavRail.svelte` (href `/my/credits`), matching the existing item shape/order.

## 4. Restate contributions in credits & remove the inline widget (frontend)

- [x] 4.1 Remove `User.points` from `web/src/lib/types.ts` and every read of `currentUser()?.points`.
- [x] 4.2 Rewrite `web/src/lib/components/ContributeView.svelte`: drop the points badge/`points` derived value; restate reward copy as "+5 AI credits per new board"; link to `/my/credits` for the balance.
- [x] 4.3 Rewrite the "points" copy in `web/src/lib/components/ContributeLandingView.svelte` to speak in AI credits ("What your credits are for", "credit you 5 AI credits", etc.).
- [x] 4.4 Remove the `CreditsBalance` widget usage from the Activity → Matches tab and the Profile page; delete `CreditsBalance.svelte` if it has no remaining consumers.

## 5. Verify

- [x] 5.1 `go build ./... && go vet ./... && go test ./...` green; frontend `svelte-check` clean.
- [x] 5.2 Manually drive the flow (verify skill): `/my/credits` shows balance + labelled history; `/my/contributions` and the landing speak only in credits; no widget on Matches/Profile; a contribution still awards +5 credits and appears in history.
