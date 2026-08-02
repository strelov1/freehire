## 1. The search, in gmailsync

- [x] 1.1 Add `BuildRecallQuery(company, role string, since, until time.Time) string` beside
      `BuildQuery` in `internal/gmailsync/senders.go`, with unit tests: the employer clause
      carries the de-spaced variant (`Blend 360` → also `Blend360`), the gate carries the
      hiring vocabulary AND `filename:ics` AND the quoted role, and quotes in either field
      are stripped rather than allowed to break the query.
- [x] 1.2 Add `MailboxSearcher` + `apiReader.Search` — an arbitrary query returning whole
      messages, capped, one page. A separate interface rather than a method on
      `GmailReader`: that one belongs to the sync worker and its query is the sync's own.
      Behaviour is covered at the `mailrecall` layer, where the fake lives — this package
      tests parsers, not its HTTP calls.
- [x] 1.3 Add a single-message fetch-and-store that reuses the sync's existing store path,
      idempotent on `(source, external_id)`, so importing a message the worker had already
      stored links the existing row rather than creating a second.

## 2. The second candidate source, in mailrecall

- [ ] 2.1 Define a `Mailbox` interface — search for an employer's mail, and import one
      message by provider id — and note in the doc comment that it is the seam the fallback
      is chosen against. Nil means no searchable mailbox.
- [ ] 2.2 Make `Recall` pick its source: search when a `Mailbox` is present, `ListForRecall`
      otherwise. Test-first: with a mailbox the store is never asked for candidates; without
      one the store is.
- [ ] 2.3 Carry the provider message id on `Proposal` and stop writing suggestions on the
      search path. Test that a search-path run performs zero writes.
- [ ] 2.4 Add `ErrSearch` beside `ErrModel` so the handler can tell "could not look" from
      "could not judge", and test that a search failure is neither swallowed nor reported as
      an empty result.
- [ ] 2.5 Extend `TestMailRecallCannotLink` and the source scan to cover the new interface:
      a `Mailbox` that could link would break the rule as surely as a `Store` that could.

## 3. Link-and-import, in the handler

- [ ] 3.1 Add `POST /me/tracking/:slug/mail-recall/link` (body: provider message id) under
      `mw.key`: import the message, then link it through `internal/inbox` so the ledger
      reconcile runs exactly as it does today. 404 for an application that is not the
      caller's; 502 when the mailbox cannot be read.
- [ ] 3.2 Wire the `Mailbox` implementation in `handler.go`, present only when the Gmail
      client and token cipher are configured — the same condition `cmd/gmail-sync` checks.
- [ ] 3.3 Integration test (`//go:build integration`): a search-path sweep writes nothing;
      linking a proposal imports and links it; linking a message already stored does not
      duplicate it; another user's application is 404.

## 4. Web

- [ ] 4.1 Point the Link button at the new call and carry the provider id through the
      response type.
- [ ] 4.2 Say on the panel that the sweep searches the mailbox rather than importing it —
      the privacy boundary should be legible where it is crossed.
- [ ] 4.3 Run `pnpm run check`, `pnpm run lint`, and the design-system ratchets.

## 5. Documentation and gates

- [ ] 5.1 Update `docs/agents/mail-stack.md`: the pull direction now searches the mailbox,
      the gate's two measured halves, and the rule that nothing unconfirmed is stored.
- [ ] 5.2 Record the two findings this change routes around rather than fixes — the 739
      messages `BuildQuery` never fetches, and `ExtractCompany`'s five `to`-only subject
      templates — where the next reader will meet them.
- [ ] 5.3 Run `go build ./...`, `go vet ./...`, `go test ./...`, and
      `go vet -tags=integration ./...` before pushing.
