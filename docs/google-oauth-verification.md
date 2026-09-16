# Google OAuth verification

What it takes to lift the "Google hasn't verified this app" screen from the Inbox
connection, and the text to paste into the forms when we do it again.

**This is a runbook, not a design doc.** None of it can be automated: Google exposes no
API for submitting verification, and the review is conducted over email with the project
owners. Treat every step below as a manual one.

## Why it costs anything at all

Scopes come in three tiers, and the tier — not the feature — decides the burden.

| Scope | Tier | What it costs us |
|---|---|---|
| `gmail.readonly` | **restricted** | Verification **plus an annual third-party security assessment (CASA)** |
| `calendar.readonly` | sensitive | Verification only |
| `calendar.events` | sensitive | Verification only |
| sign-in (`email`, `profile`) | basic | Nothing |

So the whole cost of this exercise is one scope. If Gmail sync is ever retired in favour
of the SES inbound path (`cmd/mail-ingest`, where the candidate sets up forwarding
themselves), the assessment requirement goes with it and only the free verification
remains — that is the real rollback, and it is worth re-pricing at each renewal.

**The free self-scan tier is gone.** Google used to accept a Tier 2 self-scan run with
open-source tools at no cost. As of 2026 restricted-scope apps get a limited set of
assessment options, all paid, with a rate Google negotiated with TAC Security. Budget
roughly **$500–1500 per year**, recurring. There is no open-source discount; Google prices
on a risk score derived from the scopes and the architecture, not on the licence.

Until we submit, the app can stay in **Testing** publishing status, which admits up to 100
named test users with no warning screen. That is the current state and it is a fine place
to sit while the feature is still finding its shape.

## Order of operations

The privacy policy must be **live** before submitting — a reviewer opens the public URL,
not the repo. Ship it, then submit.

