# internal/handler — HTTP Handlers

Fiber HTTP handlers: feature handler structs, route registration, auth surface, user job endpoints, error rendering.

## Architecture

- `API` (`handler.go`) holds only the cross-cutting dependencies: the DB pool, the sqlc
  queries, and the token issuer the auth middleware is built from. `Register` builds the
  shared services, constructs every feature handler, and calls each feature's `register`
  in an order that keeps literal routes before param routes (e.g. `/jobs/search` before
  `/jobs/:slug`, `/threads/count` before `/threads/:id`, the static `/me/tracking/*`
  before `/me/tracking/:slug`).
- Each feature area owns a `<feature>Handlers` struct with its own dependencies, a
  `new<Feature>Handlers` constructor, and a `register(api fiber.Router, mw middleware)`
  method that mounts its routes — `grep 'Handlers struct' *.go` lists them all. Files are
  named after the routes they serve, so a feature spans several files (`matchHandlers` →
  `match_analysis.go`, `match_analysis_stream.go`, `job_match.go`, …). Handlers are thin;
  auth primitives, user job operations, API key management and error rendering live in
  their own files.
- Two couplings worth knowing: `cvHandlers` holds a `*matchHandlers` (it reuses the
  blocker/credits helpers for tailoring), and `jobs_moderation.go` carries the
  moderator-authored writes behind the `moderator` gate.
- The `middleware` bundle (`handler.go`) carries the shared auth gates features mount
  behind: `optional` (attach caller, never reject), `key` (cookie or API key), `cookie`
  (cookie-only), `moderator` (role gate, stacked after `key`/`cookie`).
- Services shared across features (résumé store, profile, credits, CV store/renderer,
  conversation store, match analyzer, contribution, moderation, the LLM spend resolver)
  are built once in `Register` and passed to each constructor; single-feature services are
  built inside the feature's constructor. **A constructor that builds a shared service is
  the bug, not the `Register` line that digs it back out** — `cvH.cvStore` and
  `assistantH.store` were both read out of a handler for a second consumer until the
  service moved up. When a second feature needs one, hoist it; do not reach into the
  first feature's struct.
- **No post-construction setter for a dependency that exists before construction.** Three
  of them (`withAssistantSessions`, `withFollowUps`, `withKeys`) existed only because of
  the ownership above, and each made a handler's field nil for part of its life. The two
  that survive — `withAccountDeletion`, `accounts.WithCodes`/`report.WithNotifier` — are
  conditioned on an SES client built inside an `if`, which is a different thing: the
  dependency genuinely does not exist yet. Two same-typed clients go in a named struct
  (`assistantModels`), not as adjacent parameters — a swap there compiles.
- Tests construct the feature struct directly with fakes/stubs (e.g.
  `&trackingHandlers{tracking: ...}`) and mount routes on a bare `fiber.App`.
- Central `handler.RenderError` (wired in `cmd/server` via `fiber.Config{ErrorHandler: handler.RenderError}`) renders JSON envelope: `*fiber.Error`→its code, `pgx.ErrNoRows`→404, FK-violation (SQLSTATE 23503)→404, everything else→500.
- Handlers signal failure by returning an error — `fiber.NewError(status, msg)` for specific codes, bare error (e.g. `pgx.ErrNoRows`) for common cases. Don't hand-roll per-handler error JSON; don't re-map `ErrNoRows` in read handlers (just `return err`).

## Auth Handlers (`auth.go`)

- `register`/`login` set JWT cookie + return `{"data": user}`. `logout` clears it. `me` is guarded by `RequireAuthOrKey` (cookie or API key).
- Rate-limited credential endpoints (10/min, keyed on client IP).

## Account Deletion (`me_delete.go`)

- `DELETE /api/v1/me` erases the caller's account permanently — no soft-delete, no grace period, no restore. Cookie-only (`RequireAuth`), never `keyAuth`: a leaked API key must not destroy the account that issued it.
- The body confirms the caller's **own** email (case-insensitive, matching the login lookup); a mismatch is 400 and erases nothing. Success is 204 plus an expired session cookie.
- The orchestration lives in `internal/accountdelete`, not here: object keys are collected, the Gmail grant revoked (best-effort, shared with `GmailDisconnect` via `revokeGmailGrant`), objects deleted, and only then the row — so a storage failure leaves the account whole. That case surfaces as **503**, meaning "nothing was deleted, retry", and is the one status worth knowing: `ErrStorageUnavailable` must not fall through to a 500.

