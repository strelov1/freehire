## Why

We measure application silence carefully and then do nothing with it.

`internal/userjob/silence.go` carries a threshold ladder with per-value provenance — `applied` 21
days (measured over 92 observed applications, marks 16% of them), `interview` 12 (measured over 6,
raised from 7 because a badge on nearly every card is one nobody reads) — three states that
distinguish *silent* from *waiting and fine* from *unconfirmed*, and it already feeds the ghost
signal. `/me/tracking` serves `days_silent` and `silence_state` per row, and `BoardCard.svelte`
already renders "No reply for N days".

So the candidate is told, accurately and at the right moment, that an application has gone quiet —
and is offered nothing to do about it. `grep -rn "follow.?up" internal/` returns comments only;
`internal/reminder` nudges *saved jobs*, not applications. The signal stops one step short of being
useful.

## What Changes

- **A follow-up draft for a silent application.** The candidate opens it from the card that already
  carries the badge, gets a short message they can send from their own mail client, and the drafting
  costs nothing: it is assembled deterministically, not generated.
- **The one non-generic line comes from the cached fit analysis.** A chase that only says "checking
  in" is ignorable; the draft names one concrete strength. That text already exists — the fit
  analysis records the candidate's strongest evidence for this vacancy, drawn from their own CV — so
  the draft restates something true rather than inventing a reason.
- **The recipient is prefilled when we know it.** `emails.from_addr` gives a real address on
  applications where somebody replied and then went quiet. On the commonest silent case — applied,
  never answered — there is no address to prefill, and the draft is issued without one.
- **A sent follow-up is recorded, and does not stop the clock.** `user_jobs.followed_up_at` records
  that the candidate chased. `last_activity_at` is `GREATEST(applied_at, newest inbound mail)` — it
  measures when the *other side* last moved, and a chase is not a reply. So silence keeps accruing
  and the board can say "chased 4 days ago, still nothing" instead of resetting to a reassuring
  green.

Not in this change, deliberately:

- **Sending on the candidate's behalf.** `emailnotify.Client.Send` could do it and the hosted mailbox
  gives a plausible From, but a chase sent from our infrastructure to a recruiter is a
  deliverability and identity decision that deserves its own change — and it would only ever work for
  the minority of silent applications where we hold an address.
- **An LLM draft.** A template plus one line of the candidate's own evidence needs no credits, no
  metering, and cannot fabricate a claim. If the drafts prove too generic in use, an LLM pass is a
  later upgrade with a baseline to beat.
- **A batch digest** ("three applications went quiet"). That is a notification feature over the same
  data; this change puts the action next to the badge that already exists.

## Capabilities

### New Capabilities
- `application-followup-draft`: assembling a follow-up message for a silent application, prefilling
  the recipient when one is known, and recording that the candidate chased.

### Modified Capabilities
- `application-silence-signal`: a recorded follow-up is now part of what the board reports about a
  silent application, and — the requirement worth pinning — it does NOT reset the silence clock.

## Impact

- **Schema**: `user_jobs.followed_up_at timestamptz` (additive) and its backfill-free migration.
- **SQL**: `ListTrackedJobs` and the tracking detail read carry the new column; a small write to set
  it. Regenerate with `make sqlc`.
- **Go**: a new deterministic assembler (pure, testable without a DB or an LLM), a read endpoint for
  the draft, and a write endpoint recording the chase.
- **Web**: an action on the board card behind the existing silence badge, and the card's copy when a
  follow-up has been recorded.
- **No LLM, no credits, no new external dependency.**
