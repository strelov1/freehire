## Context

The gateway swap is mostly an operations change: `provision/bifrost/` in freehire-ops carries the compose file, the config generator and the measurements. What reaches this repository is narrow, because LiteLLM leaked into only three places — four admin endpoints in `internal/ai/llmkey`, one header constant in `internal/platform/llm`, and the audio surfaces.

## Decisions

### Store the credential id rather than deriving or looking it up

Bifrost's create API does not accept a caller-chosen id, and its `PUT`/`DELETE` routes accept nothing else — aimed at the `sk-bf-…` secret they answer 404 (measured). Three options were live:

1. **A second column.** One nullable `text` with a unique constraint.
2. **Pack both into `users.llm_key`.** Saves a column, costs every reader a parse and adds a way to be wrong about the separator.
3. **Look the key up by name.** `name` is deterministic (`freehire-user-<id>`), but the governance API offers no lookup by name — it would mean listing every virtual key on each block.

(1). The two values are written and read together, which is what a second column is for.

**Why the guard on `ClaimUserLLMKey` stays on `llm_key` alone:** that column decides whether an account has a credential at all. Guarding on both would let a pre-0119 row — secret set, id null — read as unclaimed and be overwritten, orphaning a key that is still spending.

### Copy the provider policy from a template key

A virtual key with no `provider_configs` is denied every provider (deny-by-default, measured: *"could not auto resolve a provider"* on every call). Teams carry budgets only, so there is nothing to inherit. Every minted key must therefore carry a policy, and the question is where it comes from.

Carrying it in this service's configuration — an env var holding a JSON array, or a provider list in Go — would put the provider vocabulary in two places, and adding a provider would become a deployment of freehire rather than an edit to the gateway's own config. Instead `Mint` reads the virtual key named by `LLM_ADMIN_TEMPLATE_KEY` and copies its `provider_configs`. Verified end to end: the clone serves traffic, and this repository still names no provider.

The cost is one extra admin call per mint. Minting happens once per account, on its first AI call, so it is bounded by new accounts rather than by traffic.

**`TemplateKey` is required configuration, not optional.** Without it a client would mint successfully and every credential it produced would fail on first use — an outage that reads as a model fault. `New` returns nil instead, which is the same absent-not-broken answer the package already gives for a missing URL.

**One asymmetry:** a read answers `key_ids` as `null` where a write requires `["*"]`. Echoing the read back mints a key pinned to no provider key at all, so the copy normalises rather than echoes.

### `/me/usage` narrows, and the alternative was worse

The old gateway aggregated by an account id we already had, so a key replaced mid-month still reported everything. Bifrost keys its log by the credential id and offers no account-scoped query. Two options:

1. **Report the current credential only.** A re-mint loses the earlier key's calls from the figure.
2. **Remember retired ids and sum them.** Complete, at the price of a growing list of dead credentials kept alive to serve a usage counter.

(1). Re-minting happens only when the gateway refuses a stored key, which is rare; keeping a graveyard to make a counter marginally more complete is not a trade worth a table.

A consequence for the handler: `newUsageHandlers` now takes the resolver, and takes it to READ. `Stored`, never `For` — minting there would issue a credential to every visitor who opened the page out of curiosity.

### Switch audio off in the SPA, do not delete it

Both audio surfaces already latch themselves away when the server answers 501, and the Go side already answers 501 when unconfigured — so "off" needed no Go change at all. What it needed was the affordance not to appear: without a client-side switch the microphone renders, records, fails once, and only then disappears, which is a worse first impression than no microphone.

Commenting the markup out was rejected: two independent surfaces switched by one decision want one named constant with the reason attached, not two blocks of dead markup that rot. `AUDIO_ENABLED` in `web/src/lib/assistant/audioAvailability.ts` gates both mount points; the capability checks under it (`canRecord`, `canUseVoiceCall`) are untouched, so their tests keep testing what they were written for.

### Feature tags become bare words

`x-bf-dim-feature` names the dimension in the header. Keeping `feature:assistant` as the value would file spend under `feature:feature:assistant`-shaped labels and split one surface in two the day somebody wrote it the other way. The test that used to require the prefix now forbids a colon.

## Risks / Trade-offs

- **Per-feature cost stops being a SQL question.** The dimension reaches the log row, the OTel span, and the Prometheus labels — but `/api/logs/histogram/cost/by-dimension` aggregates only by provider, team, customer, user and business unit. "What does tailoring cost" becomes a Grafana query. Accepted: the figure is for us, not for users, and it is still answerable.
- **The usage log lands ~5s after the call.** Harmless over a month; fatal to any read-after-write assumption. Recorded in the package contract.
- **Dead keys cost availability differently.** Bifrost's dead-key set is per request, not a cooldown, so on a pool where most keys are dead the retry budget matters — measured at 1 failure in 6 with the default budget of 2. That is tuned in the gateway's own config (`provision/bifrost/gen-config.sh`), not here.

## Open Questions

- Which model each alias should point at. `flagship` currently resolves to a reasoning model on a free coding plan and shows a 38–70s tail on enrichment, inside the range where a 90s client timeout starts killing runs. This is a gateway-config question and belongs to a follow-on.
- Whether audio moves to a second gateway rather than waiting for this one. `LLM_BASE_URL` is one variable; nothing forces speech and chat through the same host.
