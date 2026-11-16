# Mail stack: the pull direction and the syncs

Part of the [mail stack](../mail-stack.md): searching the mailbox on demand, the calendar sync, what may cost a candidate their Gmail connection, and the known gaps in the push path.

- **There is a PULL direction, and it SEARCHES the mailbox rather than reading our copy.**
  Everything above starts from a message and asks which application it belongs to.
  `POST /me/tracking/:slug/mail-recall` (`internal/application/mailrecall`) asks the opposite. It issues
  ONE Gmail search scoped to the employer inside a window around `applied_at`, adjudicates
  what comes back in one model call, and shows the confident answers.
  The first version read the stored table instead, and production retired it: candidates in
  that window ran 96 at minimum and 158 at the median, **263 of 263 applications exceeded
  the cap of 40**, and the batch was chosen oldest-first — which says nothing about
  relevance. The store simply cannot answer "about this employer": matched against sender
  name and subject the employer's name is absent from the median application's mail
  entirely. A mailbox search reads BODIES, and found mail for **14 applications in 15**
  where the store found none. A caller with no searchable mailbox still gets the stored
  path, which is why both still exist.
  Four rules carry it:
  *The search is gated* — employer AND (hiring vocabulary OR `filename:ics` OR the role
  title) — because scoping by a company name would otherwise reach personal mail. All three
  gate members were measured: hiring words alone cut 53 candidates to 41 and dropped both
  calendar invitations for one interview plus a live thread whose only subject was the role.
  `filename:ics` is the exact member for invitations, whatever language the subject uses.
  *A proposal is not stored.* The sweep keeps nothing; pressing Link imports the message
  (idempotent on `(source, external_id)`) and then links it. What nobody confirmed is not
  kept.
  *The model is addressed by POSITION in the batch, never by an id* — a searched message
  has none of ours, and an out-of-range position resolves to nothing by construction.
  *It never links, never advances a stage, never writes the ledger* — held by
  `TestMailRecallCannotLink`, which now walks both `Store` and `Mailbox`, and by a source
  scan of the package for `LinkEmailToJob` / `application_events` / `calsync`.
  **The calendar still needs no code**: `cmd/cal-sync` re-reads its whole ±90-day window
  every run, so an invitation linked here yields its meeting on the next one.

- **`internal/application/calsync` stores only meetings an application earned.** `cmd/cal-sync` reads each
  connected candidate's calendar ±90 days around now and writes ONLY a meeting `calmatch` can
  attach to one of the candidate's own applications — the schema backstops the rule
  (`application_interviews.application_id` is NOT NULL), so a mistake cannot become a stored
  dentist appointment. The whole window is re-read on every run; there is no incremental
  state to drift. The only status ever written is `confirmed`. Connections are filtered by
  the scopes recorded at consent, so a Google grant that predates the calendar consent never
  reaches the worker and never costs an API call to discover. The worker shares the Gmail
  Google grant and `gmailsync.CalendarScope`, and nothing else.

- **Only `gmailsync.RevokedGrant` may cost a candidate their connection**, and both syncs ask
  it rather than deciding for themselves. `gmail_connections.status` is SHARED between mail
  and calendar (one grant covers both scopes), nothing but a browser consent clears it, and
  the mail half is a restricted scope Google makes them re-approve — so a rule that read any
  failure as a revocation would disconnect every mailbox we hold during one Google incident,
  and the inbox's own Sync button would do it to one candidate on a bad afternoon. The rule
  reads TWO carriers, because a revocation arrives on either leg: **401/403 from the API**
  (except a 403 whose Google reason is a rate limit or quota — `gmailsync.LimitHit`, which only
  stops the run)
  (`gmailsync.APIError`, which both readers attach to a non-200 so the status survives the
  return), and **`invalid_grant` from the TOKEN endpoint**, which is where a real revocation
  actually lands — the readers' client is built from a stored refresh token, so oauth2
  exchanges it before any API call and the API is never reached. That one arrives as a
  `*url.Error` wrapping `*oauth2.RetrieveError`. `invalid_client` is deliberately NOT a
  revocation: that is our own credential being wrong.

- **Two known gaps in the PUSH path, measured and not yet fixed.** They are why the pull
  direction has so much to find. `gmailsync.BuildQuery` fetched **431** messages over 120
  days from a mailbox holding **3297**; **739** hiring-shaped messages were never fetched,
  including an acknowledgement, three interview invitations and four live recruiter threads
  from personal and corporate domains. The misses are near misses on wording — the phrase
  list knows `invite you to interview` but not `interview invite`. Separately,
  `mailmatch.ExtractCompany`'s five subject templates all use `to`, while the mailbox shows
  `at`, `with`, `as … at`, a company before a dash, a company after a pipe, and Portuguese;
  33 of 93 messages with an empty sender name carry a subject it cannot parse.
