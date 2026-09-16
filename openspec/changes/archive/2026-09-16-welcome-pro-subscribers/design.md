## Context

See proposal.md - Why. Two independent, small changes bundled because they came from one
request: make a first-time payment visibly acknowledged, automatically, going forward.

- `internal/identity/billing` (layer `identity`, 3) derives `users.pro_until_stripe` /
  `pro_until_revenuecat` / `pro_until_granted` (and the Ultra equivalents) whole, on every
  sync; `users.pro_until` / `users.ultra_until` are schema-derived from those three sources
  and are the one place `plan.TierOf` reads from. See
  `internal/identity/billing/AGENTS.md` ("derived whole, never adjusted").
- Mail already has two tones in this codebase: `internal/engage/emailnotify` (machine
  digest) and `internal/engage/onboarding` (personal, first-person, Reply-To a human inbox
  — see its package doc). The welcome email is the second kind.
- `internal/engage/discordlink` + `cmd/discord-sync` is the existing precedent for "a
  periodic `engage`-layer worker independently re-deriving the same paying-tier fact
  `billing-sync` already reconciles" — see `deploy/AGENTS.md`'s entry for `discord-sync`.
  It exists specifically because `identity` cannot import `engage`.
- `plan.Store.Tier(ctx, userID)` (`internal/ai/plan/store.go`) already resolves a tier from
  one indexed row (`GetPlanUntils`), with no network call — this is the read both the new
  worker and the `/auth/me` change should reuse, not a new resolution path.
- `ONBOARDING_REPLY_TO` already exists (read by both `cmd/onboarding` and `cmd/broadcast`)
  and is already documented as "the human inbox that answers these letters" — the same
  inbox this change's email should use, not a new variable.

## Goals / Non-Goals

**Goals:**
- Exactly one welcome email per account, ever, the first time it becomes a paying tier.
- The badge and the email both read the same tier resolution (`plan.TierOf`), so they can
  never disagree about whether an account is paying.
- No new network request added to ordinary page loads for the badge.

**Non-Goals:**
- Mobile header treatment for the badge (the profile icon it attaches to is desktop-only
  today — see `header-navigation`'s existing "Unified header layout" scenario; mobile's
  equivalent affordance is a future change, not this one).
- Any change to `internal/identity/billing`'s sync behaviour, event handling, or reconciler.
- A tier badge anywhere other than the header profile icon (e.g. not on `/my/profile`'s
  page body, not in the account nav rail) — the earlier "top of /my/profile" idea was
  superseded by "on the icon" per the approved design.
- Distinguishing Pro vs Ultra in the welcome email's *tone* (only its stated tier/date
  differ) — mirrors `discordlink`'s "every paying tier gets the same role."

## Decisions

### A new `internal/engage` package + `cmd/` worker, not a pass inside `cmd/billing-sync`

