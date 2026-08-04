## Context

`gmailsync.BuildQuery` scopes the fetch to a curated ATS sender list OR twelve multilingual
phrases. Everything it fetches enters `email_classification_outbox` and is labelled by
`mailclassify`, whose vocabulary already includes `other` — "anything not about an
application".

Measured on production, 2026-08-02, one connected mailbox: **431** fetched over 120 days,
**1151** found by a hiring-shaped query, **3297** in the mailbox. The **739** difference
contains an acknowledgement, three interview invitations, four live recruiter threads from
personal and corporate domains, nine of the candidate's own replies, and about seventeen
marketing messages. The whole corpus we hold is **544** messages, so the addition is roughly
**six a day**.

## Goals / Non-Goals

**Goals:** fetch the mail employers actually send; keep the inbox a place to find
applications; add no state that needs maintaining.

**Non-Goals:** back-filling the 739 already missed (the watermark has passed them; a re-sync
is an operator action, not this change); `ExtractCompany`'s subject templates; anything
touching linking, stages or the ledger.

## Decisions

### The classifier is the filter; no blocklist is introduced

The obvious design is a list of aggregator and course domains. Rejected. `mailclassify`
already labels every message, `other` already means "not about an application", and the
label is produced by a call the system already makes. A domain list would be a second judge,
curated by hand, forever, against senders whose business is registering domains — and it
judges by sender where the classifier judges by content, so a marketing message from a new
domain lands in the inbox until somebody notices.

### The filter is at DISPLAY, and that is load-bearing

The sync carries a watermark (`BuildQuery(afterUnix)` + the `SetSynced` cursor), so a
message not fetched today is never fetched. Filtering at fetch time is silent and permanent:
a wrong rule loses mail that fixing the rule will not bring back.

At display nothing is lost — the row is stored, and correcting the filter reveals it. This
became affordable in #1531: the recall sweep now searches Gmail rather than our copy, so
extra rows no longer degrade it. The only thing they degrade is a list a person reads.

### Hidden is reported as a number

An inbox that silently drops `other` is one where a misclassification cannot be found, and
`mailclassify` reads attacker-controlled text. The listing therefore returns how many it
omitted, and one control shows them. Unclassified mail is never hidden: nothing has judged
it.

### The candidate's own mail is excluded at the query, and storage was already safe

Nine of 42 sampled misses were the candidate's own replies. It is worth being exact about
what that costs, because the first draft of this design was wrong about it:
`worker.go` already drops them before storing (`strings.EqualFold(msg.FromAddr, u.Email)`),
so nothing was ever going to be stored twice.

What they cost is the fetch. Every one is a message id listed and a full message body
retrieved, only to be discarded — and under a widened query they compete for the page the
search returns. Excluding them at the query is therefore an efficiency measure with no
behavioural change, which is the honest reason to do it and a much smaller one than
"correctness".

## Risks / Trade-offs

- **A real message labelled `other` is hidden** → the change's sharpest edge, mitigated by
  the count and the control rather than by a promise.
- **Marketing labelled as something else** → shows in the inbox, exactly as today.
- **~6 extra classifications a day** → negligible on a 544-message corpus and the cheap
  model.
- **Query length** → Gmail bounds search strings; the addition is bounded and tested.

## Migration Plan

No migration, no new column, no backfill. The widened query applies from the next sync; the
already-missed 739 remain missed unless an operator re-syncs from an earlier watermark.

## Open Questions

None blocking. Worth revisiting with data: whether `other` proves too coarse a filter — if
real mail lands in it often, the answer is to sharpen the classifier, not to add the
blocklist this design rejected.
