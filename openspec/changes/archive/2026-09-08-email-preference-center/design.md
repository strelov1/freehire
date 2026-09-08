## Context

Ten call sites render a mail through `internal/application/mailtpl` (layer 6, the
shell: header, card, fine-print footer). Two of them — the address-verification and
password-reset mails in `internal/engage/emailnotify/authmailer.go` — set
`Body.Essential: true`, which is the only thing the footer branches on today. The
other eight get an unconditional link to `/my/notifications`, which the account
shell (`web/src/routes/my/+layout.server.ts`) gates on a session cookie.

The transport under all of them is `internal/engage/emailnotify.Client`, a thin
adapter over SES v2 `SendEmail` that already carries three overlapping methods:

```go
Send(ctx, from, to, subject, htmlBody, textBody)                        // client.go:42
SendWithReplyTo(ctx, from, replyTo, to, subject, htmlBody, textBody)    // client.go:54
SendWithAttachments(ctx, from, to, subject, htmlBody, textBody, attach) // attachment.go:33
```

Six packages declare their own narrow consumer-side interface over one of these
(`emailnotify.Sender`, `broadcast.Sender`, `onboarding.Sender`, `report.EmailSender`,
`referral.EmailSender`, `emailnotify.AttachmentSender`), and `engage/mailpreview`
implements two of them to capture rendered mail for the preview harness.

The state of the account-level flag matters more than it first appears.
`notification_settings.enabled` is read two contradictory ways across the codebase:

| Reader | Join | A missing row means |
|---|---|---|
| `queries/nudges.sql:12,33,48,64,85,100` | `JOIN ... AND ns.enabled` | do **not** send |
| `queries/broadcast.sql:20,34` | `LEFT JOIN ... COALESCE(ns.enabled, true)` | **do** send |
| `queries/onboarding.sql:23,44,...` | `LEFT JOIN ... COALESCE(ns.enabled, true)` | **do** send |

Both readings are deliberate and documented in their own files. Together they mean
a person who has never opened the settings page — which is most people, since the
row is created on demand — receives campaigns and the onboarding drip and has no
way to decline them, while the same person receives no nudges at all. One column is
answering two different product questions with two different defaults.

## Goals / Non-Goals

**Goals:**

- A person holding one of our mails can turn off the kind of mail they are holding,
  and inspect every other kind, without an account, a password, or a support ticket.
- The mail client's own unsubscribe button works (RFC 8058), which is what Gmail and
  Yahoo have required of bulk senders since February 2024.
- Declining campaigns stops campaigns and nothing else.
- A mail added a year from now cannot ship without a way out of it, enforced by a
  test rather than by review attention.

**Non-Goals:**

- Telegram, push, and webhook channels. They keep the controls they have. Email is
  the channel a person can receive without having connected anything, so it is the
  one where "I never asked for this" is a real position.
- A preference row per mail *kind*. Three groups is the granularity; a switch per
  nudge type is the kind of settings page nobody reads.
- Reworking `/my/notifications`. The authenticated page keeps working and gains the
  two new switches; the public page is a second, narrower view of the same state.
- Suppression by address. Preferences hang off a user id. An address with no account
  never received a non-essential mail from us in the first place.

## Decisions

### A stateless signed token, not a token table

The link carries `<userID>.<group>.<base64url(HMAC-SHA256)>`, signed with the JWT
secret under a fixed, distinct salt (`"email-prefs-v1"`) and compared in constant
time. Nothing is stored and nothing expires.

Only the signature is base64; the id and group ride in the clear. Every character is
already URL-unreserved, so an outer encoding would buy nothing but a decode step, and
the token stays readable in a log line without one. Base64 is not secrecy, so this
conceals nothing it otherwise would.

*Why not a token table:* a stored token buys revocation and open-tracking. Neither
helps here. It costs a row written per recipient per mail — thousands per campaign —
plus a retention sweep, and it introduces a way for the unsubscribe link to fail
(row pruned, row missing) in the one flow that must never fail.

*Why no expiry:* CAN-SPAM requires the opt-out mechanism to work for at least 30
days after sending, and a person who unsubscribes from a mail they found in an old
archive is expressing exactly the preference we want to record. An expired
unsubscribe link is a worse outcome than a long-lived one. The blast radius is
bounded by what the token unlocks: three switches and a list of saved-search names,
nothing else.

