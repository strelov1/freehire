## 1. Shared board recogniser

- [x] 1.1 Move `RecognizeBoard`, the `atsBoards` table and the extraction modes from
  `internal/contribution/board.go` into a new `internal/atsboard` package, keeping the
  existing table tests with them
- [x] 1.2 Repoint `internal/contribution` at `internal/atsboard`; leave the job-id helpers
  (`greenhouseJobID`, `ashbyJobID`) in `contribution`, they are service logic
- [x] 1.3 `go build ./... && go vet ./... && go test ./internal/atsboard/ ./internal/contribution/`

## 2. Schema

- [x] 2.1 New migration: drop `UNIQUE (source, board)` on `link_contributions`, add
  `surface text NOT NULL DEFAULT 'unknown'`
- [x] 2.2 Update `internal/db/queries/*.sql`: record with a surface, and a query answering
  "is this the first row for `(source, board)`" for reward gating
- [x] 2.3 `make sqlc`, then `go build ./...`

## 3. Reward gating without the constraint

- [x] 3.1 Failing integration test: two concurrent submissions of one new board award
  exactly one reward and record both rows
- [x] 3.2 Replace `ErrBoardAlreadyContributed`-by-constraint with the in-transaction
  first-row check; a repeat board records and returns "recorded, not rewarded"
- [x] 3.3 Update `contribution` unit tests for the new duplicate semantics
- [x] 3.4 `go test ./internal/contribution/` and `go test -tags=integration ./internal/contribution/`

## 4. Board-coverage link source

- [x] 4.1 Failing test: a vacancy on a recognised board with no host-scoped adapter
  resolves with the identity the ingest crawl would produce
- [x] 4.2 Implement the adapter: recognise `(source, board)` → look up the ingest adapter
  in `sources.All` → `Fetch` the tenant board → select the posting matching the link
- [x] 4.3 Failing test then implementation: a board fetched successfully but not containing
  the link resolves nothing (no error); a fetch failure is an error
- [ ] 4.4 Add the `boardresolve` network fallback for vanity domains in `linkimport` (not in
  the adapter: `ResolveLinks` picks a single adapter via `Find` and never tries the next,
  so a network guess cannot live in `Match`), guarding against taking a platform apex
  (e.g. `app.teamtailor.com`) as a tenant board
- [x] 4.5 Register it in the importer's registry after the host-scoped adapters and before
  `generic`; test that a Greenhouse link still takes the dedicated adapter
- [x] 4.6 `go test ./internal/linksource/ ./internal/linkimport/`

## 5. Attribution through the import path

- [ ] 5.1 Failing test: an import records the submitting user and surface
- [ ] 5.2 Give `linkimport.Import` a submitter+surface parameter; thread it to the
  contribution record
- [ ] 5.3 Update `cmd/resolve-url` (the operator surface) to pass an explicit surface
- [ ] 5.4 `go test ./internal/linkimport/` and `go test -tags=integration ./internal/linkimport/`

## 6. Intake sequence: the four outcomes

- [ ] 6.1 Failing handler tests for each outcome: `found`, `tracked`, `imported`, `queued`
- [ ] 6.2 Add the `tracked` branch to `ResolveJob`: board crawled + posting absent → import
  and answer with `company_slug`
- [ ] 6.3 Move contribution recording out of the `imported == false` branch so a successful
  import also queues an uncrawled board
- [ ] 6.4 Accept and validate the `surface` request field; unknown/absent records `unknown`
- [ ] 6.5 `go test ./internal/handler/` and `go test -tags=integration ./internal/handler/`

## 7. Telegram onto the unified sequence

- [ ] 7.1 Failing test: a linked user sending a readable vacancy link gets a reply carrying
  the posting URL
- [ ] 7.2 Replace the direct `contribution.Submit` call with the intake service; map the
  four outcomes to bot replies
- [ ] 7.3 Raise `telegramContribTimeout` to cover a board fetch and reply on timeout rather
  than going silent
- [ ] 7.4 `go test ./internal/handler/ -run Telegram` and the tagged integration test

## 8. Website

- [ ] 8.1 Remove `POST /me/contributions`; keep the `GET` list
- [ ] 8.2 Point `web/src/lib/api.ts` and `/contribute` at the intake endpoint with
  `surface: "web"`, rendering all four outcomes
- [ ] 8.3 Show the surface column in `/my/contributions`
- [ ] 8.4 `cd web && pnpm lint && pnpm build`

## 9. freehire-cli (`../freehire-cli`)

- [ ] 9.1 Remove `newSubmitCmd` and its `submit` registration; keep `submissions`
  (moderator review is a separate feature)
- [ ] 9.2 Add `freehire contribute <url>` calling the intake endpoint with
  `surface: "cli"`, printing the outcome and the posting URL when there is one
- [ ] 9.3 Add `freehire contributions` listing the caller's own
- [ ] 9.4 Tests for both commands, including `--json`
- [ ] 9.5 Document both in `skills/using-freehire` and the README command list
- [ ] 9.6 `go build ./... && go test ./...`

## 10. freehire-extension (`../freehire-extension`)

- [ ] 10.1 Send `surface: "extension"` from `resolveJob`
- [ ] 10.2 Add a `tracked` case to `resolveNotice` and its test
- [ ] 10.3 `pnpm test && pnpm build`

## 11. Coverage audit and close-out

- [ ] 11.1 Produce the coverage table — hosts recognised by `atsboard`, providers with an
  ingest adapter, and hosts with a dedicated link-source adapter — and record the gaps
  worth a follow-up change
- [ ] 11.2 Update `internal/contribution/AGENTS.md`, `internal/linksource/AGENTS.md` and
  the new `internal/atsboard` docs for the moved recogniser and the new flow
- [ ] 11.3 Update the `onboard-contributions` skill to group the queue by `(source, board)`
  now that a board may have several rows
- [ ] 11.4 Full check: `go build ./... && go vet ./... && go test ./...`
