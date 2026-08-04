## Why

The Gmail sync fetches too little, and what it misses is what matters most.

Over 120 days it fetched **431** messages from a mailbox holding **3297**; a loosely
hiring-shaped query finds **1151**. So **739** hiring-shaped messages are invisible. A
42-message sample contains an acknowledgement (`We've received your a16z speedrun
application!`), three interview invitations (`micro1 interview invite`), and four live
recruiter threads from personal and corporate domains (`Re: Senior Backend Engineer` from
`@op.tech`).

The misses are near misses on wording: the phrase list knows `invite you to interview` but
not `interview invite`, `your application at` but not `we've received your … application`.

**Cost is not what constrains this.** The whole production corpus is **544** messages; 739
over 120 days is about **six a day** on the cheap model. What widening costs is the inbox —
roughly 40% of the addition is marketing, and the inbox is where a person looks for their
applications.

## What Changes

- **The fetch widens**: the measured near-miss phrasings plus broader hiring vocabulary,
  which took the same mailbox from 431 to 1151 over the window.
- **The candidate's own replies are excluded.** Nine of 42 sampled misses were their own
  mail; the sync already carries the connected address to skip them while walking a thread
  and needs the same exclusion at the top level.
- **The inbox hides `other` by default** — the label the classifier already applies to mail
  that is not about an application at all. No blocklist is introduced: the classifier
  already judges every message on a call we already pay for, and a curated domain list would
  need maintaining forever against senders who register domains for a living.
- **The listing reports how much it hid**, and one control shows it. A filter with no
  indicator makes a misclassification unfindable.
- The `gmail-ats-sync` spec is corrected while we are here: it claims non-ATS mail is
  "never fetched or stored", which stopped being true when the phrase clauses were added.

No change to linking, stages, or the ledger.

## Capabilities

### New Capabilities
<!-- None. -->

### Modified Capabilities
- `gmail-ats-sync`: the fetch is scoped to hiring-shaped mail rather than to ATS senders,
  and excludes the caller's own messages.
- `email-inbox`: the listing hides mail classified `other` by default, reports the count it
  hid, and can show it.

## Impact

- **`internal/gmailsync/senders.go`**: the phrase list and the query builder; an exclusion
  for the connected address.
- **`internal/db/queries/gmail.sql`**: the listing gains a "hide `other`" predicate and the
  count of what it hid. `make sqlc`.
- **`internal/inbox`**: the query option and the count on the page.
- **Web**: the inbox surfaces the hidden count and the control.
- **No migration.** No new column: the label already exists.
- **Backfill**: none required. The widened query picks new mail up from the next sync
  onward; the 739 already-missed messages stay missed unless somebody re-syncs from an
  earlier watermark, which is deliberately not part of this change.
