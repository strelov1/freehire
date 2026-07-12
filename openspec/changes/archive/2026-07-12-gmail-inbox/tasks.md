## 1. Data model

- [x] 1.1 Add migration for `gmail_connections` (user_id UNIQUE, email, refresh_token_enc, status, connected_at, last_synced_at, sync_cursor) and `emails` (user_id, gmail_msg_id UNIQUE, thread_id, from_addr, from_name, subject, subject_norm, body_text, body_html, received_at; indexes on (user_id, subject_norm) and (user_id, received_at))
- [x] 1.2 Add sqlc queries: upsert email (idempotent on gmail_msg_id), list groups by subject_norm, list messages by group, get message by id+user, get/upsert connection, clear connection + purge emails; regenerate `internal/db`

## 2. Pure helpers

- [x] 2.1 Subject normalization (strip Re:/Fwd: + locale variants, trim/collapse, case-insensitive) as a pure, table-tested function
- [x] 2.2 ATS sender-domain registry (curated list) + a Gmail `q=from:(…)` builder

## 3. Token encryption

- [x] 3.1 Refresh-token encrypt/decrypt at rest (AEAD with a key from env); round-trip unit test; never log plaintext

## 4. Connect flow (incremental OAuth)

- [x] 4.1 `GET /api/v1/me/gmail/connect` — start incremental consent for `gmail.readonly` on the existing Google client (state cookie)
- [x] 4.2 `.../callback` — verify state, exchange code, store encrypted refresh token, mark connected; degrade cleanly for a testing-mode block
- [x] 4.3 `GET /me/gmail` status + `DELETE /me/gmail` disconnect (revoke with Google + purge token and synced mail)
- [x] 4.4 Handler tests (integration) for connect callback storage, status, and disconnect-purges-all

## 5. Gmail API client

- [x] 5.1 Define a `GmailReader` interface (list ATS message ids since cursor; get full message) + a fake for tests
- [x] 5.2 Implement it over the Gmail API using a per-user access token minted from the stored refresh token; token refresh isolated behind an adapter

## 6. Sync worker

- [x] 6.1 `cmd/gmail-sync`: for each connected user — list ATS mail since cursor → get → normalize subject → upsert `emails` → advance cursor; best-effort per user (revoked token → mark needs-reconsent, continue), unit-tested with fakes
- [x] 6.2 Wire the worker for cron (run-once-and-exit), gated on config; document the schedule

## 7. Inbox API

- [x] 7.1 `GET /me/inbox` — groups by normalized subject (count, latest, senders), caller-scoped
- [x] 7.2 `GET /me/inbox/groups/:key` — a group's messages, newest first
- [x] 7.3 `GET /me/emails/:id` — a single message body, caller-scoped (404 for another user's)
- [x] 7.4 Handler tests (integration) for grouping + caller-scoping
- [x] 7.5 Inbox search: optional ?q= filtering messages by subject/sender/body, grouped; SPA search box (debounced)

## 8. SPA (web/)

- [x] 8.1 `/my/inbox` page: Connect Gmail button when not connected; on connect, the subject-grouped list
- [x] 8.2 Expandable group → message list → sandboxed-iframe reading pane
- [x] 8.3 Disconnect control + wire the page into nav

## 9. Config + Google setup

- [x] 9.1 `internal/config`: token-encryption key (`GMAIL_TOKEN_KEY`), Gmail scope constant; add the incremental scope to the connect flow only
- [x] 9.2 Document the Google Cloud setup (enable Gmail API, add `gmail.readonly` scope, add test users) + `.env.example`

## 10. Verify

- [ ] 10.1 `go build ./... && go vet ./...`, `go test ./...`, integration tests, web check pass
- [x] 10.2 End-to-end: connect a test-user Gmail, sync pulls ATS mail, `/my/inbox` shows it grouped by subject with readable bodies

## 11. UX additions (during live testing)

- [x] 11.1 Inbox pagination (20 groups/page, Load more) + total count
- [x] 11.2 available flag on status + SPA hides Connect when the feature is unconfigured
- [x] 11.3 Manual Sync button (POST /me/gmail/sync, background sync + poll)
