## Context

`internal/mailrecall` shipped in #1514. It reads unattached mail from `emails` in a window
around `applied_at`, oldest first, capped at 40, and hands that batch to one model call.
Every number below is from production on 2026-08-02, measured against one connected mailbox
and 263 applications that have mail.

- Candidates in the window: min **96**, median **158**, p90 **180**. **263 of 263**
  applications exceed the cap of 40. The order is deterministic, so a second press returns
  the same batch and ~118 messages are unreachable.
- Two real presses: `scanned: 40, suggested: []`, in 10.3 s and 7.6 s.
- The employer's name matched against sender name + subject: median **0**; present at all
  for 110 of 263.
- `gmailsync.BuildQuery` fetched **431** messages over 120 days where the mailbox holds
  **3297**; **739** hiring-shaped messages were never fetched, including an acknowledgement,
  three interview invitations and four live recruiter threads.
- One Gmail query per employer, over 15 applications carrying ~100 candidates and **zero**
  links: found mail for **14 of 15**.

## Goals / Non-Goals

**Goals:**

- Candidates that are about the application, not about the window it sits in.
- Reach mail the sync never fetched.
- Keep the interaction synchronous: a press still answers in one wait.
- Keep the mailbox out of our store until the caller says otherwise.

**Non-Goals:**

- Widening `gmailsync.BuildQuery`. The 739 missed messages are a real defect; this change
  routes around it and the two should not be entangled.
- Extending `mailmatch.ExtractCompany`'s subject templates (all five use `to`, while the
  mailbox shows `at`, `with`, `as … at`, a leading company before a dash, a company after a
  pipe, and Portuguese). Separate defect, separate change.
- Re-linking mail already attached to another application.
- Removing the stored-mail path. It is the fallback.

## Decisions

### The search replaces the net; the net becomes the fallback

`Recall` gains a second candidate source. Where a searchable mailbox exists it issues one
query and adjudicates 0–9 results; otherwise it runs today's `ListForRecall`. Both feed the
same adjudication, the same discard-outside-the-batch rule and the same proposal shape, so
only the source of candidates forks.

*Why not tune the net instead:* the cap was raised, reordered and re-windowed once already
in #1514 and the result is still 40 of 158 chosen by date. The defect is not the cap's
value; it is that the store cannot answer "about this employer" at all. Gmail can, because
it indexes bodies.

*Why not parallel batches over the stored net:* it would make coverage complete and keep
every candidate irrelevant. Four calls to reject 158 messages is worse than one call to
judge 6.

### The query, and both halves of its gate

```
after:<applied−7d> before:<applied+90d>
("<company>" OR "<company without spaces>")
(<hiring vocabulary> OR filename:ics OR "<role title>")
```

The employer clause carries the de-spaced variant because the catalogue writes `Blend 360`
and the sender signs `Blend360` — the same class of mismatch that killed the calendar
title tier, which compared against a hyphenated slug.

The gate exists because searching a mailbox for a company name reaches past the boundary
the sync draws: the product holds only job mail, and `Apple` or `Ramp` would pull personal
correspondence.

Its first version — hiring vocabulary alone — was **measured and rejected**: over 20
applications it cut 53 candidates to 41, and 8 of the 12 dropped were real, including
**both calendar invitations** for a Jahnel Group interview and a live Derq thread.
Invitations have no other route in: an interview reaches the calendar view only if its
invitation is linked.

The shipping gate adds `filename:ics`, which every invitation carries whatever language its
subject uses, and the role title, which catches `Re: Full-Stack Engineer (Scalable
Systems)` — a real message with no hiring word in it. Re-measured: 53 → **47**, six
dropped, five plainly noise (two payment slips, two job-board digests). The sixth, a Drive
share plausibly carrying a take-home, is a known gap and is written down rather than
hidden.

### A proposal is a message id, not a row

The search returns messages that are not in `emails`, and every linking path works on rows.
So a proposal carries the provider's message id and the fields needed to draw it, and
nothing is persisted until the caller links.

This is better than the order it replaces, not merely forced by it: #1514 wrote a
suggestion for every confident answer, so pressing the button planted state whether or not
anybody agreed with it. Now what a person has not confirmed is not kept.

Linking therefore imports first. The import reuses the sync's own store path so a message
arrives identical to one the worker would have fetched, and is idempotent on
`(source, external_id)` — a message the sync had already stored is linked, not duplicated.

### Failure is loud on both new paths

A search failure joins the model failure as an error rather than an empty result, for the
same reason: the caller is waiting, and "nothing found" is a different sentence from "we
could not look". `mailrecall.ErrModel` gains a sibling, and the handler keeps rendering
anything that is neither as a plain 500 so it reaches the error tracker.

## Risks / Trade-offs

- **Live Gmail call on the request path** → ~200–500 ms against the 7.6–10.3 s the model
  already costs; well inside the latency budget that ruled out raising the cap.
- **Two candidate paths** → the fallback is today's code, and the risk is that only one
  stays maintained. A test covers the fallback explicitly.
- **Reaching past the sync's privacy boundary** → the measured gate, plus a UI line saying
  this searches the mailbox rather than importing it.
- **Employer names that are common words** (`Stone`, `Ramp`, `Apple`) → the gate is what
  stands between them and personal mail; its false-negative cost is measured, its
  false-positive cost is one proposal to reject.
- **Gmail quota and abuse** → one search per press, under the existing 20/hour limiter.
- **Role title interpolated into a query** → quoted and stripped of quotes, like every other
  interpolated term.

## Migration Plan

No migration and no backfill. Confirmed messages are stored by the existing ingest path.
Deploy is ordinary; the endpoint keeps its shape, and a deployment whose Gmail client is
unconfigured falls back to the stored path rather than failing.

## Open Questions

None blocking. Two to revisit with data: whether the window still earns its ±90 days once
the search does the narrowing, and whether the Drive-share class (a take-home behind a link)
deserves a gate term of its own.
