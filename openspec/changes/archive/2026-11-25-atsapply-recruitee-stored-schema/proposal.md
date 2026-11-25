## Why

A Recruitee auto-apply attempt parks unconditionally at `Client.fetchSchema` (`internal/api/atsapply/client.go`) before any field resolution runs at all, because `internal/ingest/applyform.Fetchers` has no live fetcher for Recruitee — its form arrives free with the ingest crawl and is written directly to `apply_forms` instead. The code already names this a real gap: "Reaching Recruitee would need reading its already-captured form from storage instead of `applyform.Fetcher` — a real gap, not a decision" (`internal/api/atsapply/browseruse_fill.go`). The fix pattern already exists and is proven in production for the preview path (`PreviewClient`'s `StoredFormReader`) — it was simply never wired into the real submission path.

## What Changes

- `Client` (the real submission path `cmd/auto-apply` drives) gains the same `StoredFormReader`-backed fallback `PreviewClient` already has: when no live fetcher is registered for a provider, `fetchSchema` reads the job's captured form from `apply_forms` before giving up.
- The already-existing `dbApplyFormReader` adapter (`cmd/auto-apply/apply_form_reader.go`) is wired into the `atsapply.NewClient(...)` call site in `cmd/auto-apply/main.go`, alongside its existing use in `NewPreviewClient(...)`.
- A Recruitee attempt now reaches deterministic field resolution and, where fields are missing, LLM-drafted answers (`ResolveWithDrafting`) — the same treatment every other non-Greenhouse provider already gets — instead of parking before any of that runs.

**Explicitly unchanged, stated plainly so this isn't mistaken for more than it is:** Recruitee still cannot actually submit after this change. It is in neither `fillProviders` (Greenhouse-only DOM fill) nor `browserUseProviders` (Ashby/Workable-only cloud fallback), so a fully-resolved Recruitee `Plan` still parks with the same candidate-facing `reasonSubmissionNotImplemented` ("submission not yet implemented for this provider") it already shows today. What changes is precision: an attempt that is *not* fully resolvable now reports which fields are actually `Unmapped` instead of an undifferentiated park, and the fields that *are* resolvable get resolved. Adding Recruitee to a real fill/submit path is a separate, later change.

Not in scope, and not being revisited by this change: `internal/api/handler/auto_apply_enqueue.go`'s `autoApplyEnqueueSources` allowlist — which providers may be queued at all is an already-documented, deliberate product decision, unrelated to whether schema-fetch itself works.

## Capabilities

### New Capabilities
- `atsapply-stored-schema-fallback`: when resolving an application form for submission and no live schema fetcher is registered for the posting's ATS provider, the system reads the form already captured into storage during ingest, when one exists, instead of parking immediately.

### Modified Capabilities
(none — `fillProviders`/`browserUseProviders` and the park reasons they produce are unchanged; this only changes whether schema resolution itself can proceed for a provider with no live fetcher)

## Impact

- `internal/api/atsapply/client.go`: `Client` gains a `forms StoredFormReader` field; `fetchSchema` tries it before falling through to `errNoSchemaFetcher`, mirroring `PreviewClient.schemaFor`.
- `internal/api/atsapply/client.go` (`NewClient`): new parameter to accept a `StoredFormReader` (may be nil, degrading to today's live-fetch-only behavior — the same optionality `PreviewClient` already has).
- `cmd/auto-apply/main.go`: passes the existing `&dbApplyFormReader{q: queries}` into `atsapply.NewClient(...)`.
- No migration, no new table — reuses `apply_forms` and `GetApplyFormByJobID`, both already in production use via the preview path.
- No change to `fillProviders`, `browserUseProviders`, `reasonSubmissionNotImplemented`, or `autoApplyEnqueueSources`.
