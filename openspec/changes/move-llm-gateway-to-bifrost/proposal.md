## Why

The LLM gateway is being replaced. LiteLLM needs one *deployment* entry per key × model, which is how its production config reached 164 entries for a 44-key pool, and it has no per-key cooldown story that survives a pool where most keys are dead. A candidate — Bifrost — was stood up beside it on the same host and the same key pool, and measured rather than read about:

- **Cross-provider fallback works from configuration alone.** A virtual key carrying weighted `provider_configs` picks a primary and files the rest as fallbacks. Observed live: Z.AI failed → Gemini failed → Mistral answered, one user-facing request. The caller sends no `fallbacks` array.
- **Structured output is not worse, and is better on Mistral.** Ten runs of the exact `response_format: json_schema` this repo builds: `gemini-2.5-flash` scored 7/10 through either gateway; `mistral-large-latest` scored 2/10 through LiteLLM and 10/10 through Bifrost.
- **Streamed tool calls arrive whole.** The fragmentation `mergeToolCallFragments` exists to repair does not reproduce, verified to an 1851-byte argument.
- **Spend history outlives the credential.** LiteLLM hangs the spend record off the key, which is why `internal/ai/llmkey` blocks rather than deletes. Bifrost keys its usage log by the credential's id; a deleted key's figures survive.

Three things about the replacement force code changes rather than an environment edit, and they are what this change is for.

## What Changes

- **A credential becomes two values.** Bifrost addresses a virtual key by an opaque id and answers 404 to anything aimed at the secret, so a stored secret alone can spend but never be revoked — which is what account deletion has to do. Add `users.llm_key_id`; write and clear it with the secret.
- **Minting reads its provider policy instead of carrying one.** A virtual key with no `provider_configs` is refused every provider. Rather than duplicate the provider list into this service's configuration, `Mint` copies it from a template virtual key named by `LLM_ADMIN_TEMPLATE_KEY`.
- **The administrative surface changes shape.** Four endpoints move under `/api/governance/...` and `/api/logs/stats`, and authenticate over HTTP Basic rather than a bearer token. `LLM_ADMIN_KEY` retires in favour of `LLM_ADMIN_USERNAME` / `LLM_ADMIN_PASSWORD` / `LLM_ADMIN_TEMPLATE_KEY`.
- **The feature tag changes header and shape.** `x-litellm-tags: feature:assistant` becomes `x-bf-dim-feature: assistant` — the header names the dimension, so the value is a bare word.
- **`/me/usage` narrows to the current credential.** Bifrost offers no way to ask by account, so a key replaced mid-month leaves the earlier one's calls out of the figure.
- **Audio goes dark.** Dictation and voice mode reach the gateway too, and the replacement's `/v1/audio/transcriptions` and `/v1/realtime/client_secrets` have never been exercised against it — the pool behind it carries no OpenAI credential. The Go side already degrades to 501 when unconfigured; the SPA gains a matching switch so the microphone is absent rather than present-then-broken.

## Capabilities

### Modified Capabilities

- `llm-spend-attribution`: the per-user gateway credential becomes an (id, secret) pair, mints its provider policy from a template rather than from configuration, and reports usage scoped to one credential.
- `speech-to-text`: audio is unavailable while the gateway migration leaves its audio routes unproven, and the SPA suppresses the affordance rather than offering one that fails.

### Removed Capabilities

<!-- None. Audio is switched off, not removed: no code was deleted and one constant restores it. -->

## Impact

- **Deploy order matters.** Migration 0119 must land before the binary that reads `llm_key_id`. It is one nullable column with a unique constraint — additive, unread by the previous binary, harmless to roll back.
- **Rollback is unsetting the new environment variables.** Any of the four unset ⇒ `llmkey.New` returns nil, nothing is minted, and every call goes out on `LLM_API_KEY` exactly as before.
- **No backfill.** Rows minted before 0119 hold a secret and no id. They keep working for inference and acquire an id the first time the gateway refuses them, through the existing forget-and-re-mint path.
- **Retired credentials at the old gateway are left alone.** They stop being presented the moment `LLM_BASE_URL` moves; erasing them is a separate, reversible decision.
