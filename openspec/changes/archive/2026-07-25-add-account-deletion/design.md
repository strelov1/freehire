## Context

freehire holds a member's CV, their hosted mailbox and synced ATS mail, per-job
AI analyses, credits, referral proof PDFs, and a pseudonymous community identity —
and offers no way to delete any of it. This change adds the exit.

The database is already prepared for it. Nearly every user-scoped table declares
`REFERENCES users(id) ON DELETE CASCADE`, so a single `DELETE FROM users`
reaches `api_keys`, `user_identities`, `user_jobs`, `user_job_analysis`, `cvs`,
`credit_ledger`, `credit_balances`, `saved_searches`, `user_profiles`,
`subscriptions`, `subscription_matches`, `telegram_links`, `job_reminders`,
`reminder_settings`, `company_votes`, `emails`, `mailboxes`,
`gmail_connections`, `community_personas`, `referral_offers`,
`referral_requests`, `link_contributions`, and `job_submissions`. Moderator and
authoring trails (`jobs.created_by/updated_by`, `*.reviewed_by`,
`referral_offers.decided_by`, `referral_requests.acted_by`,
`thread_replies.author_user_id`) are `SET NULL` and stay that way.

Three things the cascade cannot do, and they are where the actual work is:

1. **S3 objects.** `resumes/<user_id>` (deterministic),
   `referral_offers.proof_object_key` (one per offer), and `emails.s3_key` (one
   per hosted message, nullable for Gmail-sourced rows). The latter two are
   knowable only while their rows exist.
2. **The Google grant.** `gmail_connections.refresh_token_enc` is a live OAuth
   grant. `gmailsync.Connector.Revoke` already exists and is called by
   `GmailDisconnect` (`internal/handler/gmail.go:118`); account deletion must use
   the same path.
3. **Sessions.** JWTs are stateless. `auth.RequireAuth` parses the token and
   trusts the subject, so after deletion a cookie on another device stays
   "valid": reads return empty and writes hit FK violations — 500s, not 401s.

One more pre-existing hazard surfaces here: `threads.author_user_id` is CASCADE,
so deleting an author would delete their thread *and every reply other members
wrote in it*. And `thread_replies.author_user_id` is already `SET NULL`, while
the reply renderer maps a blank handle to the AI persona
(`internal/handler/community.go:53-54`) — a departed member's words would be
served as a bot's. Both are fixed here rather than shipped as collateral damage.

## Goals / Non-Goals

**Goals:**

- `DELETE /api/v1/me`: cookie-only, email-confirmed, immediate, irreversible.
- Erase every user-owned row, every S3 object, and the Google grant — no orphans.
- Terminate sessions on all devices the moment the account is gone.
- Preserve community discussion, de-authored, without misattributing it.
- Failure modes that never leave PII behind in object storage.

**Non-Goals:**

- Soft-delete, grace period, undo, or "deactivate" — explicitly rejected by the
  requester; deletion is final.
- Data export before deletion (GDPR portability). Separate capability.
- Admin-initiated deletion of another member's account.
- Deleting derived aggregates that carry no user reference (view counts,
  engagement rollups) — they are not personal data.
- Reworking the moderator audit trails; `SET NULL` is the intended behaviour.

## Decisions

### D1: Objects first, rows second

**Decision.** The orchestration is: collect keys → revoke at Google
(best-effort) → delete every S3 object → `DELETE FROM users` (single statement,
cascade does the rest) → expire the cookie. An S3 failure aborts *before* any
row is deleted and returns `503`; the member can retry.

**Why.** The two possible partial states are not symmetric. If rows go first and
S3 fails, the CV and raw mail sit in the bucket forever with no key left to find
them — the exact promise this change makes, broken silently. If objects go first
and the `DELETE` fails, the rows point at objects that no longer exist: the
member sees a broken CV download until they retry, which is recoverable and
visible. Retrying is safe because object deletion is idempotent (S3 `DeleteObject`
on a missing key succeeds).

**Alternatives considered.** *Rows first, then objects* — smaller blast radius on
the happy path, unacceptable on the sad path (undeletable orphaned PII).
*Outbox table for deferred object cleanup* — survives crashes, but adds a queue,
a worker, and a new failure surface for a single-user operation; the retry-by-
the-member path covers the same ground at MVP scale. Note the seam: if deletion
volume ever justifies it, the collected key list is exactly the outbox payload.

### D2: `internal/accountdelete` owns the orchestration

**Decision.** A new package with one entry point,
`Service.Delete(ctx, userID) error`, depending on narrow interfaces (a repository
for the queries it needs, `blobstore.Store`, a revoker). The handler does auth,
confirmation, and cookie expiry; it does not sequence the erasure.

**Why.** The sequence spans Postgres, S3, and Google and has ordering
constraints that must be unit-testable with fakes — which is impossible inside a
Fiber handler. It also matches how `internal/resume` and `internal/userjob`
already factor use cases out of `internal/handler`.

