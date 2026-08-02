## 1. SQL layer

- [x] 1.1 Add `ListEmailsForRecall` to `internal/db/queries/mail_linking.sql`: the caller's
      live mail (`deleted_at IS NULL`) attached to nothing (`job_id IS NULL AND
      application_id IS NULL` — both, because a message auto-linked before its application
      row existed holds the first without the second), received at or after a given
      window, OLDEST first, with a limit — oldest first so the cap trims the far tail
      rather than the acknowledgement. Carries the columns the adjudication needs —
      id, from_addr, from_name, subject, body_text, body_html, received_at, ical_uid — and
      nothing else.
- [x] 1.2 Add `SuggestJobForEmail` to the same file: set `suggested_job_id` and
      `match_confidence` for one message owned by the caller, under the same two IS NULL
      predicates plus `deleted_at IS NULL`, so a linked message is unreachable from this
      path. The suggested id is cast to `bigint` so a zero value cannot silently clear the
      suggestion. Comment says the predicates are the guard, not an optimisation.
- [x] 1.3 Run `make sqlc` and confirm `internal/db` regenerates cleanly. No migration.

## 2. The service

- [x] 2.1 Create `internal/mailrecall` with its package doc: what the pull direction is, why
      a proposal is never a link, and the `body_text`-is-empty-for-HTML-only trap that
      dictates the net's shape.
- [x] 2.2 Define the narrow `Store` interface (the two queries) and the `Candidate` /
      `Proposal` / `Result` types. `Result` carries `Scanned`, the proposed message ids with
      confidences, and the invitation count.
- [x] 2.3 Implement the net: window from seven days before `applied_at` to ninety days
      after, oldest first, cap of 40 candidates, bodies through `maillink.ReadableBody`
      truncated to 800 runes. Pure and testable without a store.
- [x] 2.4 Implement the adjudication call — one batched, schema-bound request through the
      `llm` provider, per-candidate belongs/does-not with a confidence. Test-first: an id
      absent from the batch is discarded; an empty candidate set makes no call.
- [x] 2.5 Implement `Recall`: net → adjudicate → write suggestions through the store,
      returning `Result`. A model failure propagates; nothing is written on that path.
- [x] 2.6 Write `TestMailRecallCannotLink` — no path in the package sets `application_id`,
      in the manner of `calmatch.Tier.Links()`.
- [x] 2.7 Write the remaining unit tests against a fake store and a fake provider: an
      HTML-only message reaches the model with a non-empty body; a message whose suggestion
      names another application is overwritten; a linked message never enters the net.

## 3. HTTP

- [x] 3.1 Add `POST /me/tracking/:slug/mail-recall` beside the follow-up routes in
      `internal/handler/gmail.go`, under `mw.key`. Resolve the slug to the caller's
      application; 404 for someone else's, for a missing one, and for a tracking row with
      `applied_at IS NULL`.
- [x] 3.2 Render the response as `{"data": {"scanned", "suggested", "invitations"}}`. The
      proposed rows come from the run itself, not from a re-read: nothing fetches emails by
      an id list, and `GET /me/emails/:id` would mark each one READ. A model failure renders
      502 `{"error": ...}`; an unconfigured model renders 503.
- [x] 3.3 Resolve the caller's gateway credential through `internal/llmkey` and tag the call
      `feature:mail-recall`; unresolvable credential falls back to the service one.
- [x] 3.4 Add the integration test (`//go:build integration`): 404 for another user's
      application, a suggestion is written for the caller's, a linked message is unchanged,
      and no `application_events` row appears.

## 4. Web

- [x] 4.1 Wire the response type into `web/src/lib/types.ts` and the call into
      `web/src/lib/api.ts`. `cmd/gen-contracts` emits domain contracts, not handler wire
      shapes, so it had nothing to add here — run for drift, no diff.
- [x] 4.2 Add the button and the result list to the Emails tab of
      `web/src/lib/components/JobDrawer.svelte`, reusing the existing email row and the
      existing confirm/reject calls. Disabled while in flight; the error path shows the
      server's message rather than an empty list.
- [x] 4.3 When the result carries invitations, say that the meetings arrive after the next
      calendar sync.
- [x] 4.4 Run `pnpm run check` and `pnpm run lint` in `web/`, and the design-system
      ratchets (`check:tokens`, `check:adoption`) — the token gate counts arbitrary values
      per file, so the new row uses `text-xs` rather than copying its neighbour's
      `text-[11px]` and raising the baseline.

## 5. Documentation and gates

- [x] 5.1 Add the pull direction to `docs/agents/mail-stack.md`: the action exists, it
      proposes and never links, and the calendar follows from the mail with no calendar code.
- [x] 5.2 Run `go build ./...`, `go vet ./...`, `go test ./...`, and
      `go vet -tags=integration ./...` before pushing.
