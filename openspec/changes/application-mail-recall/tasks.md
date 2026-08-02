## 1. SQL layer

- [ ] 1.1 Add `ListEmailsForRecall` to `internal/db/queries/mail_linking.sql`: the caller's
      live mail (`deleted_at IS NULL`) attached to no application (`application_id IS NULL`),
      received at or after a given instant, newest first, with a limit. Carries the columns
      the adjudication needs — id, from_addr, from_name, subject, body_text, body_html,
      received_at, ical_uid — and nothing else.
- [ ] 1.2 Add `SuggestApplicationForEmail` to the same file: set `suggested_job_id` and
      `match_confidence` for one message owned by the caller, `WHERE application_id IS NULL`
      so a linked message is unreachable from this path. Comment says the predicate is the
      guard, not an optimisation.
- [ ] 1.3 Run `make sqlc` and confirm `internal/db` regenerates cleanly. No migration.

## 2. The service

- [ ] 2.1 Create `internal/mailrecall` with its package doc: what the pull direction is, why
      a proposal is never a link, and the `body_text`-is-empty-for-HTML-only trap that
      dictates the net's shape.
- [ ] 2.2 Define the narrow `Store` interface (the two queries) and the `Candidate` /
      `Proposal` / `Result` types. `Result` carries `Scanned`, the proposed message ids with
      confidences, and the invitation count.
- [ ] 2.3 Implement the net: window opening seven days before `applied_at`, cap of 40
      candidates, bodies through `maillink.ReadableBody` truncated to 800 runes. Pure and
      testable without a store.
- [ ] 2.4 Implement the adjudication call — one batched, schema-bound request through the
      `llm` provider, per-candidate belongs/does-not with a confidence. Test-first: an id
      absent from the batch is discarded; an empty candidate set makes no call.
- [ ] 2.5 Implement `Recall`: net → adjudicate → write suggestions through the store,
      returning `Result`. A model failure propagates; nothing is written on that path.
- [ ] 2.6 Write `TestMailRecallCannotLink` — no path in the package sets `application_id`,
      in the manner of `calmatch.Tier.Links()`.
- [ ] 2.7 Write the remaining unit tests against a fake store and a fake provider: an
      HTML-only message reaches the model with a non-empty body; a message whose suggestion
      names another application is overwritten; a linked message never enters the net.

## 3. HTTP

- [ ] 3.1 Add `POST /me/tracking/:slug/mail-recall` beside the follow-up routes in
      `internal/handler/gmail.go`, under `mw.key`. Resolve the slug to the caller's
      application; 404 for someone else's, for a missing one, and for a tracking row with
      `applied_at IS NULL`.
- [ ] 3.2 Render the response as `{"data": {"scanned", "suggested", "invitations"}}`, with
      `suggested` in the listing's own `inbox.Message` projection. A model failure renders
      502 `{"error": ...}`.
- [ ] 3.3 Resolve the caller's gateway credential through `internal/llmkey` and tag the call
      `feature:mail_recall`; unresolvable credential falls back to the service one.
- [ ] 3.4 Add the integration test (`//go:build integration`): 404 for another user's
      application, a suggestion is written for the caller's, a linked message is unchanged,
      and no `application_events` row appears.

## 4. Web

- [ ] 4.1 Run `cmd/gen-contracts` and wire the new response type into `web/src/lib/api`.
- [ ] 4.2 Add the button and the result list to the Emails tab of
      `web/src/lib/components/JobDrawer.svelte`, reusing the existing email row and the
      existing confirm/reject calls. Disabled while in flight; the error path shows the
      server's message rather than an empty list.
- [ ] 4.3 When the result carries invitations, say that the meetings arrive after the next
      calendar sync.
- [ ] 4.4 Run `pnpm run check` and `pnpm run lint` in `web/`.

## 5. Documentation and gates

- [ ] 5.1 Add the pull direction to `docs/agents/mail-stack.md`: the action exists, it
      proposes and never links, and the calendar follows from the mail with no calendar code.
- [ ] 5.2 Run `go build ./...`, `go vet ./...`, `go test ./...`, and
      `go vet -tags=integration ./...` before pushing.
