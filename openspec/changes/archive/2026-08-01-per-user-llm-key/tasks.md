## 1. The credential

- [x] 1.1 Add the migration putting a nullable key column on `users`. Take the next free number after re-checking `origin/main` — `0066` was last when this was planned and numbers have collided here before.
- [x] 1.2 Add the queries: read the key, claim it conditionally (`WHERE id = $1 AND llm_key IS NULL RETURNING llm_key`), and clear it. Run `make sqlc` and commit the generated diff.
- [x] 1.3 Add `internal/llmkey`: the admin client over `/key/generate`, `/key/info` and `/key/delete`. Test it against an `httptest` stub — the fields sent on mint, the fields read back, and a non-2xx surfacing as an error rather than an empty key.
- [x] 1.4 Add resolve-or-mint. Test the concurrent-first-call race explicitly: both mint, one wins the conditional claim, the loser uses the winner's key AND deletes the one it minted.

## 2. Spending under it

- [x] 2.1 Give `llm.Client` a clone bound to a credential and a tag list. **Test first that the clone does not share `schemaModels`** — two clones on different credentials must not serve each other's schema-bound model. This is the cross-account leak the design names; write that test before the method exists.
- [x] 2.2 Inject `x-litellm-tags` through `openai.WithHTTPClient`, following `schemaInjector`. Assert the header reaches the wire against a stub endpoint, with more than one tag.
- [x] 2.3 Test that every failure falls open: minting fails, the admin API is unreachable, the stored key is rejected. In each case the call goes out on the service credential and completes; a rejected key is additionally re-minted and stored.

## 3. The call sites

- [x] 3.1 Add the one helper every per-user site resolves through, and wire the assistant turn to it — tagged with its feature and its preset.
- [x] 3.2 Wire the rest: follow-ups, ~~CV tailoring~~, match analysis, CV extraction, the ATS review, the autofill planner. One tag each. **CV tailoring makes no model call of its own** — `/me/cvs/tailor` mints a CV and debits credits, and the work is an assistant turn under the `tailor` preset, already tagged. A second tag would double-count one spend.
- [x] 3.3 Assert the worker entrypoints are untouched — an enrichment run resolves no user credential and spends on the service one.

## 4. Reading it and erasing it

- [x] 4.1 Add `GET /api/v1/me/usage`. **Reshaped after review**: it reads the gateway's daily-activity rollup, not `/key/info`, and reports model calls / failures / tokens rather than cost. The gateway's figure is a list price on a mixed pool — not our cost and not the caller's price, which is credits over the same calendar. The read is scoped by account id, so it needs no credential at all.
- [x] 4.2 Handler tests: a caller with activity, a caller with none (200 and zeroes, not 404), owner scoping across two accounts, 401 without a credential, an unreachable gateway answering 200 with zeroes, the period matching the credits calendar, and no money-shaped field anywhere in the response.
- [x] 4.3 Delete the gateway key in `accountdelete`, before `DeleteUser` — the column is about to vanish. Test that a failing gateway does not stop the account from being deleted.

## 5. Configuration

- [x] 5.1 Add `LLM_ADMIN_URL`, `LLM_ADMIN_KEY`, `LLM_USER_MAX_BUDGET`, `LLM_USER_RPM_LIMIT`, `LLM_USER_BUDGET_WINDOW`. Test that an unset admin API disables the whole path — no minting, no resolution, every call on `LLM_API_KEY` exactly as today. **`LLM_ADMIN_URL` was not in the original plan**: the admin routes live at the gateway root and `/v1/key/*` answers 404, so the endpoint cannot be derived from `LLM_BASE_URL`.
- [x] 5.2 Test that no ceiling is sent when none is configured, and that a configured one is passed through on mint.

## 5b. The panel

- [x] 5b.1 `GET /me/usage` in the SPA API client plus its type.
- [x] 5b.2 An activity panel on the credits page, gated on `beta_tester` — the balance is what you may spend, the panel is what you did. It loads on its own request and fails on its own, so a network blip cannot take the balance and history with it, and it states outright that one conversation is several calls.

## 6. Close out

- [x] 6.1 Update `internal/assistant/AGENTS.md`: replace the "No metering" limitation with what is now true — a turn is attributed and readable, still not bounded. Write down the shared-`modelCache` hazard where the next person will look for it (`internal/llm`), because a clone sharing that cache reads as an optimisation until someone explains it is a credential leak. Also added `internal/llmkey/AGENTS.md`, its row in the module table, and the root convention bullet.
- [x] 6.2 `go build ./... && go vet ./... && gofmt -l .`, `go test ./...`, `go test -tags=integration ./internal/handler/` — plus `./internal/db/`. **The integration suite caught a real bug**: `userLLM` returns a typed `*llm.Client`, and a nil one assigned into the runner's `Model` interface is NOT a nil interface, so the turn panicked mid-stream. Fixed in `boundRunner`; the unit test that missed it passed an *untyped* nil, and a regression test now covers the typed one.
- [x] 6.3 Verify against the spec's scenarios — every one carries a test; the two that did not are now covered (the credential never appears in a response body; a ceiling refusal is NOT retried past). Deploy note below, still to run.
  - Deploy: set `LLM_ADMIN_URL` and `LLM_ADMIN_KEY` in the deployment environment — never in the repository — then confirm within a day that `LiteLLM_DailyTagSpend` carries the new `feature:`/`preset:` tags and that `LiteLLM_VerificationToken` is growing per-user rows.
- [ ] 6.4 Let it run a week before choosing any ceiling. The distribution is the point.

## 7. Deliberately not here

Choosing a budget number, refusing a call that exceeds one, reconciling the gateway's dollars
with `internal/credits`' points, moving the workers off the master key, and dictation. Open
those once this has a week of attributed data behind it.
