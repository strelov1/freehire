# internal/api/handler — HTTP Handlers

Fiber HTTP handlers: feature handler structs, route registration, auth surface, user job endpoints, error rendering.

## Architecture

- `API` (`handler.go`) holds only the cross-cutting dependencies: the DB pool, the sqlc
  queries, the token issuer the auth middleware is built from, and the browser-tool hub
  (`browserTools`) that relays tool frames between a user's harness and their extension.
  `Register` builds the
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
- **Surfaces are moving into subpackages (#4586)**, one per route group (`jobs`, `cv`,
  `account`, `tracking`, `engage`, `admin`). A surface package imports nothing of `handler`
  (that would be a cycle): its handlers struct and constructor are exported
  (`jobs.NewGeoHandlers`), it takes the auth gates it mounts behind as plain `fiber.Handler`
  arguments to an exported `Register`, and `contractHandlers` embeds it under a name unique
  across surfaces (`*jobs.GeoHandlers`). Route tests that go through `newContract` or
  `Register` stay in `handler`; the surface keeps its own unit tests. Moved so far:
  `jobs/geo.go` (city autocomplete). `web/static/openapi.yaml` does not change on a move.
- Two couplings worth knowing: `cvHandlers` holds a `*matchHandlers` (it reuses the
  blocker/plan helpers for tailoring), and `jobs_moderation.go` carries the
  moderator-authored writes behind the `moderator` gate.
