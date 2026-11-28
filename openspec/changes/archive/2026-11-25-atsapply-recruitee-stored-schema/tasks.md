## 1. Client gains a stored-form fallback

- [x] 1.1 Add a `forms StoredFormReader` field to `Client` (`internal/api/atsapply/client.go`), reusing the `StoredFormReader` interface already defined in `preview_client.go` — no new interface.
- [x] 1.2 Add `WithStoredFormReader(forms StoredFormReader) *Client`, mirroring `WithBrowserUse`'s exact shape (sets the field, returns `c`), leaving `NewClient`'s signature untouched.
- [x] 1.3 Update `fetchSchema` to try `c.fetchers[claimed.Provider]` FIRST as today, and only when no fetcher is registered, fall back to `c.forms.GetStoredForm(ctx, claimed.JobID)` when `c.forms != nil` — deliberately NOT `PreviewClient.schemaFor`'s storage-first order (design.md: `apply_forms` already holds rows for Greenhouse/Ashby/Workable/Lever too, so storage-first would risk a real submission reading a possibly-stale, display-captured row for providers that already work).

## 2. Wiring

- [x] 2.1 In `cmd/auto-apply/main.go`, call `.WithStoredFormReader(&dbApplyFormReader{q: queries})` on the `sidecar` Client after construction (the adapter already exists and is already used for `NewPreviewClient` two lines below).

## 3. Tests for the added spec requirements

- [x] 3.1 Unit test: `fetchSchema` (or `Submit`, at whichever level existing tests for `errNoSchemaFetcher` already sit) returns the stored form when `c.forms` has one and the provider has no live fetcher.
- [x] 3.2 Unit test: with `c.forms` nil (or returning not-found), a provider with no live fetcher still parks with `errNoSchemaFetcher`/`reasonSubmissionNotImplemented`, unchanged from today.
- [x] 3.3 Unit test: for a provider that DOES have a live fetcher, a non-nil `c.forms` returning a stored form is NOT used — the live fetcher is still preferred and called (call-counting fake), confirming this change does not alter behavior for Ashby/Workable/Greenhouse/Lever even though each has a row in `apply_forms` too.
- [x] 3.4 Scenario test: a Recruitee attempt with a stored form and an incomplete resolution reports `Unmapped` naming the actual missing fields, not the generic `reasonSubmissionNotImplemented`.
- [x] 3.5 Scenario test: a Recruitee attempt with a stored form and a fully-resolved plan still parks with `reasonSubmissionNotImplemented` (same message as today), asserting `plan.FullyResolved()==true` is reached and nothing further executes.
- [x] 3.6 Run `go vet -tags=integration ./...` and the full test suite for `internal/api/atsapply` and `cmd/auto-apply`.

## 4. Wrap-up

- [x] 4.1 Update `internal/api/atsapply/AGENTS.md`'s Recruitee note (currently: "Reaching Recruitee would need reading its already-captured form from storage instead of `applyform.Fetcher` — a real gap, not a decision, and out of scope here") to reflect that schema resolution now does this, while stating plainly that Recruitee still has no fill/submit path.
- [x] 4.2 `gofmt -l .`, `go vet ./...`, `go test ./...` clean before commit.