*Why a distinct salt:* deriving from the JWT secret without one would make an
unsubscribe token and a session token interchangeable inputs to the same verifier.
Rotating the JWT secret invalidates every outstanding link, which is the intended
and only revocation path. A test mints one of each and asserts neither parses as the
other, in both directions — the property is mutual, and the two formats happening not
to collide today is not the reason it holds.

*Why not `internal/platform/linktoken`:* the repository already has a stateless
HMAC link token, and it was considered. It does not fit, and forcing it would damage
the caller it already serves. Expiry is structural there — `PayloadLen = 16` is
eight bytes of user id and eight of expiry unix — and expiry is precisely what this
token may not have; removing it means changing a payload format that live Telegram
deep-link tokens are parsing right now. Its MAC is truncated to 16 bytes on the
stated grounds that it is "ample for a short-lived token", which is an argument that
does not survive a credential with no lifetime. And its payload is fixed-width
binary where this one carries a variable-length group name. Widening a layer-1
primitive with one live consumer to serve a second whose requirements are the
opposite of the first's is how a shared package acquires a parameter nobody
understands. If a third stateless link token appears, revisit — two is the point at
which a common primitive is a guess and three is when it is a fact.

### The token names a group, and one-click honours it

`List-Unsubscribe` must point at something that acts without a confirmation screen —
Gmail POSTs to it and tells the user they are unsubscribed. So the URL cannot be
generic; it has to know which mail it came from. Encoding the group in the token is
what makes a one-click on a digest stop digests rather than everything.

*Alternative considered:* one-click turns off all non-essential mail. Safer for
sender reputation and simpler, but it means a stray tap on one campaign silences the
job alerts the person actually wants, and those are the product. Rejected. The
one-click response body is a page confirming what was turned off, with a link to the
full preference page — so a person who did want everything off is one click away.

### Two new columns rather than a new table or a rewritten flag

```sql
ALTER TABLE public.notification_settings
    ADD COLUMN alerts_email_enabled boolean NOT NULL DEFAULT true,
    ADD COLUMN news_email_enabled   boolean NOT NULL DEFAULT true;
```

`enabled` keeps its exact present meaning and both of its present readings, now
scoped to `activity` alone. The campaign and onboarding queries move from
`COALESCE(ns.enabled, true)` to `COALESCE(ns.news_email_enabled, true)`; the nudge
queries are untouched.

*Why this is safe:* `DEFAULT true` on a `NOT NULL` column plus `COALESCE(..., true)`
for the missing-row case means every account keeps receiving exactly what it
receives today. Nobody is opted out and nobody is opted in by the migration itself.

*Why not a new `email_preferences` table:* `notification_settings` is already the
per-user, PK-on-`user_id` home for this question, and it has already been widened
once for the same reason — migration 0082 renamed `reminder_settings` to
`notification_settings` when a second family of mail needed the same gate. A third
family goes through the same seam. A table beside it would be a second answer to
"is this person opted in", and the drift between them would look exactly like a bug.

*Why not repurpose `enabled` and add `activity_enabled`:* it would require rewriting
the meaning of an existing column's stored values, and the two readings above mean
there is no single correct rewrite.

### Collapse the transport's three send methods into one struct

```go
type Message struct {
    From, To, Subject, HTML, Text string
    ReplyTo     string
    Headers     map[string]string
    Attachments []Attachment
}
func (c *Client) Send(ctx context.Context, m Message) error
```

*Why now:* the headers have to go somewhere, and a fourth `SendWithHeaders` — or
worse, `SendWithReplyToAndHeaders` — is the combinatorial dead end the third method
already warned about. SES v2 supports this directly:
`sesv2/types.Message.Headers []MessageHeader` (present in the pinned v1.66.x).

*Secondary benefit:* six same-typed positional string parameters is a signature
where `from` and `to` can be swapped without the compiler noticing.

*Cost:* six consumer interfaces and `mailpreview`'s capture change shape. Mechanical
and compiler-checked. CLAUDE.md's "MVP stage — keep the architecture fluid; prefer
reshaping the affected part over bolting on an awkward special case" is the licence.

### The gate lives in SQL, the URL is passed in

