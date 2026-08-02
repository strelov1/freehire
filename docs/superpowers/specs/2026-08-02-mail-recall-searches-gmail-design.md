# Mail recall searches Gmail, not the copy we kept

The mailbox sweep shipped in #1514 asks the wrong store. It reads the mail we already
synced; it should ask Gmail, scoped to the employer.

## What the measurements said

All of these are from production on 2026-08-02, one connected mailbox, 263 applications
that have mail. Nothing here is estimated.

**The cap is not a bound, it is the whole answer.** The net gathers unattached mail in a
window around `applied_at` and takes 40. Candidates in that window: minimum **96**, median
**158**, p90 **180**. **263 of 263 applications exceed the cap** — every single one, by
2.4× at best. Because the order is deterministic, pressing again returns the same 40 and
the remaining ~118 are unreachable by any number of presses.

**And the 40 it picks are picked for no reason.** The order is oldest-first, which carries
no information about relevance. Two real presses in production returned `scanned: 40,
suggested: []` in 10.3 s and 7.6 s.

**The employer's name cannot rescue the ordering.** Matching the catalogue company against
sender name or subject: median **0** matches per application; a match exists for only
**101 of 263** literally, **109** de-spaced, **110** on the first token. Where a match
exists the median is 2 messages. A name *filter* would therefore return nothing for six
applications in ten — which is the same lesson `mailclassify` already carries from the
other side (a corroboration guard would have dropped 16 of 99 correct links).

**Part of the mail is not in the store at all.** `gmailsync.BuildQuery` fetches only mail
from a known ATS domain or containing one of twelve phrases. Over 120 days it fetched
**431** messages where a loose hiring-shaped query finds **1151** and the mailbox holds
**3297** — so **739** messages are invisible to us. A sample of them contains an
acknowledgement (`We've received your a16z speedrun application!`), three interview
invitations (`micro1 interview invite`), and four live recruiter threads from personal and
corporate domains (`Re: Senior Backend Engineer` from `@op.tech`). The misses are near
misses on wording: the phrase list knows `invite you to interview` and not `interview
invite`, `your application at` and not `we've received your … application`.

**Asking Gmail per employer inverts the result.** Over 15 applications with **96–104**
candidates each and **zero** linked messages today, a single Gmail query scoped to the
employer found mail for **14 of 15** — 9 for Derq (a whole thread), 6 for truelogic
(including `Interview Invitation for Full Stack Engineer`), 5 for Jahnel Group (including
the calendar invitations), 1–3 for the rest.

## Decisions

### Gmail is the source of candidates; the database is the fallback

The sweep issues one Gmail search per press, scoped to the employer and the window, and
adjudicates what comes back. Typical result is 0–9 candidates rather than 158, so the cap,
the ordering and the whole ranking problem disappear rather than being tuned.

It works because **Gmail searches the body**, and our SQL net could not. That is the entire
difference between "median 0 name matches" and "found mail for 14 of 15".

A caller with no Gmail grant — a hosted mailbox, or the bring-your-own-harness tier — keeps
today's path over `emails`. Two paths, and the second is the one that already exists.

### The search is gated, and the gate was measured twice

Searching the mailbox for a company name reaches past the boundary the sync deliberately
draws: the product holds only job mail, and `Apple`, `Stone` or `Ramp` would pull personal
correspondence. So the query is `(employer) AND (job-shaped)`.

The first gate — hiring words only — was measured and **rejected**: over 20 applications it
cut 53 candidates to 41, and 8 of the 12 it dropped were real, including **both calendar
invitations** for a Jahnel Group interview and a live Derq thread. Calendar invitations
have no other route into the product: an interview appears on the calendar view only if its
invitation is linked.

The gate that ships adds two things and was re-measured: **`filename:ics`**, which every
calendar invitation carries whatever language its subject is in, and **the role title**,
which catches mail that names the job and never says "application". Result: 53 → **47**,
six dropped, five of them plainly noise (two Brazilian payment slips, two job-board
digests). The sixth — a Google Drive share, plausibly a take-home — is a known gap.

### A proposal is a Gmail message, and only a link imports it

A found message is not in `emails`, and the linking machinery works on rows. So a proposal
carries a Gmail id and lives on the screen only; pressing Link imports the message first,
then links it exactly as today.

That ordering is better than the one it replaces, not merely necessary: **nothing a person
has not confirmed is stored**. The sweep stops writing suggestions into the mailbox table
as a side effect of being pressed.

### Everything else is unchanged

The model still adjudicates, and still only proposes — the asymmetry that only a
deterministic signal may link is untouched, and the ids it returns are still validated
against the batch it was given. Confirmation stays on `POST /me/emails/:id/confirm|reject`.
There is still no calendar code: a linked invitation yields its meeting on the next
`cal-sync`.

## Risks / Trade-offs

- **A live Gmail call on the request path** → one search plus a small model call; the search
  is ~200–500 ms against the 7.6–10.3 s the model already costs. A Gmail failure is an
  error, not an empty result, for the same reason a model failure is.
- **Reaching past the sync's privacy boundary** → the gate above, measured. Worth restating
  in the UI: this searches your mailbox, it does not import it.
- **Two candidate paths to keep honest** → the fallback is the code that exists today; the
  risk is that only one gets maintained. A test should cover the fallback explicitly.
- **Gmail quota** → one search per press under an existing 20/hour limiter.
- **The role title is caller-supplied text in a search query** → quoted and stripped of
  quotes, in the manner of every other interpolated term.

## Out of scope

- Widening `gmailsync.BuildQuery`. The 739 missed messages are a real defect, but this
  change routes around them rather than fixing them, and the two should not be entangled.
- Extending `mailmatch.ExtractCompany`'s five subject templates, all of which use `to`,
  when the mailbox shows `at`, `with`, `as … at`, a leading company before a dash, a
  company after a pipe, and Portuguese. Same reason: separate defect, separate change.
- Re-linking mail already attached to another application — still the non-goal it was.
