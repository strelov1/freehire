## 1. Shared board recogniser

- [x] 1.1 Move `RecognizeBoard`, the `atsBoards` table and the extraction modes from
  `internal/contribution/board.go` into a new `internal/atsboard` package, keeping the
  existing table tests with them
- [x] 1.2 Repoint `internal/contribution` at `internal/atsboard`; leave the job-id helpers
  (`greenhouseJobID`, `ashbyJobID`) in `contribution`, they are service logic
- [x] 1.3 `go build ./... && go vet ./... && go test ./internal/atsboard/ ./internal/contribution/`

## 2. Schema

- [x] 2.1 Migration `0050`: add `surface text NOT NULL DEFAULT 'unknown'`. Revised during
  implementation — the plan was to drop `UNIQUE (source, board)`, but PR #1218 (migration
  0049, already in prod) narrowed it to the live statuses instead, keeping one row and one
  reward per board on anti-farming grounds. This change defers to that.
- [x] 2.2 Update `internal/db/queries/*.sql`: record with a surface, and a query answering
  "is this the first row for `(source, board)`" for reward gating
- [x] 2.3 `make sqlc`, then `go build ./...`

## 3. Reward gating without the constraint

- [x] 3.1 Superseded by #1218: the unique index over live statuses keeps the race safe, so
  `TestRecordConcurrentDuplicateRecordsOnce` (from main) covers it
- [x] 3.2 Superseded — `ErrBoardAlreadyContributed` stays constraint-backed; the advisory lock
  and its two queries were removed when this change rebased onto #1218
- [x] 3.3 Update `contribution` unit tests for the new duplicate semantics
- [x] 3.4 `go test ./internal/contribution/` and `go test -tags=integration ./internal/contribution/`

## 4. Board-coverage link source

- [x] 4.1 Failing test: a vacancy on a recognised board with no host-scoped adapter
  resolves with the identity the ingest crawl would produce
- [x] 4.2 Implement the adapter: recognise `(source, board)` → look up the ingest adapter
  in `sources.All` → `Fetch` the tenant board → select the posting matching the link
- [x] 4.3 Failing test then implementation: a board fetched successfully but not containing
  the link resolves nothing (no error); a fetch failure is an error
- [x] 4.4 Add the `boardresolve` network fallback for vanity domains in `linkimport` (not in
  the adapter: `ResolveLinks` picks a single adapter via `Find` and never tries the next,
  so a network guess cannot live in `Match`), guarding against taking a platform apex
  (e.g. `app.teamtailor.com`) as a tenant board
- [x] 4.5 Register it in the importer's registry after the host-scoped adapters and before
  `generic`; test that a Greenhouse link still takes the dedicated adapter
- [x] 4.6 `go test ./internal/linksource/ ./internal/linkimport/`

## 5. Attribution

Revised during implementation. The plan was to give `linkimport.Import` a submitter and have
it record the contribution itself. That would make `linkimport` depend on `contribution`
purely to carry an attribution, while the handler already calls both — so the importer stays
a pure importer and attribution rides on the intake call, which is task 6.3. `cmd/resolve-url`
needs no surface for the same reason: it imports without recording.

- [x] 5.1 Keep `linkimport` free of `contribution`; attribution is carried by the intake call
- [x] 5.2 `surface` reaches the store through `contribution.SubmitInput` (done in group 3)

## 6. Intake sequence: the four outcomes

- [x] 6.1 Failing handler tests for each outcome: `found`, `tracked`, `imported`, `queued`
- [x] 6.2 Add the `tracked` branch to `ResolveJob`: board crawled + posting absent → import
  and answer with `company_slug`
- [x] 6.3 Move contribution recording out of the `imported == false` branch so a successful
  import also queues an uncrawled board
- [x] 6.4 Accept and validate the `surface` request field; unknown/absent records `unknown`
- [x] 6.5 `go test ./internal/handler/` and `go test -tags=integration ./internal/handler/`

## 7. Telegram onto the unified sequence

- [x] 7.1 Failing test: a linked user sending a readable vacancy link gets a reply carrying
  the posting URL
- [x] 7.2 Replace the direct `contribution.Submit` call with the intake service; map the
  four outcomes to bot replies
- [x] 7.3 Raise `telegramContribTimeout` to cover a board fetch and reply on timeout rather
  than going silent
- [x] 7.4 `go test ./internal/handler/ -run Telegram` and the tagged integration test

## 8. Website

- [x] 8.1 Remove `POST /me/contributions`; keep the `GET` list
- [x] 8.2 Point `web/src/lib/api.ts` and `/contribute` at the intake endpoint with
  `surface: "web"`, rendering all four outcomes
- [x] 8.3 Show the surface column in `/my/contributions`
- [x] 8.4 `cd web && pnpm lint && pnpm build`

## 9. freehire-cli (`../freehire-cli`)

- [x] 9.1 Remove `newSubmitCmd` and its `submit` registration; keep `submissions`
  (moderator review is a separate feature)
- [x] 9.2 Add `freehire contribute <url>` calling the intake endpoint with
  `surface: "cli"`, printing the outcome and the posting URL when there is one
- [x] 9.3 Add `freehire contributions` listing the caller's own
- [x] 9.4 Tests for both commands, including `--json`
- [x] 9.5 Document both in `skills/using-freehire` and the README command list
- [x] 9.6 `go build ./... && go test ./...`

## 10. freehire-extension (`../freehire-extension`)

- [x] 10.1 Send `surface: "extension"` from `resolveJob`
- [x] 10.2 Add a `tracked` case to `resolveNotice` and its test
- [x] 10.3 `pnpm test && pnpm build`

## 11. Coverage audit and close-out

- [x] 11.1 Coverage audit: 49 hosts / 45 providers recognised by `atsboard`, 153 ingest
  adapters, 8 host-scoped link adapters + generic. **45 of 45 recognised providers have an
  ingest adapter**, so board coverage reaches all of them — single-page coverage went from 8
  adapters to 45 providers. The audit found one real defect (`factorialhr` mapped to a source
  that does not exist; fixed). Remaining gap is vanity domains, handled by `boardresolve`
  rather than by the recogniser.
- [x] 11.2 Update `internal/contribution/AGENTS.md`, `internal/linksource/AGENTS.md` and
  the new `internal/atsboard` docs for the moved recogniser and the new flow
- [x] 11.3 Update the `onboard-contributions` skill to group the queue by `(source, board)`
  now that a board may have several rows
- [x] 11.4 Full check: `go build ./... && go vet ./... && go test ./...`