Each "who do we mail" query gains its group's predicate, so a notifier cannot forget
one: it receives an already-filtered list. `mailtpl` stays ignorant of tokens — it
gains an `UnsubscribeURL` field its caller fills — because `mailtpl` is layer 6
(`application`) and `emailprefs` is layer 7 (`engage`), and layer 6 may not see it.
That constraint produces the right design anyway: the template renders a URL, it
does not mint one.

### The guard is an AST walk, not a list

A test walks the module's syntax tree for every composite literal of type
`mailtpl.Body` and asserts each either sets `Essential: true` or sets
`UnsubscribeURL`. A hand-maintained list of senders checked against the same
hand-maintained list proves consistency, not coverage — the repo has been bitten by
exactly that twice. The `mailpreview` package already reads senders out of the AST,
so the technique and its helpers exist here.

## Risks / Trade-offs

**A forwarded mail hands its unsubscribe link to whoever received it** → The token
unlocks three booleans, a set of saved-search subscription flags, and the account's
own email address — no CV, applications, billing, or session. The page is `noindex`
and returns no other account data. This is the standard trade every bulk sender
makes; the mitigation is scope, not secrecy.

**Someone brute-forces tokens to mass-unsubscribe users** → HMAC-SHA256 over a
secret makes forgery infeasible; the routes are rate-limited like every other public
route. Worst realistic case is a targeted nuisance against a known user id, which
requires the secret.

**The token is a never-expiring bearer credential in a URL, and nginx logs URLs** →
`deploy/nginx/snippets/freehire-app.conf:14` logs every request with the query string
included, which is why `internal/application/viewlog` has to strip it. So every
`?t=…` a recipient clicks is written to `/var/log/nginx/access.log` and lives as long
as its rotations do. That is a bulk store, unlike the forwarded-mail case above,
which reaches one person. Three mitigations, and this change takes all three:

- `PATCH` and the "unsubscribe from everything" call take the token in the request
  **body**, not the query. Only the initial `GET` needs it in the URL, because a link
  in an email has nowhere else to put it.
- The page strips `?t=` from the address bar after its first read, so the token does
  not then travel into browser history, a screenshot, or a pasted URL. Use
  `onRouterReady`, never `replaceState` in `onMount` — that combination throws only
  in a production build, where the error is also unreadable.
- `access_log off` for the unsubscribe location in the nginx snippet. The one-click
  `POST` has no alternative: RFC 8058 fixes the body to `List-Unsubscribe=One-Click`,
  so the token must be in the URL Gmail was given, and switching the log off for that
  path is the only place left to fix it. Note this is a **host** change — nothing in
  `deploy/` deploys itself, so the edit is only half done until it is copied to the
  machine and nginx is reloaded.

Residual risk after all three: an operator reading the access log during the window
before rotation could unsubscribe someone. An operator already has the database.

**One-click scoped to a group leaves a person still receiving other mail, and they
mark it spam instead** → The confirmation page names exactly what was turned off and
offers "unsubscribe from everything" as one further click. Watch complaint rate in
SES after rollout; widening one-click to all-non-essential is a one-line change if
the data says so.

**The transport refactor touches every mail path at once** → It is compiler-enforced
across the module, and `engage/mailpreview` renders every mail for visual diffing
before deploy. The integration-tagged tests in these packages must be run
(`go test -tags=integration ./internal/engage/...`), not just `go test ./...`.

**Two migrations in flight take the same number** → `main` has held three `0144`
files at once before. Take the next free number at the moment of writing and re-check
it against `origin/main` immediately before opening the PR.

## Migration Plan

1. Ship the migration first. Both columns are additive with a `DEFAULT true`, so the
   running binary ignores them and nothing changes behaviour.
2. Ship the code. Mails begin carrying headers and footer links; the public page and
   its routes go live. Existing accounts are unaffected until someone uses a link.
3. Reply to the complainant with his own preference link, and turn his mail off by
   hand if he prefers.
4. Watch SES complaint and bounce rates, and the Gmail Postmaster spam rate, for a
   week. The expected direction is down.

**Rollback:** revert the code. The two columns stay — they are inert to the previous
binary — so a rollback needs no down-migration and loses no recorded preference.

## Open Questions

None blocking. Two to revisit after a week of real use:

- Should one-click widen to all non-essential mail? Decide on the observed complaint
  rate, not in advance.
- Should the public page offer a "pause for 30 days" instead of only on/off? Only
  worth building if people use "unsubscribe from everything" and then come back.