**Alternatives.** All in `me_delete.go` — fewer files, but the ordering logic
(the part most worth testing) would only be reachable through integration tests.

### D3: Storage-disabled and Gmail-disabled deployments still delete

Both S3 (`blobstore.New` returns `nil, nil` when unconfigured) and Gmail are
optional. With storage off there is nothing to erase, and deletion proceeds; with
Gmail off there is nothing to revoke. The service treats a nil dependency as
"nothing to do", never as an error — otherwise local and self-hosted deployments
could not delete accounts at all.

### D4: Session termination rides on the existing token-version check

**Decision.** Nothing new. `RequireAuth` and the cookie path of
`RequireAuthOrKey` already resolve a session through `resolveSession`, which
loads the account's `token_version` and **fails closed on any load error**. For a
deleted account that read is `ErrNoRows`, so every device's cookie stops
authenticating the moment the row is gone. This change only makes the guarantee
explicit in the spec and covers it with a test.

**History.** This was first built as a separate `UserExists` check in the
middleware, before session revocation landed on `main`. Rebasing showed the two
were the same query answering the same question, so the added interface was
dropped rather than kept alongside — one lookup, one meaning.

**Alternatives.** *A second existence lookup next to the version load* — a
redundant round-trip per authenticated request for information already in hand.
*A tombstone table plus an in-process cache* — cache-invalidation semantics and a
window where a deleted account still works; not worth it without a measured
problem. *Do nothing and let handlers fail* — leaves 500s where 401 is the honest
answer, and was explicitly rejected.

**Note.** The API-key path needs nothing either: `api_keys` cascades with the
owner, so a deleted user's key stops resolving on its own.

### D5: Community — `SET NULL` plus a distinct deleted-author marker

**Decision.** A migration flips `threads_author_user_id_fkey` to `ON DELETE SET
NULL` and makes `threads.author_user_id` nullable. Every read query that joins
`community_personas` becomes a `LEFT JOIN` — an inner join would silently drop
de-authored threads from listings. The wire shape keeps its single `author`
string; a de-authored thread or reply renders a reserved marker distinct from
both live handles and the existing `aiAuthor` ("AI") constant, chosen so it
cannot collide with a minted handle.

**Why.** CASCADE on threads makes one member's exit destroy other members'
replies — content they own and did not consent to lose. `SET NULL` keeps the
discussion and drops the identity, which is what "anonymize" means here. The
renderer fix is not optional: with `is_ai=false` and an empty handle, current
code labels the reply "AI".

**Alternatives.** *Delete the member's community content outright* — honest in a
different way, but takes other people's replies with it (the thread is the
container). *Reassign to a shared "deleted" persona row* — one more row to
maintain and a fake handle that could be confused for a real member.

### D6: Confirmation by typed email, cookie-only

Email comparison is case-insensitive, consistent with `GetUserByEmail`'s `lower`
lookup. The route is registered under `auth.RequireAuth`, not `keyAuth`, so a
leaked API key cannot destroy its owner's account — the same reasoning that
already makes key management cookie-only (`internal/handler/handler.go:597-599`).

## Risks / Trade-offs

- **Objects deleted, rows retained (D1 sad path)** → The member retries; the
  operation is idempotent. Logged with the user id so the state is diagnosable.
- **A concurrent request writes rows mid-deletion** (e.g. a background Gmail sync
  inserting mail while the delete runs) → The insert either lands before the
  `DELETE` and is cascaded away, or lands after and fails its FK — no orphan
  survives, because every user-scoped table has the FK. The one gap is an S3
  object written between key collection and row deletion; accepted as a narrow
  race, and mitigated by collecting keys immediately before the delete.
- **Per-request user lookup adds latency (D4)** → Primary-key hit on a pooled
  connection; the interface seam keeps a cache one refactor away.
- **Migration on a live prod volume** → `migrations/` is applied by initdb only on
  a fresh volume; this one must be run by hand on prod BEFORE the binary ships,
  like 0014/0015 before it. An unapplied migration means deleting a thread author
  still cascades their thread away — data loss, not a 500, so ordering matters.
- **The community marker leaks "this account was deleted"** → It reveals that an
  account existed and is gone, but not who; the handle is already pseudonymous
  and is removed with the persona row.

## Migration Plan

1. Ship the migration file; apply it by hand on prod (`ALTER TABLE threads ALTER
   COLUMN author_user_id DROP NOT NULL` + drop/recreate the FK as `SET NULL`)
   **before** deploying the binary.
2. Deploy the binary (new endpoint, middleware check, `LEFT JOIN` reads).
3. Ship the web surface.

**Rollback.** The endpoint and middleware check revert with the binary. The
migration is a permissive widening (nullable column, weaker FK action) that older
code tolerates, so it does not need to be rolled back — and reversing it would
be unsafe once any thread has a null author.

## Open Questions

None blocking. The exact deleted-author marker string is a copy decision to
settle when the web surface is written.
