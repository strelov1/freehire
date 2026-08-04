# Widen the mail fetch, and let the classifier keep the inbox clean

`gmailsync.BuildQuery` fetches too little, and the mail it misses is the mail that matters
most. Widening it is cheap; the thing that needs designing is what the inbox looks like
afterwards.

## What the measurements said

Production, 2026-08-02, one connected mailbox.

Over 120 days the sync fetched **431** messages. A loosely hiring-shaped query finds
**1151**, and the mailbox holds **3297**. So **739** hiring-shaped messages are invisible to
us. A 42-message sample of them contains:

- an acknowledgement — `We've received your a16z speedrun application!`
- three interview invitations — `micro1 interview invite`, `AI interview with micro1`,
  `start micro1 interview`
- four live recruiter threads from personal and corporate domains — `Re: Senior Backend
  Engineer` from `@op.tech`, `Re: Вакансия Senior Backend developer` from `@headshotit.com`
- nine of the candidate's **own replies** in those threads
- and roughly seventeen marketing messages — Jobright ×5, Jobgether ×4, Careerist ×2, an
  English course, job-board blasts

**The misses are near misses on wording.** The phrase list knows `invite you to interview`
but not `interview invite`; it knows `your application at` but not `we've received your …
application`. The second class is deeper and no phrase list reaches it: a recruiter writing
from a company domain in their own words.

**Cost is not the constraint, and I was wrong to think it was.** The whole production
corpus is **544** messages. 739 over 120 days is **~6 a day** — six extra classifications on
the cheap model. What widening actually costs is the inbox: about 40% of the addition is
marketing, and the inbox is where a person looks for their applications.

## Decisions

### The classifier already judges this; do not build a second judge

The obvious design is a blocklist of aggregator and course domains. It is the wrong one.
`mailclassify` already labels every message, and `other` means precisely "not about an
application at all". A Jobright digest is `other` by the vocabulary's own definition.

| | domain blocklist | the `other` label |
|---|---|---|
| New code | a list, and forever maintaining it | none |
| Who judges | us, in advance, by sender | the classifier, by content |
| A new marketing sender | lands in the inbox until we notice | hidden immediately |
| A mistake | invisible until somebody reports it | visible under "show all" |

The list would have to be curated indefinitely against an adversary who registers domains
for a living. The label is already computed, on a call we are already paying for.

### The filter belongs at DISPLAY, not at fetch

The sync carries a watermark: `BuildQuery(afterUnix)` plus the `SetSynced` cursor. A message
not fetched today is **never** fetched — the cursor has moved past it. Excluding at fetch
time is therefore silent and permanent, and a mistake in the rule cannot be repaired by
fixing the rule.

At display nothing is lost: the message is in the store, and correcting the filter reveals
it. This is the same reasoning that made the recall net rank rather than drop, and it is
newly affordable because **the recall sweep no longer reads our copy** — since #1531 it
searches Gmail directly, so extra rows in `emails` no longer degrade it. The only thing they
degrade is a list a person reads, and that is exactly what a display filter fixes.

### Hidden must be visible as a number

An inbox that quietly drops `other` is an inbox where a misclassification cannot be found.
The listing therefore reports how much it hid, and one control shows it. A filter with no
indicator is the silent-failure class this codebase keeps re-learning.

### The candidate's own mail is not incoming mail

Nine of 42 sampled misses were the candidate's own replies. `gmailsync` already carries the
connected address to skip own replies while walking a thread; the widened query needs the
same exclusion at the top level, or every conversation arrives twice — once as the
employer's message and once as the answer to it.

## Risks / Trade-offs

- **A marketing message classified as something other than `other`** → it shows in the
  inbox, exactly as today. No regression; the widening only adds candidates.
- **A real message classified `other`** → hidden, and this is the change's sharpest edge.
  Mitigated by the count and the control, not by a promise.
- **~6 extra classifications a day, plus a one-off backfill of ~739** → negligible against a
  544-message corpus, and the model is the cheap one.
- **The wider query is a Gmail search string that must stay under its length limits** →
  the phrase list is already long; the addition is bounded and tested.

## Out of scope

- `mailmatch.ExtractCompany`'s five `to`-only subject templates. Measured separately: 24
  unattached messages have an unparseable subject and only **9** of those name an employer
  the caller has applied to. The prize is small and the risk is not — `ExtractCompany` feeds
  `TierName`, which auto-links, so a loose template creates wrong links rather than merely
  missing right ones.
- Any change to linking, stages, or the ledger.