- The `middleware` bundle (`handler.go`) carries the shared auth gates features mount
  behind: `optional` (attach caller, never reject), `key` (cookie or full-scope API key),
  `cvKey` (`key` widened to admit the narrow `cv` key — the CV surface and `/me` only),
  `cookie` (cookie-only), `optionalCookie` (attach a cookie session, never reject — for
  provider callbacks, which are browser navigations), `moderator` (role gate, stacked
  after `key`/`cookie`), and `paidAI(feature)` — the paid-entitlement gate every route that
  makes a model call for its caller mounts AFTER its authentication gate (`paid_ai.go`: 402
  for an unpaid caller in the ordinary plan-refusal body, 503 when the plan cannot be read,
  otherwise it marks `c.UserContext()`; a 402 also records a paywall hit — user, feature, job — through a detached, panic-recovered goroutine that never delays or alters the refusal, see internal/ai/plan/AGENTS.md; the auto-apply orchestrator's tailor refusal calls `authorizePaidAIUnrecorded` and records nothing). The binding (`user_llm.go`) refuses a model client
  for an unmarked context, so handlers pass `c.UserContext()` (or a context detached from
  it, as the assistant turn and the fit stream do) to `bind`/`caller`. Tests that are not
  about the gate mount `markPaidAI` in its place..
- **A guest model call spends the service client directly, never through the binding.**
  `llmBinding.bind`/`caller` with user id 0 would ask `llmkey.Resolver.For` about account 0,
  which reads and then MINTS a gateway credential for it. The guest interpretation takes
  `h.llm.client.As("", nil, llm.Feature(tagSearchIntent))` — still tagged, so its spend is
  visible under the feature — and a test asserts the resolver is never read.
- **A guest path refuses `Content-Encoding` before reading the body** (`refuseEncodedBody`).
  Fiber decompresses an encoded body with no ceiling; behind an account that is tolerable,
  without one a single request is enough. Signed-in callers of the same route are not
  refused, so a client that gzips today keeps working.
- Services shared across features (résumé store, profile, plan, CV store/renderer,
  conversation store, match analyzer, contribution, moderation, the LLM spend resolver)
  are built once in `Register` and passed to each constructor; single-feature services are
  built inside the feature's constructor. **A constructor that builds a shared service is
  the bug, not the `Register` line that digs it back out** — `cvH.cvStore` and
  `assistantH.store` were both read out of a handler for a second consumer until the
  service moved up. When a second feature needs one, hoist it; do not reach into the
  first feature's struct.
- **No post-construction setter for a dependency that exists before construction.** Three
  of them (`withAssistantSessions`, `withFollowUps`, `withKeys`) existed only because of
  the ownership above, and each made a handler's field nil for part of its life. What
  survives is the other kind — the dependency genuinely does not exist when the handler is
  constructed: `accounts.WithCodes`/`report.WithNotifier` attach to an SES client built
  inside an `if`, `inboxH.withRecall` waits on `cfg.LLM`, and `withAccountDeletion`,
  `experienceH.withRequireContext` and `accountDeletion.WithGatewayKeys` wire services
  `Register` finishes building further down. A few plain field assignments do the same
  (`authH.*` config fields, `assistantH.realtime`, the `resumeH.llm`/`matchH.llm`
  bindings). Two same-typed clients go in a named struct (`assistantModels`), not as
  adjacent parameters — a swap there compiles.
- **Catalogue scale is read, never counted** (`stats.go` `CatalogScale`, `jobs.go`
  `openJobTotal`). Every figure describing how big the catalogue is —
  `GET /stats/catalog` and the jobs list's `meta.total` — comes from the snapshot
  `cmd/rollup-stats` publishes (`internal/ingest/catalogstats`). Both call
  `catalogstats.Load`, which takes no exact counter, so no request path can reach a
  catalogue-wide scan even by mistake. Neither read can fail: a cold cache, an
  unreachable Redis, a payload from an older build and no `cfg.Cache` at all all
  degrade to the approximate estimate with `exact: false`. Add a new consumer by
  calling `Load`, not by counting — and pass `exact` through to whatever renders it,
  because a degraded snapshot zeroes the figures that exist only in the database and a
  zero must not reach a page as if it were a measurement.
- Tests construct the feature struct directly with fakes/stubs (e.g.
  `&trackingHandlers{tracking: ...}`) and mount routes on a bare `fiber.App`.
- Central `handler.RenderError` (wired in `cmd/server` via `fiber.Config{ErrorHandler: handler.RenderError}`) renders JSON envelope: a `codedError`→its status with a `code` field, `*fiber.Error`→its code, `pgx.ErrNoRows`→404, FK-violation (SQLSTATE 23503)→404, `inbox.ErrNotFound`→404, `inbox.InvalidError`/`inbox.ErrSlugRequired`/`search.ErrBadQuery`→400, `inbox.ErrPendingSuggestion`→409, `context.Canceled`→499 (client closed request), everything else→500.
- Handlers signal failure by returning an error — `fiber.NewError(status, msg)` for specific codes, bare error (e.g. `pgx.ErrNoRows`) for common cases. Don't hand-roll per-handler error JSON; don't re-map `ErrNoRows` in read handlers (just `return err`).

## Auth Handlers (`auth.go`)

Trusted infrastructure can resolve credentials through the optional internal
[authentication endpoint](../../../docs/agents/auth-infrastructure.md). Its caller
metadata is attribution only; it never grants application permissions.

- `register`/`login` set JWT cookie + return `{"data": user}`. `logout` clears it. `me` is guarded by `RequireAuthOrScopedKey` (cookie or API key — the narrow `cv` key included, so an agent can resolve which account its key belongs to).

## Every `/api/v1/me/*` route refuses an anonymous caller (`signed_in_door.go`)

- **This is load-bearing outside the repo.** Cloudflare challenges the hosting/VPN networks the enumeration fleet rents, and a CLI cannot solve a challenge; the edge rule exempts `/api/v1/me/*` precisely because nothing there answers without a credential, so the exemption hands the fleet nothing. A public read added under `/me/` would quietly reopen it to the fleet.
- The same edge rule also exempts paths starting `/api/v1/mcp` (that is `/api/v1/mcp`; the OJCP server at `/api/v1/ojcp/mcp` is still challenged) and `/.well-known/oauth-*`, since 2026-11-09: an MCP client cannot solve a challenge either, and a VPN user's connector failed with "Couldn't reach". Since 2026-11-13 `/api/v1/mcp` answers an anonymous caller 401, so the exemption no longer opens any catalogue read to the fleet.
- `/api/v1/me/api/<rest>` is the signed-in door: it checks `RequireAuthOrKey` and serves the request as `/api/v1/<rest>`, which is how a signed-in CLI on a VPN reaches the public reads. The CLI uses it only after the edge has answered a challenge (`cf-mitigated: challenge`). The target route keeps every gate of its own. It rewrites the path and lets routing continue rather than calling `RestartRouting`, which would replay the app-wide middleware and count and log every request twice; so it must stay mounted before every `/api/v1` route.

## Account Deletion (`me_delete.go`)

- `DELETE /api/v1/me` erases the caller's account permanently — no soft-delete, no grace period, no restore. Cookie-only (`RequireAuth`), never `keyAuth`: a leaked API key must not destroy the account that issued it.
- The body confirms the caller's **own** email (case-insensitive, matching the login lookup); a mismatch is 400 and erases nothing. Success is 204 plus an expired session cookie.
- The orchestration lives in `internal/identity/accountdelete`, not here: object keys are collected, the Gmail grant revoked (best-effort, shared with `GmailDisconnect` via `revokeGmailGrant`), objects deleted, and only then the row — so a storage failure leaves the account whole. That case surfaces as **503**, meaning "nothing was deleted, retry", and is the one status worth knowing: `ErrStorageUnavailable` must not fall through to a 500.

## User Job Handlers (`user_jobs.go`)

- `view`/`apply`/`save`/`track` interaction endpoints. Addressed by job's public `:slug` (resolved to internal id before write). All writes are idempotent upserts behind `RequireAuthOrKey`.
- Return `{"data": interaction}` with `user_id` omitted; public job reads stay unauthenticated.
- **The `/me/tracking` silence fields are null together or set together.** `last_activity_at`, `days_silent` and `silence_state` describe an application awaiting a reply; a row that is merely viewed or saved, or one in a settled stage, carries all three as null. Null means "nothing is owed here", which the board must be able to tell apart from "owed and answered promptly" — so never substitute a zero-day `active`. The derivation lives on `jobtracking.TrackedJob.Silence`; the handler takes **one** `now()` for the whole page, so two rows cannot disagree about when the page was rendered.

## Mail Inbox + Harness Surface (`gmail.go`, `inbox.go`, `inbox_linking.go`, `inbox_harness.go`)

Pipeline and cross-package invariants live in [docs/agents/mail-stack.md](../../../docs/agents/mail-stack.md); this section is the HTTP surface only. The use cases themselves live in `internal/application/inbox` — these handlers parse, call it and render, and the in-app assistant's mail tools call the same service without going through HTTP. A rule added to a handler here is a rule the assistant never meets.

It is the **harness** surface, not "the agent surface": there are two agents on this store now, and only this one speaks HTTP.

- Mounted on `mw.key`, so a user's own agent harness drives the inbox with the full-scope key it already uses for the tracker. **Exception: the Google OAuth pairs — `/me/gmail/connect`+`/me/gmail/callback`, and the calendar's own connect/callback beside them.** Each connect stays `mw.cookie` — it redirects a browser to Google's consent screen and a keyed client cannot complete it. Each callback mounts `mw.optionalCookie` instead: it is the browser returning from Google, and under `RequireAuth` a session that did not survive the round-trip would render a JSON 401 into the address bar, so the callback answers that case itself with a redirect.
- `TriageEmail` is `SetEmailClassification`'s sibling: status, link, provenance (`link_source = 'agent'`) and the classified stamp in **one** update, then `mailclassify.AdvanceStage`. Splitting it would manufacture states the worker never produces. An omitted slug means "not deciding the link", never "clear it". The stage advance is best-effort — the verdict is already durable.
- `IngestEmails` validates the whole batch before writing any of it and commits in one transaction, so a bad message at the end cannot leave earlier ones stored under a 400.
- `renderMutation` is the shared tail of every mutation that returns the message it changed (link, unlink, confirm, reject, triage, create-application), so those cannot drift from one another or from `GetEmail`. Unlike `GetEmail` it does not mark the message read: the caller is acting on the message, not opening it.
- **`?link=` partitions the mailbox** into `linked` / `suggested` / `unlinked` — `suggested` is the confirmation queue, `unlinked` the mail with nowhere to go yet. The predicate must go into `ListEmails` **and** `CountEmails` together or a filtered page reports an unfiltered total. A message that is both linked and carrying a stale suggestion reads as `linked`: the resolved answer wins over the proposal it superseded.
- `CreateApplicationFromEmail` records an application from mail and links it in one call. It **borrows `jobtracking`** rather than writing its own apply, so the `applied_count`-increments-once rule stays in one place; the application is dated by the email's `received_at`, never `now()`. Mail carrying a pending suggestion is a **409** — the matcher already proposed an answer, and overwriting it silently would make `link_source = 'manual'` a lie about a decision the caller never saw.
## Assistant (`assistant.go`, `assistant_*_tools.go`)

Moved to [AGENT-REFERENCE-ASSISTANT.md](AGENT-REFERENCE-ASSISTANT.md#assistant-assistantgo-assistant__toolsgo).

## Talent Network catalogue (`talent_catalog.go`)

- `GET /talent` and `GET /talent/:handle` are the **only** unauthenticated routes that
  serve candidate CV content — they take no `mw` auth middleware at all. They replaced
  `GET /talent-network/:publicID`, which served one candidate by an opaque UUID; that
  route, its handler and its column are gone (migration `0149`), because two public
  identifiers for one page is a drift waiting to happen.
- **The projection is not this package's to make.** The body is
  `talentnetwork.CatalogueMember`, built by `talentnetwork.ProjectCard` — a whitelist that
  emits only dictionary-resolved terms, numbers and dates, never text a candidate typed.
  Do not reach into `row.ResumeStructured` here for anything, and do not add a field to
  the response that did not come through that projection. The reasoning, and the test that
  enforces it, are in [internal/candidate/talentnetwork/AGENTS.md](../../candidate/talentnetwork/AGENTS.md).
- **The 404-identity invariant:** a member who left, a member whose CV extract has gone
  stale, a handle nobody holds, and a string that could not be a handle all render the
  byte-identical `{"error":"not found"}`. A malformed handle is refused by
  `talentnetwork.ValidHandle` before the query, so it 404s rather than 400s. Do not add a
  distinct status or message for any of the four; that is the leak this route is built to
  not have, and a test asserts the bodies match.
- `:handle` is the minted catalogue handle, never `users.id` and never `users.username` —
  the username is derived from the email's local part, so for most accounts it is the
  person's own name.
- Membership is re-read from the database on the single-card route rather than from the
  list's snapshot, so leaving takes effect on the next request.
- The card response carries a short `Cache-Control` and `X-Robots-Tag: noindex`: a card is
  a person, and a cache of one outlives their decision to leave.
- Unread query params go in `meta.ignored_params` through `ignoredTalentParams`, which
  measures against `talentnetwork.KnownParams()` — **not** `search.UnknownParams`, whose
  vocabulary is the job filter's. Passing these facets to that function as `alsoKnown`
  would additionally accept every job-search facet on an endpoint that reads none.

## The caller's own membership (`me_talent_network.go`)

- `GET /me/talent-network` takes a key; the `PUT` is **cookie-only**, so a leaked API key
  cannot put somebody in a public catalogue.
- Two values, `off` and `anonymous`. `public` was retired with the mode picker (migration
  `0146`) and a request asking for it is a 400 from the handler rather than a 500 from the
  CHECK constraint.
- Joining calls `talentnetwork.Join`, which mints the handle. The handler does not hold
  the mint loop: what to name somebody and how to resolve a collision is domain logic, and
  it is unit-tested there without a Fiber context.
- The mint runs **before** the visibility write. A member without a handle is a member the
  catalogue cannot link to, so a failure must leave the candidate outside the network
  rather than inside it with no address.

## Résumé / CV (`resume.go`, `cv_roast.go`)

- `POST /cv/roast` is public — no cookie, no key. It stores nothing (unlike
  `/me/resume/extract`, which stores for a signed-in caller because their later steps
  need the file) and never calls the LLM analyzer. This endpoint forks `pdftotext`.
- **`RoastCV` refuses any `Content-Encoding` header outright, before `readResumeUpload`
  runs.** Fiber's `Ctx.Body()` — which `BodyParser` and `c.FormFile` both read through —
  decompresses `Content-Encoding` transparently, chaining up to three layers, with no
  ceiling on the decompressed size (see `billing.go`'s `webhookFor` for the same trap on
  the app's other unauthenticated POST). The server's 8MB `BodyLimit` bounds only the wire
  body; a few compressed megabytes can expand to gigabytes in memory. That was tolerable
  while this reader sat behind `mw.cookie` — an attacker needed an account — but this
  route has none, and one request is enough. The check is a bare "is this header non-empty", not an allow-list or a parse of
  its value: this route has exactly one job and nobody legitimately gzips a CV upload, so
  a second place to get that logic wrong is not worth having. `readResumeUpload` itself is
  deliberately untouched — `/me/resume/extract` applies the same refusal to its guests
  only, and widening the shared reader would silently break any signed-in client that
  gzips today.
- `POST /me/resume/extract` takes an optional session: the AI search lets a guest attach a
  CV. For a guest it stores nothing and derives nothing (no structured extraction, so no
  model call); for a signed-in caller it is unchanged, storing the file when S3 is
  configured.

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

## Pagination (`handler.go`)

- **`offset` is read in exactly one file.** `pageParams` / `pageParamsBounded` /
  `pageParamsWindowed` / `pageParamsWithin` live in `handler.go`, and `pagination_rule_test.go` fails the build if
  any other non-test file in the package reads the raw param. The clamp into int32 range is
  why: without it `?offset=3000000000` binds negative and Postgres answers 500.
- **Every PUBLIC list calls `pageParams`/`pageParamsWindowed`**, which refuse `offset+limit >
  maxPageWindow` with 400. One constant, one helper, both stores — the Meili-backed search and
  the Postgres-backed lists share it rather than each carrying a number that can drift apart.
  The one exception is NARROWER, never wider: `CompanyClosedJobs` goes through
  `pageParamsWithin` with `closedJobsWindow` (2000, the web's `FEED_WINDOW`), because each
  skipped offset there is a heap fetch of a closed row — a ~10k offset measured 6.4s cold.
- **Check the window before any query, including a slug lookup.** `GetCompany` and `JobCopies`
  both read the page params before their first read, so a refused request costs no database
  work at all. That ordering is asserted by `pagination_window_routes_test.go`, which drives
  each route with zero-valued handlers: a 400 proves nothing was queried, and its sibling
  control proves a shallow request really does reach the handler.

**A caller-controlled offset is a caller-controlled cost.** Keep query bounds
at the API boundary: walking deep offsets can occupy the database pool even when
individual requests are valid.

## Error Convention

- Genuinely domain-specific status choices (e.g. `Me` returning 401 for a gone user token) stay in the handler.
- Recovered panic is **not** double-reported (recover middleware marks it via `handler.LocalPanicReported`).
- The exception log gets **every 500**, including one a handler declared with `fiber.NewError(500, …)` to phrase it for a human. `classify` decides on the STATUS, not the error type: a declared 500 used to report nothing, which is how thirteen call sites quietly left the error inbox. Routine 4xx and mapped 404s are never reported, and the test is an equality — the ~42 sites answering 503 mean "this deployment has no Meilisearch/LLM", which is not a fault.
- **`fiber.NewError` cannot wrap**, so a 500 declared that way is reported as a message with no cause. When there is a cause worth keeping, return `fmt.Errorf("…: %w", err)` and let the fall-through render the generic body instead of phrasing the message and dropping the error.