`billing` is layer `identity` (3); every mail package is layer `engage` (7); `identity`
cannot import `engage`. This is the exact shape `discord-sync` already solves for the same
reason (`deploy/AGENTS.md`). Package name: `internal/engage/prowelcome`; binary:
`cmd/pro-welcome-mail`. Reuses `emailnotify.Client` (SES) as its `Sender` and
`internal/application/mailtpl` for the layout, the same way `onboarding.Mailer` does —
copy that mailer's shape (`Sender` interface, `NewMailer(sender, from, replyTo, baseURL,
links)`) rather than inventing a new one.

### Idempotency: a nullable `users.pro_welcome_sent_at` column, set only after a successful send

Alternatives considered:
- **Diff previous vs. current tier inside the sync path.** Rejected: `billing.sync`
  deliberately never diffs (see AGENTS.md), and this worker has no "previous" to diff
  against between runs without inventing its own shadow state — a stamped column already
  answers "has this account been welcomed" directly.
- **A separate `pro_welcome_emails` ledger table.** Rejected as unnecessary weight for a
  single boolean-shaped fact about one account; the existing codebase's idiom for exactly
  this shape is a nullable timestamp on `users` (`hydrated_at`, `last_yield_at`,
  `company_info_wikipedia_checked_at`), not a side table.

The column is set **once, ever** — a cancel-then-resubscribe does not re-trigger the email
(see the `pro-welcome-email` spec's scenario). A returning subscriber is not a new one, and
re-welcoming them on every reactivation would look like a bug, not a feature.

Candidate query: accounts where `pro_welcome_sent_at IS NULL` AND (`pro_until` or
`ultra_until` reaches beyond now) — the same shape `discordlink`'s `ListChronicBoards`-style
pages already use elsewhere in this codebase (page ordered by the nullable stamp, bounded
by a `MAX_PER_RUN`-style env var).

### Worker cadence: its own timer, every 10 minutes, independent of `billing-sync`/`discord-sync`

Chosen (over piggy-backing on `discord-sync`'s hourly timer) because a purchase
confirmation reads as broken if it arrives an hour late in a way a Discord role does not.
Uses the ordinary `internal/platform/worker` bootstrap (`Main`/`Bootstrap`, `ExitCode`) like
every other cron worker; a per-account send failure is counted and stepped over (its column
stays unset, so the next run retries it), matching every other worker's convention — only a
failure to read the candidate page at all exits non-zero.

### `tier` on `GET /api/v1/auth/me`, not a second endpoint

The header renders on every page for every signed-in visitor, and `currentUser()` already
reads `page.data.user`, populated once per navigation by the root layout's call to
`/api/v1/auth/me`. Calling `GET /api/v1/me/plan` from the header as well would add a second
request to every page load for a value already knowable from data the app already fetches.

`toUserResponse` is a free function called from ~8 handler methods (register, login,
oauth callback, timezone/language updates, password recovery, `Me`). It becomes a method on
`authHandlers` (`h.toUserResponse(ctx, u)`) so it can reach `h.plan.Tier(ctx, u.ID)` — one
indexed row read, no network call, the same cost `plan.Store.Tier` already carries at every
metered-feature check today. A failed tier lookup degrades to `free` and is logged rather
than failing the whole response: nothing about signing in, registering, or reading one's
own account should ever depend on a badge rendering correctly.

## Risks / Trade-offs

- **[Risk]** A person paying and then immediately refunding within one 10-minute window
  still gets welcomed once. → Accepted: the same is true of Discord's role grant and of
  Stripe's own receipt; "you paid" was momentarily true, and treating it as noise would
  require distinguishing a genuine cancellation from a refund inside the welcome worker,
  which duplicates `billing`'s own "derived whole, re-read" logic for no real benefit.
- **[Risk]** Adding a DB read to `toUserResponse`'s ~8 call sites is a per-response cost
  that did not exist before. → Mitigated: it is one indexed row by primary key, the same
  read `plan.Store.Tier` already performs on every metered AI action; no measurable load
  concern at current or near-term scale.
- **[Risk]** The new systemd timer is not installed anywhere by this change (per
  `deploy/AGENTS.md`, "nothing here deploys itself") — until an operator copies the unit to
  host2 and reloads systemd, the worker code ships inert. → Call this out explicitly in the
  PR/rollout notes; not a code defect, a deploy step.
- **[Risk]** `Runner.Run` sends the letter, THEN claims (`SetProWelcomeSent`) — the reverse
  of this codebase's usual claim-first pattern (see `mentorship-remind`: "the claim comes
  first because a worker that sends and then records sends twice whenever the second step
  fails"). Two overlapping runs of this worker (nothing holds a lock beyond systemd's
  `Type=oneshot`, which only protects the TIMER path — a hand-run invocation has no lock at
  all) could both read the same candidate before either claims it, and both send. → Accepted
  deliberately, not an oversight: claiming first and then failing to send would DROP the
  welcome silently forever (the row would read "already welcomed" with no mail ever sent),
  which is worse than the rare duplicate a race produces. The failure mode this ordering
  actually prevents — a failed send permanently losing an account's welcome — is the whole
  reason `prowelcome.Runner` differs from `onboarding.Runner`'s claim-then-send shape in the
  first place (see the design decision above). A duplicate welcome email is a minor
  annoyance; a subscriber never welcomed at all is the bug this change exists to fix.

## Migration Plan

1. New migration: `ALTER TABLE users ADD COLUMN pro_welcome_sent_at timestamptz;` (nullable,
   no backfill — a pre-existing subscriber simply never receives the automated welcome;
   the one subscriber that exists today was already welcomed by hand, outside this system).
2. Ship `internal/ai/plan`-backed `tier` on `/auth/me` and the header badge together — the
   badge has nothing to read without the field.
3. Ship `internal/engage/prowelcome` + `cmd/pro-welcome-mail`; add its systemd timer file
   under `deploy/systemd/` (not installed on host2 by this change).
4. No rollback hazard beyond the ordinary "stop the timer" — the column is additive and
   nothing reads it except this worker.