## User Job Handlers (`user_jobs.go`)

- `view`/`apply`/`save`/`track` interaction endpoints. Addressed by job's public `:slug` (resolved to internal id before write). All writes are idempotent upserts behind `RequireAuthOrKey`.
- Return `{"data": interaction}` with `user_id` omitted; public job reads stay unauthenticated.
- **The `/me/tracking` silence fields are null together or set together.** `last_activity_at`, `days_silent` and `silence_state` describe an application awaiting a reply; a row that is merely viewed or saved, or one in a settled stage, carries all three as null. Null means "nothing is owed here", which the board must be able to tell apart from "owed and answered promptly" — so never substitute a zero-day `active`. The derivation lives on `jobtracking.TrackedJob.Silence`; the handler takes **one** `now()` for the whole page, so two rows cannot disagree about when the page was rendered.

## Mail Inbox + Harness Surface (`gmail.go`, `inbox.go`, `inbox_linking.go`, `inbox_harness.go`)

Pipeline and cross-package invariants live in [docs/agents/mail-stack.md](../../docs/agents/mail-stack.md); this section is the HTTP surface only. The use cases themselves live in `internal/inbox` — these handlers parse, call it and render, and the in-app assistant's mail tools call the same service without going through HTTP. A rule added to a handler here is a rule the assistant never meets.

It is the **harness** surface, not "the agent surface": there are two agents on this store now, and only this one speaks HTTP.

- Mounted on `mw.key`, so a user's own agent harness drives the inbox with the full-scope key it already uses for the tracker. **Exception: the Gmail OAuth connect/callback pair stays `mw.cookie`** — it redirects a browser to Google's consent screen and a keyed client cannot complete it.
- `TriageEmail` is `SetEmailClassification`'s sibling: status, link, provenance (`link_source = 'agent'`) and the classified stamp in **one** update, then `mailclassify.AdvanceStage`. Splitting it would manufacture states the worker never produces. An omitted slug means "not deciding the link", never "clear it". The stage advance is best-effort — the verdict is already durable.
- `IngestEmails` validates the whole batch before writing any of it and commits in one transaction, so a bad message at the end cannot leave earlier ones stored under a 400.
- `renderMutation` is the shared tail of every mutation that returns the message it changed (link, unlink, confirm, reject, triage, create-application), so those cannot drift from one another or from `GetEmail`. Unlike `GetEmail` it does not mark the message read: the caller is acting on the message, not opening it.
- **`?link=` partitions the mailbox** into `linked` / `suggested` / `unlinked` — `suggested` is the confirmation queue, `unlinked` the mail with nowhere to go yet. The predicate must go into `ListEmails` **and** `CountEmails` together or a filtered page reports an unfiltered total. A message that is both linked and carrying a stale suggestion reads as `linked`: the resolved answer wins over the proposal it superseded.
- `CreateApplicationFromEmail` records an application from mail and links it in one call. It **borrows `jobtracking`** rather than writing its own apply, so the `applied_count`-increments-once rule stays in one place; the application is dated by the email's `received_at`, never `now()`. Mail carrying a pending suggestion is a **409** — the matcher already proposed an answer, and overwriting it silently would make `link_source = 'manual'` a lie about a decision the caller never saw.
## Assistant (`assistant.go`, `assistant_*_tools.go`)

Routes (all on `mw.key` — the session cookie, the session JWT the extension's
connect flow minted, or a full-scope API key — and nothing else). Authentication
is the whole gate: every signed-in user reaches the assistant. The gate is `key`
rather than `cookie` because the extension's side panel holds conversations too
and cannot send an httpOnly cookie across origins. **Nothing meters a turn** —
the beta-tester gate that used to bound this spend is gone, and credit metering
has not replaced it yet:

