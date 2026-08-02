## Context

The mail stack links a message to an application in one direction only. `cmd/classify-mail`
drains a queue; `internal/mailmatch` tries two deterministic signals (thread continuity, the
company name in the sender display name or subject) and `internal/mailclassify` runs an LLM
over the body. `internal/maillink` then decides: a deterministic hit at or above the
threshold links, anything else becomes a suggestion. Every user-facing surface starts from a
message, and the Emails tab of an application's drawer only renders what is already attached.

`internal/calmatch` trusts exactly one signal — a meeting carrying the `iCalUID` of an
invitation the mail matcher already linked. A first version also matched the employer's name
against an event title and was deleted: it compared against a hyphenated `company_slug`, was
an unanchored substring match for single-token employers, and nothing could confirm or
dismiss what it produced. `cmd/cal-sync` therefore discards, without a trace, any meeting
whose invitation never linked.

Three facts about the existing code shape this design:

- `emails.body_text` is **empty** for HTML-only mail, which is how Gem, Ashby and Greenhouse
  send. `ListEmails`'s `q` filter searches it, so a text search for the employer's name is
  blind exactly where the mail is. `maillink.ReadableBody` is what the worker reads instead.
- A suggestion lives in `emails.suggested_job_id`, and `POST /me/emails/:id/confirm|reject`
  already resolve one. `link_source` describes how `job_id` was set (`auto` / `manual` /
  `agent`) and says nothing about a suggestion.
- `inbox.ReconcileMailEvent` writes `employer_reply` into `application_events` on a **link**,
  not on a suggestion. That ledger is what a company's public response rate reads.

## Goals / Non-Goals

**Goals:**

- From an application, find the caller's mail that belongs to it, including mail neither
  deterministic tier could catch.
- Produce proposals only, resolved by the confirm/reject surfaces that already exist.
- Bounded, predictable cost: one model call per press.
- Make the calendar reachable again for applications whose invitation never linked, without
  reading a calendar.

**Non-Goals:**

- Linking, stage advancement, or any ledger write.
- Re-linking mail already attached to another application. The corrective case is real, but
  retract-and-re-record on a public response rate deserves its own change.
- A weaker calendar tier matching event titles. Its code was deleted for measured reasons;
  this change happens to build the confirmation surface it would need, which makes it a
  cheaper future proposal, not part of this one.
- Exposing the sweep as an assistant tool.
- A credit price for the action.

## Decisions

### A batched pipeline, not an assistant turn

The assistant already holds `my_jobs`, `inbox_search` and `inbox_link`, and
`POST /assistant/sessions/:id/opening` is a precedent for a server-authored brief, so an
agent turn was the obvious alternative. Rejected on three counts: a turn's latency and cost
are unbounded while the candidate waits at a button; a new preset is a CHECK-constraint
migration on `assistant_sessions.preset`; and an agent's body-bearing page is capped at 10
messages because a tool result replays into context on every later turn, against the 40 this
sweep wants to read once.

`internal/mailrecall` is therefore a service with no Fiber and no pgx, in the manner of
`internal/jobtracking`. Should the sweep later be worth an assistant tool, the tool calls
this service — the same shape as `internal/inbox` serving both the HTTP handlers and the
agent.

### The model's answer is written as a suggestion, not a link

The whole mail stack rests on one asymmetry: only a deterministic tier auto-links, because
the LLM reads attacker-controlled text. Pressing a button does not change what the model is
reading, so the same rule holds. Writing to `suggested_job_id` also means the confirmation
surface, the inbox queue and the resolve endpoints already exist, and the change adds no
second place where a link is decided.

It is also what keeps the action clear of the ledger. `employer_reply` is recorded on a link;
a proposal writes nothing there. The Workable incident — one catalogue company collecting 23
acknowledgements meant for 23 other employers, permanently unable to look silent — is the
cost of a wrong link, and this action cannot produce one.

*Alternative considered:* two thresholds, as `maillink` has, auto-linking the confident tail
on the grounds that pressing the button is consent. Rejected: consent to search is not
consent to a ledger write, and the failure is invisible until a company's public rate is
already wrong.

### The net is attachment state and time; the model does the narrowing

Selecting candidates by searching for the employer's name reproduces `mailmatch`'s known
blind spot — measured on a live mailbox, 16 of 99 correct links were to messages that never
name the employer — and, because `q` searches `body_text`, would additionally miss every
HTML-only sender. So the net filters on `application_id IS NULL` plus a time window and stays
deliberately wide, and the model reads `ReadableBody` to narrow it.

The window opens **seven days before `applied_at`**. `applied_at` is when the application was
recorded: for one recorded from mail it is that message's own `received_at`, and for one
recorded by hand it can be days late. A window starting exactly at it would exclude the
acknowledgement that proves the application. A tracking row with `applied_at IS NULL` is not
an application and answers 404, and that check is in the service rather than the handler —
a rule enforced in a Fiber handler is one the in-process caller never meets.