1. Deploy the privacy policy covering Gmail (the "Gmail and Google Calendar" section of
   `web/src/routes/privacy/+page.svelte`, added in freehire#2888). It has to carry the
   verbatim Limited Use sentence and describe each scope, or the submission is refused
   before anyone looks at the product.
2. Verify domain ownership of `freehire.me` with Google. `dig +short TXT freehire.me`
   already returns a `google-site-verification=` record (measured 2026-09-16), so this is
   most likely done from the Search Console setup. **Confirm the account, not the record**
   — the verification has to sit under the same Google account that owns the Cloud
   project, and a record proving some account owns the domain looks identical to one
   proving the right one does.
3. Google Cloud Console → OAuth consent screen → **Publish App**. Apps in testing are not
   eligible for review.
4. Fill the branding information and submit for **brand verification** (app name, logo,
   homepage, privacy policy and terms URLs). This gate comes first — data-access
   verification cannot be requested until branding is approved.
5. Record the demo video (script below), upload it somewhere publicly fetchable, and
   submit for **data access verification** with the scope justifications below.
6. Google assigns a CASA assessor for `gmail.readonly`. They run a DAST scan against
   production and send a self-assessment questionnaire.

Keep the project's owner/editor contact addresses current: **every message from the review
team arrives by email**, and a thread nobody reads stalls the whole submission.

Re-verification is triggered by adding redirect URIs or JavaScript origins, or by renaming
the product. Worth remembering before a routine-looking config change.

## Branding assets

The consent screen wants a **square 120x120** logo. Derive it rather than drawing one, so
the consent screen and the installed app stay the same mark:

```sh
sips -Z 120 web/static/pwa-512x512.png --out /tmp/freehire-oauth-logo-120.png
```

Not committed, because it is derived and the site never serves it — one command is a
smaller thing to keep true than a binary that silently drifts from the icon it was cut
from. The other four branding fields are already live and were checked on 2026-09-16:
`freehire.me`, `/privacy` and `/terms` all answer 200, and the footer links the last two
from every page, which is where a reviewer looks for them.

## Scope justifications

Paste these into the form. They are written to answer the question the reviewer is
actually asking — *why does this app need to read mail at all, and why can't a narrower
scope do it* — because a justification that only restates the feature is the common
rejection.

### `https://www.googleapis.com/auth/gmail.readonly`

> freehire helps a candidate track the jobs they have applied to. Employers reply by
> email, almost always through an applicant-tracking system (Greenhouse, Ashby, Workable,
> Lever and others), so the state of an application — acknowledged, interview scheduled,
> rejected, offer — exists only in the candidate's inbox. The Inbox feature reads those
> replies and advances the matching application's stage automatically, which is the whole
> point of the feature: without it the candidate re-types into our tracker what their
> mailbox already knows.
>
> We do not read the mailbox. Every sync issues a Gmail search restricted to
> hiring-shaped mail: messages from a curated list of applicant-tracking-system sender
> domains, or messages carrying recognised application and interview phrasing, excluding
> anything the connected address itself sent. Messages outside that query are never
> fetched.
>
> A narrower scope cannot do this. `gmail.metadata` returns headers only, and the subject
> line does not distinguish a rejection from an interview invitation — both commonly read
> "Your application to <Company>". We must read the message body to classify the stage
> correctly, and body access is only available through a restricted read scope. We do not
> request `gmail.modify`, `gmail.send`, or full mailbox access: we never write, label,
> delete, or send anything, and read-only is the least privilege that delivers the
> feature.
>
> Classification runs on our own servers against a fixed keyword vocabulary first. Only a
> message that vocabulary cannot place is sent to our language-model provider, and then
> only the sender, the subject, and the first 4,000 characters of the body. That provider
> processes the text to answer the single request and is contractually barred from
> training on it. No freehire employee reads user mail except where a user explicitly asks
> us to look at a specific message for support.
>
> Disconnecting revokes the refresh token with Google and deletes every message we synced
> from that account. The token is encrypted at rest with AES-256.

### `https://www.googleapis.com/auth/calendar.readonly`

> Candidates using the Inbox feature see their interviews on the same timeline as their
> applications, so a scheduled round is visible next to the application it belongs to. We
> read free/busy and event times to place them on that timeline. We never create, modify,
> or delete an event with this scope.

### `https://www.googleapis.com/auth/calendar.events`

> Requested only from a mentor who offers paid mentorship sessions on freehire, and only
> at the moment they opt in to taking bookings — never from a candidate. When someone
> books a session, we create that one calendar event on the mentor's calendar and attach
> a Google Meet link, which is what the two parties join. We create nothing else and
> delete nothing. Google offers no narrower scope for creating a single event.

## Demo video script

Google wants to see the OAuth flow and each scope being used, from the user's point of
view. Screen recording with no narration is fine; keep it under about three minutes and do
not cut away from the consent screen.

1. **Where it starts.** Open `freehire.me`, sign in, and navigate to the Inbox feature the
   ordinary way — through the account navigation, not a deep link. Show that it is empty
   and offers to connect a Google account. This establishes the feature exists before the
   grant, which is the framing the reviewer is checking.
2. **The consent screen, in full.** Click connect. Let the Google consent screen render
   completely and stay on it long enough to read every requested scope. Do not speed this
   up or cut it.
3. **`gmail.readonly` in use.** Land back on the Inbox with hiring mail populated. Open a
   message and show the classification it produced and the application it linked to.
4. **The stage advance.** Show the tracked application whose stage moved because of that
   message. This is the payoff that justifies the scope — a reviewer who sees only a
   mail list will ask why we needed the body.
5. **`calendar.readonly` in use.** Show an interview appearing on the timeline next to its
   application.
6. **`calendar.events` in use.** Separately, as a mentor account: opt in to bookings, have
   a session booked, and show the event and Meet link created on the mentor's calendar.
   Make it visible that this is a different account and a different opt-in from step 2.
7. **Disconnect.** Go to account settings, disconnect the Google account, and show the
   synced mail is gone. Reviewers look for a working revocation path, and it is the
   cheapest part of the video to shoot.

Shoot steps 1–5 and 7 in one take if you can. A video assembled from fragments invites the
question of whether it is the same account throughout.

## What the assessor will ask first

Mail bodies leave our infrastructure for a third-party language-model provider. Have the
answer ready in the shape the code actually has it, not as a reassurance:

- `internal/application/mailclassify` tries `KeywordStatus`, a fixed vocabulary, on our own
  servers before any model call.
- Only an undecided message reaches the model, capped at 4,000 characters of body
  (`maxBodyRunes`) and a truncated subject.
- The refresh token is AES-256 encrypted at rest under `GMAIL_TOKEN_KEY`.
- `DELETE /me/gmail` revokes at Google and deletes the synced rows in the same handler.
- `internal/application/mailrecall` also reads Gmail, and reads *less*: one search scoped
  to a single employer, returning proposals that are never stored and never linked.

See [agents/mail-stack.md](agents/mail-stack.md) for how those packages compose.