| Route | Does |
|---|---|
| `POST /assistant/sessions` | start a conversation in the preset asked for — `chat` or `browse`; a tailoring one is minted by the CV bootstrap, which knows the CV and vacancy to bind |
| `GET /assistant/sessions` | the caller's conversations, most recently active first |
| `GET /assistant/sessions/:id` | one conversation with its stored transcript, for replay |
| `DELETE /assistant/sessions/:id` | remove a conversation and its transcript |
| `POST /assistant/sessions/:id/messages` | run one turn, streamed as named SSE events |
| `POST /assistant/sessions/:id/autopilot` | run the tailoring pass unattended — same stream, server-owned brief and ceiling (**cookie-only**) |

A session the caller does not own is a 404, never a 403, so ids stay unprobeable.
The autopilot route departs from the `key` gate above: it rewrites a CV without
being asked anything, and the browser is the one place the candidate can watch it
happen and undo it. It refuses anything but a tailoring session bound to a CV
(**409**), and snapshots that CV itself before the turn starts — a snapshot the
client could forget to ask for is a run nobody can take back. Undoing it is
`POST /me/cvs/:id/autopilot/undo` (also cookie-only), which restores the pre-run
document and clears the run's report with it.

Both SSE endpoints — the turn and the fit analysis — write through `sseStream`
(`match_analysis_stream.go`), the one owner of the stream protocol: it serializes the
heartbeat goroutine against the event callback and re-arms the write deadline before every
write. `event` reports whether the frame reached the client, and a marshal failure reports
**true** — an unencodable frame is our bug, not a dead reader. That is how a streamed turn
learns the client is gone: the failure cancels the loop's context, so it stops before
spending another model call. The fit analysis deliberately ignores the same signal — its run
was paid for with an AI credit, so it finishes into the cache rather than aborting.

`assistant_tools.go` / `assistant_tracking_tools.go` / `assistant_cv_tools.go` /
`assistant_profile_tool.go` / `assistant_present_tool.go` / `assistant_page_tools.go`
build the agent's tools from the same services these handlers use, and
`assistantRegistry` picks the set for a session's preset. The loop itself lives in
[internal/assistant](../assistant/AGENTS.md).

`read_current_page` (`assistant_page_tools.go`) is the only tool that leaves this
process for something we do not control: it drives the caller's browser through the
relay in [internal/browsertools](../browsertools/AGENTS.md). It is registered for the
`browse` preset alone, and it carries its own deadline, because every other tool is
bounded by a database or a search call and this one waits on a tab.

`present_jobs` is the odd one out: it is the only tool whose purpose is presentation
rather than retrieval or state change. It writes nothing and returns a receipt of
slugs, not vacancies — the client renders the deck and fetches each card's data by
slug, so a recommendation costs the model's context once, not twice.

`get_profile` is built on `profileHandlers` rather than the services under it, so the
tool and `GET /me/profile` share one assembly and cannot drift. It returns the CV as
`resumeextract.Professional` — **contacts omitted**, and not merely as good manners: a
tool result is persisted in the transcript and replayed into the model's context on
every later turn, so a name that lands there stays for the conversation's life. A
caller with no profile gets a result naming `/my/profile`, never an error and never an
empty profile the model would read as "no preferences".

## Application forms (`apply_form.go`)

- `GET /jobs/:slug/apply-form` serves the questions a posting's application will ask,
  projected for reading by `applyform.Display` — not the stored form, which keeps each
  platform's own field identifiers and option values so an autofiller can hand them back.
- **A 404 is the ordinary answer.** Forms can be read from three ATS platforms, roughly a
  sixth of technical postings, so most of the catalogue has none and never will. Do not
  treat it as an error worth logging or alerting on.
- Two queries, not one join — mirroring `JobCopies` on the same resource. An unknown slug
  fails at `GetJobIDBySlug`, a posting with no captured form at `GetApplyFormByJobID`.
  Both render 404, and keeping them distinguishable is the point: "this employer asks
  nothing" and "we cannot read this platform" are different statements.

## Error Convention

- Genuinely domain-specific status choices (e.g. `Me` returning 401 for a gone user token) stay in the handler.
- Recovered panic is **not** double-reported (recover middleware marks it via `handler.LocalPanicReported`).
- Sentry reports only fall-through unexpected 500s — routine errors never reported.
