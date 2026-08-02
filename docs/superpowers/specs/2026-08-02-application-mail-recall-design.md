# Mail recall from an application

A button in an application's card that sweeps the caller's mailbox for messages
belonging to *that* application and offers them as suggestions.

## The gap

Linking mail to an application is a **push**: a message arrives, `internal/maillink`
asks which application it belongs to, and either links it (deterministic tier) or
files a suggestion (LLM tier). There is no pull. Every existing surface starts from
a message — `?link=suggested`, `?link=unlinked`, `POST /me/emails/:id/link` — so a
candidate looking at an application that ought to have mail and does not has nowhere
to ask. The Emails tab in `JobDrawer.svelte` renders what is already linked and
cannot re-ask.

The calendar inherits the same gap and makes it worse. `calmatch` trusts exactly one
signal: the meeting carries the `iCalUID` of an invitation the mail matcher already
tied to an application. When that link is missing, `cmd/cal-sync` reads the meeting
into memory, matches nothing, and discards it — the interview is invisible and no
row records that it was ever seen.

## What it is

One button, in the Emails tab of an application's drawer. It runs a three-step pass:

```
[Find this application's mail]
   │
   ▼  POST /me/tracking/:slug/mail-recall
internal/mailrecall
   ├─ 1. Net (deterministic, no model)
   │     ListEmailsForRecall: application_id IS NULL,
   │     received_at >= applied_at - 7d, capped at 40
   │
   ├─ 2. One model call (batched, structured output)
   │     in:  the application (company, role, date) + the candidates
   │          (from_name, from_addr, subject, ReadableBody, truncated)
   │     out: per candidate — belongs/does not, with a confidence
   │
   └─ 3. Confident ones are written as this application's suggested_job_id
```

The calendar needs **no code**. `cal-sync` re-reads its ±90-day window on every run
and re-matches it against the candidate's current applications, so an invitation
linked today produces its meeting on the next run. The response reports how many of
the suggestions carry an `ical_uid` so the card can say so.

## Decisions

### Everything goes to confirmation; nothing is linked

The button writes into `suggested_job_id` — the same field `maillink` writes an LLM
pick into — and confirmation runs through the existing `POST /me/emails/:id/confirm`
and `/reject`. No new confirmation surface, and the asymmetry that holds the mail
stack up is untouched: **only a deterministic tier may link, a model's pick is a
suggestion**. Email bodies are attacker-controlled text; that rule is why.

This also keeps the button clear of `application_events`. An `employer_reply` is
recorded when a message is **linked**, not when it is proposed, and that ledger is
what a company's public response rate reads. The Workable incident — one catalog
company collecting 23 acknowledgements meant for 23 other employers — is what a
wrong link costs. The button cannot produce one.

### The net is metadata and time, not words

`ListEmails`'s `q` matches `subject`, `from_name`, `from_addr` and `body_text`. The
last is **empty** for HTML-only mail, which is most of the ATS senders that matter
(Gem, Ashby, Greenhouse) — a company-name search over bodies is blind exactly where
the mail is. `q` is also a single term, so a five-word net would be five queries.

So the net filters on link state and a time window and stays deliberately wide; the
model does the narrowing, reading bodies through `maillink.ReadableBody` the same way
the classification worker does.

The window opens **seven days before `applied_at`**, not at it: `applied_at` is when
the application was *recorded*, which for one recorded from mail is the message's own
`received_at` and for one recorded by hand can be days late. A window starting exactly
at it would exclude the acknowledgement that proves the application.

A tracking row with `applied_at IS NULL` is not an application and has no mail to
find; the endpoint answers 404 for it, the same way it answers for a row that is not
the caller's.

### The net takes unlinked and suggested mail, never linked

An unconfirmed suggestion is often the very thing the button exists to fix, so it is
in scope. A linked message is not: re-linking retracts and re-records in the ledger,
and the button must not be able to reach that. The guard is the `WHERE
application_id IS NULL` predicate in `SuggestApplicationForEmail`, in SQL rather
than in Go.

A message already suggested to a *different* application is **overwritten**. The
candidate asked about this application explicitly, and a suggestion is a proposal
that costs nothing to lose. `suggested_job_id` holds one value; there is no
provenance column for suggestions and none is added — `link_source` describes how
`job_id` was set (`auto` / `manual` / `agent`), not who proposed what.