**It closes ninety days after, and the closing edge is one decision with the cap and the
sort, not three.** Left open, newest-first, the forty candidates go to a busy mailbox's most
recent mail: a three-month-old application never shows the model the acknowledgement, and
the button answers "nothing found" on exactly the applications people press it for. The net
is therefore ordered **oldest first** inside a closed window, so the cap trims the far tail
rather than the head. Ninety days comfortably covers a funnel whose silence ladder
(`internal/userjob`) tops out at 21 days for `applied`. The cost is real and stated: an
application still moving after three months will not find its recent mail this way. That is
the trade a bounded run buys, and it is measurable later from confirm rates by application
age.

### The question put to the model changes shape, and that is the point

The worker asks "which of these N applications does this message belong to?" — a pick where a
wrong answer transplants one employer's history onto another, and where a guard requiring the
employer to be named in the body was measured and rejected for dropping 16 correct links out
of 99.

This action asks "does this message belong to the application the candidate just named?" —
independently per message, with the application supplied by a human. That is why a batched
call is enough and why no disambiguation tier is needed.

### The guard against a wrong write lives in SQL

`SuggestJobForEmail` carries `WHERE job_id IS NULL AND application_id IS NULL` in the
statement. A linked message cannot be modified by this path even if the net, the model and
the service layer all went wrong at once. `ListEmailsForRecall` is a query of its own
rather than new parameters on `ListEmails`, which serves the web inbox and seven assistant
tools; extending a shared query for one reader is how the two drift.

**It takes both columns to say "unattached", and one of them was nearly enough.** A
message can hold `job_id` with `application_id` still NULL: `ListUserApplicationsForMatch`
offers the matcher saved-only jobs, `SetEmailClassification` derives an application row
that does not exist yet and leaves it NULL, and `MarkJobApplied` never returns to repair
the mail — `cmd/backfill-applications` does, and it is a one-shot, not a cron. Testing
`application_id` alone would have admitted exactly that message to the net and ended in a
confirm that RE-LINKS it, which this change lists as a non-goal. The name is
`SuggestJobForEmail` and not `...Application...` for the same reason `LinkEmailToJob` is:
the column names a job, and the application is what `ConfirmEmailLink` derives.

### Bounds are the injection defence and the cost ceiling at once

At most 40 candidates per run, 800 runes of body each — against `mailclassify`'s 4000, which
reads one message where this reads forty. Structured output, and any id absent from the batch
is discarded, so a body that talks the model into naming a message outside the net achieves
nothing. The reachable damage from a successful injection is one spurious proposal, removed
by Reject.

### No calendar code

`cmd/cal-sync` re-reads its ±90-day window on every run and re-matches it against the
caller's applications as they then stand, so an invitation linked today yields its meeting on
the next run with nothing added. The response counts proposed messages carrying an `ical_uid`
so the card can say so; a synchronous Google call in a user request would add a failure mode
and a missing-grant path for a result that arrives on its own.

### Spend is attributed, not metered

The call goes out on the caller's own gateway credential (`internal/llmkey`), tagged
`feature:mail-recall`: searching a candidate's mailbox is work that belongs to them, not to
the service credential that pays for enrichment. Attribution fails open, per that package's
rule. No credit debit — `internal/credits` knows `match` and `tailor`, both expensive
one-shot actions, and this is one cheap call; a price set before the spend distribution is
known is a guess, the same reasoning that leaves `LLM_USER_MAX_BUDGET` unset. The store stays
the seam.

### A failed model call is loud

Unlike the assistant's follow-up strip, which answers an empty list on every failure path
because it is decoration, this is what the person pressed. An empty success is
indistinguishable from a mailbox with nothing in it, so the model failing is a 502.

## Risks / Trade-offs

- **A wide net proposes noise, and a noisy button teaches people to ignore it** → the model
  is the narrowing step, and only its confident answers are written; the cap on candidates
  bounds how much noise one press can produce. Recall and precision here are measurable after
  the fact from confirm-vs-reject rates on proposals.
- **Overwriting another application's unconfirmed suggestion loses a proposal nobody saw** →
  accepted deliberately. `suggested_job_id` holds one value, a suggestion is not a decision,
  and the caller asked about this application explicitly.
- **Prompt injection in a body** → structured output validated against the batch, no link, no
  stage move, no ledger write, no outbound channel. Worst case is one proposal to reject.
- **Cost per press scales with mailbox size** → it does not: the cap is on candidates, not on
  the mailbox, and an empty net makes no call at all.
- **Someone later adds an auto-link tier here** → `TestMailRecallCannotLink` fails, in the
  manner of `calmatch.Tier.Links()`: the rule is written so the next reader must answer it
  rather than trip over it.

## Migration Plan

No migration and no backfill: the change writes to columns that already exist. Deploy is
ordinary — `make sqlc` regenerates `internal/db`, the endpoint and the button ship together.
Rollback is removing the route; nothing it wrote needs undoing, since a stale suggestion is
resolvable by the surfaces that predate this change.

## Open Questions

None blocking. Two to revisit with data:

- Whether the proposal's confirm rate justifies a credit price, or a higher candidate cap.
- Whether a confirmation surface now existing makes the deleted calendar title tier worth
  re-proposing.
