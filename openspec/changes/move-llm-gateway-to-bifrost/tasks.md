> **Provenance.** Sections 1–5 were implemented and committed (`f24c66f4`) before
> this change existed, during the evaluation that produced `provision/bifrost/` in
> freehire-ops. They are ticked because the work and its tests are in the branch, not
> because they ran through a red-first loop — they did not. Section 6 is the part of
> the delivery discipline that can still be honoured, and it is open.

## 1. Schema

- [x] 1.1 Migration `0119_user_llm_key_id.sql`: nullable `users.llm_key_id text` with a unique constraint. Additive, no backfill, unread by the previous binary.
- [x] 1.2 `llm_keys.sql`: `GetUserLLMKey` and `ClaimUserLLMKey` return both columns; `ClearUserLLMKey` clears both. The claim guard stays on `llm_key` alone so a pre-0119 row is not overwritten.
- [x] 1.3 `make sqlc`.

## 2. Gateway client

- [x] 2.1 `Credential{ID, Secret}` replaces the bare secret; `Mint` returns both and refuses a half credential.
- [x] 2.2 Endpoints move to `POST /api/governance/virtual-keys`, `PUT`/`DELETE` on `/{id}`, `GET /api/logs/stats`; authentication becomes HTTP Basic.
- [x] 2.3 `Mint` reads the template key and copies its `provider_configs`, normalising `key_ids: null` to `["*"]`; a policyless template fails the mint.
- [x] 2.4 `Activity` takes a credential id, makes two reads (totals, then filtered to errors for the exact failure count) and widens the window to whole days.
- [x] 2.5 Tests: unconfigured-is-nil across all four fields, policy copied not invented, allowlist normalised, half-credential refused, unwrapped answer read, 404-is-done, admin 401 is not a stale user key.

## 3. Resolver and callers

- [x] 3.1 `read`/`Stored`/`mint` carry the pair; `Forget` reads before clearing and blocks only the credential the refusal was about; `Revoke` blocks by id.
- [x] 3.2 `newUsageHandlers` takes the resolver to READ (`Stored`, never `For`).
- [x] 3.3 Config: `LLM_ADMIN_USERNAME` / `LLM_ADMIN_PASSWORD` / `LLM_ADMIN_TEMPLATE_KEY` replace `LLM_ADMIN_KEY`; `cmd/server` rewired.
- [x] 3.4 Tests: fakes carry ids, the usage fake answers both reads, seeding a secret seeds its id.

## 4. Feature tag

- [x] 4.1 `x-litellm-tags` → `x-bf-dim-feature`; the constants in `user_llm.go` become bare words.
- [x] 4.2 The tag test forbids a colon in the value rather than requiring a `feature:` prefix.

## 5. Audio off

- [x] 5.1 `web/src/lib/assistant/audioAvailability.ts` — one `AUDIO_ENABLED` constant carrying the reason and the restore path.
- [x] 5.2 Gate both mount points: the microphone in `Composer.svelte`, voice mode in `AssistantChat.svelte`. Capability checks underneath left untouched.
- [x] 5.3 `internal/ai/speech/AGENTS.md` records why it is dark, that nothing was deleted, and the `session.model` wire-shape difference waiting on the way back.

## 6. Delivery gates — OPEN

- [x] 6.1 `simplify` over the branch diff; tests stay green after it. Extracted `Client.policy`, hoisted the governance path to a constant, folded the request-body encode onto one path, and stopped `Activity` aliasing its `url.Values` between the two reads.
- [x] 6.2 Code review of the diff. Found one Critical and four Important, all fixed:
  - **Critical** — the assistant passed two tags, which the transport joined with a comma into ONE `x-bf-dim-feature`, filing it under `assistant,preset:chat` and five siblings. None was `assistant`. The variadic tag list was the previous gateway's shape; replaced with `llm.Dimension{Name, Value}` and one header per dimension, so the preset gets `x-bf-dim-preset`.
  - **Important** — `Forget` discarded `read`'s `ok`, so a transient database fault cleared the row (the only record of the gateway id) while declining to block, permanently orphaning a live key. Now it leaves the row alone.
  - **Important** — `Revoke` had no test at all, and it is the reason the id column exists. Added: blocks by id, leaves the row, no-ops without an id.
  - **Important** — `/me/usage` must never mint; nothing asserted it. Added a test with an unseeded store.
  - **Important** — a comment in `me_usage_test.go` still asserted the pre-migration semantics.
  - Minors taken: the failure count is now observable (the fake answers the two reads differently), and the window closes at the last instant of the day rather than dropping its final second.
- [x] 6.3 `verification-before-completion`: unit suite, `go vet` under both build tags, layering guard, lint ratchet, svelte-check, web tests — each run with its output: gofmt clean, unit suite pass, `go vet` clean under `integration` and `llmlive`, layering guard ok, lint ratchet 0 issues, svelte-check 0 errors, 1186 web tests pass.
- [ ] 6.4 `finishing-a-development-branch`: integrate `bifrost-gateway`.
- [ ] 6.5 `/opsx:archive` then `/opsx:sync`.
