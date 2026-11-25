## Context

`internal/api/atsapply` already has `PreviewClient.schemaFor` (preview_client.go), which checks `p.forms` (a `StoredFormReader`) BEFORE `p.fetchers[claimed.Provider]` — storage-first, for every provider, not only one with no live fetcher. That order is right for what `Preview` is: a cheap, non-authoritative look at what an attempt would currently send, where a "rarely-stale" captured row is an acceptable trade for skipping a network round trip (`schemaFor`'s own comment).

`Client.fetchSchema` (client.go) is the real submission path and has no storage fallback at all today — it only ever checks `c.fetchers[claimed.Provider]`, so a provider with no live fetcher (today: only Recruitee) parks immediately via `errNoSchemaFetcher`.

**`apply_forms` is not a Recruitee-only store.** `cmd/capture-apply-form` drains a queue and writes a row there for every provider `internal/ingest/applyform.NeedsRequestCapture` says needs one — `fetcherFor` names Greenhouse, Ashby, Workable, and Lever (fetch.go). Recruitee is the one provider that reaches the same table a different way (its form arrives free with the ingest crawl and is written directly, never queued). So a live-fetcher provider's row in `apply_forms` is not hypothetical — it exists today, captured for the job page's own apply-form display (`internal/api/handler/apply_form.go`'s `GetApplyFormByJobID`), and can be older than the posting's current form. `PreviewClient`'s storage-first order accepts that staleness deliberately, for preview's own purpose; a real submission should not inherit that trade-off for free.

`StoredFormReader` and its real adapter `dbApplyFormReader` (`cmd/auto-apply/apply_form_reader.go`, backed by `db.GetApplyFormByJobID`) already exist and are already in production use by `PreviewClient`. This design does not invent anything new at the data-access layer — it wires an existing, working piece into a second consumer.

`Client` already has a precedent for adding an optional dependency without touching `NewClient`'s positional signature: `WithBrowserUse(executor *BrowserUseExecutor) *Client`, added specifically "so adding this optional dependency never touches `NewClient`'s existing positional signature or any of its other call sites."

## Goals / Non-Goals

**Goals:**
- `Client.fetchSchema` tries a stored form before giving up on a provider with no live fetcher — matching the spec's own scope, not `PreviewClient.schemaFor`'s storage-first order (see Decisions: the two deliberately differ).
- The existing `dbApplyFormReader` is reused as-is — no new adapter, no new query.
- No change to `fillProviders`, `browserUseProviders`, or any park-reason string.
- No change to `fetchSchema`'s observable behavior for a provider that already has a live fetcher (Greenhouse, Ashby, Workable, Lever) — every one of those also has a row in `apply_forms` today, so this matters, not a hypothetical to guard against.

**Non-Goals:**
- Making Recruitee (or any provider) actually submit. Out of scope, per proposal.md.
- Touching `PreviewClient` itself — it already does this correctly; nothing here changes it.
- Touching `autoApplyEnqueueSources` or any enqueue-time eligibility gate.
- Deduplicating `PreviewClient.schemaFor` and `Client.fetchSchema` into one shared function. They are the same *shape* of logic but serve different callers with different signatures (`Preview` vs `Submit`) and different failure handling around them; forcing a shared helper now would be a larger refactor than this fix calls for, and nothing about this change requires it.

## Decisions

**Add `WithStoredFormReader(forms StoredFormReader) *Client`, mirroring `WithBrowserUse` exactly, rather than a new `NewClient` parameter.** `Client` already established this pattern for exactly this situation — an optional dependency that only one caller (`cmd/auto-apply/main.go`) needs to wire, without touching `NewClient`'s signature or any test that constructs a bare `Client`. A positional parameter would ripple through every existing `NewClient(...)` call site (production and test) for a dependency that is allowed to be nil; a setter does not.

**`StoredFormReader` interface is reused unchanged from `preview_client.go`** — not duplicated, not moved. `Client` and `PreviewClient` live in the same package, so both can reference the one interface directly; no new file is needed for it.

**`fetchSchema` tries `c.fetchers` FIRST, falling back to `c.forms` only when no fetcher is registered for the provider — deliberately NOT `PreviewClient.schemaFor`'s storage-first order.** The mirrored, storage-first order was the original plan here and is wrong for this caller: since `apply_forms` already holds a row for Greenhouse/Ashby/Workable/Lever too (Context, above), storage-first would silently prefer that possibly-stale, display-captured row over a fresh fetch for a REAL submission on every one of those providers — not a hypothetical, since the table demonstrably has those rows today. Live-fetcher-first, storage-as-fallback, changes nothing observable for any provider that already has a fetcher (the fetcher branch runs exactly as it did before this change), and reaches `c.forms` only for the one case a live fetch cannot answer at all: no fetcher registered — today, only Recruitee. This is also what the spec itself already says ("no registered live schema fetcher"), so this correction brings the code in line with the spec that was already scoped correctly, not the other way around.

**`c.forms` may be nil**, degrading `fetchSchema` to exactly today's behavior (live-fetch-only, `errNoSchemaFetcher` when none registered) — the same optionality every other injected dependency on `Client` already has (`llmClient`, `atoms`, `cvs` are all documented as "may be nil, disables X, leaves everything else unchanged").

**No change to the `Submit` control flow below `fetchSchema`.** Once `apiForm` is obtained (live or stored), everything downstream — `mergedFromAPIOnly`, `resolve`, the `fillProviders`/`browserUseProviders` branching, the park reasons — is untouched. This is deliberate: the spec's third requirement (resolvable-but-unsubmittable providers still park, with precise `Unmapped` reporting when incomplete) is already exactly what this code does today for Ashby/Workable when `browserUseEligible` returns false or the spend guard refuses; Recruitee simply starts reaching that same, already-correct logic instead of being turned away one step earlier.

## Risks / Trade-offs

- **[Risk]** A stored Recruitee form could be stale (the posting's form changed since capture, no re-capture has run). → **Mitigation**: not a new risk introduced by this change — `PreviewClient` already carries the identical risk today for its own preview reads, documented in its own comment ("a stored, rarely-stale row"); this change does not add a second, differently-stale copy, it reuses the same one preview already trusts.
- **[Risk]** Someone reads "Recruitee now reaches field resolution" as "Recruitee auto-apply now works." → **Mitigation**: proposal.md states the non-outcome plainly and repeatedly; the spec's own requirements explicitly cover the still-parks case; `internal/api/atsapply/AGENTS.md` should be updated (see tasks.md) so the package's own documentation doesn't imply otherwise either.
- **[Trade-off]** This does not close the "candidate spends a real allowance on an attempt with no submit path" concern the earlier audit raised — that remains true after this change, unchanged, and is explicitly out of scope here (see proposal.md and this session's own scoping decision).
