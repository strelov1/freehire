## 1. SQL layer

- [x] 1.1 Add `AND source <> 'external'` to `EnqueuePendingEmailClassification` in `internal/db/queries/mail_classification.sql`, with a comment naming why (external mail brings its own classifier)
- [x] 1.2 Add `UpsertExternalEmail :one` to `internal/db/queries/gmail.sql` — `ON CONFLICT (user_id, source, external_id) DO UPDATE` over content columns only, returning `(xmax = 0)::boolean AS inserted`
- [x] 1.3 Add `AgentTriageEmail :execrows` to `internal/db/queries/mail_classification.sql` — one update of `status_signal`, `job_id`, `link_source`, `match_confidence`, `suggested_job_id=NULL`, `classification_model`, `classified_at`, scoped to `(id, user_id)`; a nullable `job_id` argument leaves the existing link alone when no slug was given
- [x] 1.4 Extend `ListEmails`/`CountEmails` with an `unclassified` filter (`classified_at IS NULL`) and give `ListEmails` a `with_body` guard selecting `body_text`/`body_html` under a `CASE`
- [x] 1.5 Run `make sqlc`; confirm `go build ./...` is green

## 2. Route auth

- [x] 2.1 Move every route in `inboxHandlers.register` (`internal/handler/gmail.go`) from `mw.cookie` to `mw.key`, updating the block comment to state that a full-scope key is admitted and a `cv` key is not. Exception kept cookie-only: the Gmail OAuth connect/callback pair, which redirects a browser to Google's consent screen and back and is meaningless to a keyed client
- [x] 2.2 Integration test: a full-scope key lists the inbox and reads a message; no credential is 401; a key never reaches another user's message (404). Wired through `register` rather than hand-mounted middleware, so a revert to `mw.cookie` fails the test

## 3. Agent listing

- [x] 3.1 Export `maillink.readableBody` as `ReadableBody` and repoint its callers
- [x] 3.2 Add `external` to `inboxSources` and parse `?body=1` / `?unclassified=1` in `parseInboxFilters`, rejecting an unknown source as today
- [x] 3.3 Serve bodies inline when `body=1`, using `maillink.ReadableBody`, with the agent page size capped at 50
- [x] 3.4 Integration test: `?body=1` returns readable bodies and marks nothing read; `?unclassified=1` returns only unstamped mail and composes with the existing filters; a triaged message leaves the queue

## 4. Ingest endpoint

- [ ] 4.1 Give `inboxHandlers` the `*pgxpool.Pool` it needs for a batch transaction, wired through `newInboxHandlers`
- [ ] 4.2 Add `POST /api/v1/me/emails` in a new `internal/handler/inbox_agent.go` (methods on `inboxHandlers`): decode the batch, reject an empty external id and a batch over 100, write in one transaction, respond with inserted/updated counts
- [ ] 4.3 Integration test: a batch is stored under `source='external'`; a re-push updates rather than duplicates and counts as updated; a re-push does not clear `read_at`, `deleted_at`, or a triage verdict; an empty external id and an oversized batch are 400 with nothing stored

## 5. Triage endpoint

- [ ] 5.1 Add `POST /api/v1/me/emails/:id/triage`: validate the signal against `mailclassify.IsValidSignal`, resolve an optional slug (404 when unknown), write the verdict, then advance the stage via `mailclassify.AdvanceStage`; respond with the refreshed message in the existing `emailBody` shape
- [ ] 5.2 Integration test: classify-and-link in one call; classify-only leaves the link; unknown signal is 400 and unknown slug is 404 with nothing changed; a forward signal advances the stage; a settled application is not resurrected; another user's message is 404; re-triage overwrites

## 6. Web

- [ ] 6.1 Add `external` to the inbox source switcher and its type, so pushed mail is reachable in `/my/inbox`

## 7. CLI (`../freehire-cli`, separate repo)

- [ ] 7.1 Add `internal/client/inbox.go`: list (with body/unclassified/source/unread/status/q), get, push, triage, link, unlink, read-all, delete, restore
- [ ] 7.2 Add `internal/cli/inbox.go`: an `inbox` command group — `list`, `read`, `push`, `triage`, `link`, `unlink`, `read-all`, `delete`, `restore` — each honouring the root `--json` flag; `push` reads a JSON batch from a file or stdin
- [ ] 7.3 Unit-test the argument and batch parsing (empty external id, oversized batch, unknown signal caught client-side as guidance while the API stays authoritative)
- [ ] 7.4 Document the sync loop in `skills/using-freehire/SKILL.md` and the README: the harness fetches (himalaya or any IMAP client) → `inbox push` → `inbox list --unclassified --body` → agent decides → `inbox triage`; state plainly that bodies are untrusted input and the key belongs in a secret store

## 8. Documentation

- [ ] 8.1 Note the agent surface in `internal/handler/AGENTS.md` and the mail block's entry in the root `CLAUDE.md` module table