### Why a pipeline and not an assistant turn

The assistant already has the tools for this — `my_jobs`, `inbox_search`,
`inbox_link` — and `POST /assistant/sessions/:id/opening` is a precedent for a
server-authored brief. It was rejected for three reasons: a turn's latency and cost
are unbounded while the candidate waits; a new preset is a CHECK-constraint
migration; and an agent's body-bearing page is capped at 10 messages because a tool
result replays into context on every later turn. A single batched call has none of
these properties.

`internal/mailrecall` is a service with no Fiber and no pgx, like `internal/jobtracking`.
Exposing it as an assistant tool later is additive and is not part of this change.

## Wire

`POST /api/v1/me/tracking/:slug/mail-recall`, under `mw.key` — a session cookie or a
full-scope key, matching its neighbours on the same prefix. POST because it spends
money and writes suggestions.

```json
{"data": {"scanned": 34, "suggested": [ /* inbox.Message */ ], "invitations": 2}}
```

`scanned` is how many messages the net caught, `suggested` those the model kept —
using the listing's own projection so the Emails tab renders them with the row it
already draws. `invitations` counts the suggested ones carrying an `ical_uid`.

| Case | Response |
|---|---|
| No mailbox connected | 200, `scanned: 0` — the UI invites connecting one |
| Net empty | 200, `scanned: 0`, `suggested: []` |
| Tracked but never applied | 404 |
| Model unreachable | 502 `{"error": ...}` — **loudly**. Unlike the follow-up strip, this is what the person pressed; a silent empty result is indistinguishable from "nothing found" |
| Application missing or not theirs | 404 |

## Spend

The call goes out on the **caller's own gateway credential** (`internal/llmkey`),
tagged `feature:mail-recall`. Searching a candidate's mailbox is work that belongs to
them, not to the service credential that pays for enrichment. Attribution fails open,
per the package's rule: an unmintable credential falls back to the service one and the
call completes.

No credit debit. `internal/credits` knows `match` and `tailor`, both expensive
one-shot actions; this is one call to a cheap model over metadata and truncated
bodies. The store stays the seam — a price set before the spend distribution is known
is a guess, the same reasoning that leaves `LLM_USER_MAX_BUDGET` unset.

## Bounds

These are the injection defence and the cost ceiling at once:

- At most 40 candidates per run; 800 runes of body each (against `mailclassify`'s
  4000 — that reads one message, this reads forty).
- Bodies via `maillink.ReadableBody`, never `body_text`.
- Structured output, and **any id absent from the batch is dropped**. A body that
  talks the model into proposing a message outside the net gets nothing.
- Reachable damage from a successful injection: one spurious suggestion, removed by
  Reject. No link, no stage movement, no ledger write.

## Code

**No migration.** Two queries in `internal/db/queries/mail_linking.sql`, then `make sqlc`:

- `ListEmailsForRecall` — owner, `deleted_at IS NULL`, `application_id IS NULL`,
  `received_at >= $since`, limit. A query of its own rather than new parameters on
  `ListEmails`, which serves the web inbox and seven assistant tools.
- `SuggestApplicationForEmail` — sets `suggested_job_id` + `match_confidence` where
  `application_id IS NULL`.

**New package** `internal/mailrecall`: a narrow `Store` (the two queries), an
`llm.Provider`, and the pure part — building the net from an application and
adjudicating the model's answer against the batch.

**Frontend**: the button and the result list in the Emails tab of
`web/src/lib/components/JobDrawer.svelte`, reusing the email row and the existing
confirm/reject calls. Types through `cmd/gen-contracts`.

## Tests

1. `TestMailRecallCannotLink` — no path in the package sets `application_id`. The
   same device `calmatch.Tier.Links()` uses: the rule is written so the next reader
   must answer it rather than trip over it.
2. An id absent from the batch is dropped.
3. An empty net does **not** call the model — a button on an application with no mail
   must not pay for nothing.
4. An HTML-only message reaches the model with a non-empty body.

Integration (`//go:build integration`): 404 for someone else's application, the
suggestion is written, a linked message is untouched.

## Out of scope

- A weaker calendar tier (matching an employer's name in an event title). Its code was
  written and deleted; returning it requires a confirmation surface, which this change
  happens to create — but it is a separate decision with its own evidence.
- Exposing `mailrecall` as an assistant tool.
- Any credit price for the action.
