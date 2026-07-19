## Context

freehire already has the pieces a referral flow needs: jobs/companies addressed
by slug, per-user interaction tables (`user_jobs`) as a modeling precedent, a
moderation precedent (`link_contributions`), two CV stores (the stored original
résumé at `users.resume_object_key` and the CV builder `cvs` table, including
tailored CVs with `job_id` + `agent_session_id`), S3 résumé storage, and a
channel-agnostic notification layer (`internal/notify` with `ChannelTelegram` /
`ChannelEmail`, SES + `telegramnotify`). This change composes those seams into a
moderated, anonymous employee-referral channel rather than inventing new
infrastructure.

## Goals / Non-Goals

**Goals:**
- Let members offer to refer into a company, gated by manual moderation of an
  uploaded proof CV.
- Surface an "ask for a referral" affordance on company/job pages only when an
  approved referrer exists.
- Let seekers request a referral at the company level, attaching either their
  original résumé or a tailored CV, with a contact and note.
- Notify approved referrers on their own channel while keeping them anonymous to
  the seeker; manage the two-sided lifecycle in the cabinet.
- Keep it free but spam-resistant.

**Non-Goals:**
- Email-domain verification / "verified employee" badge.
- Status pings to the seeker on referrer action.
- Credit/paywall gating of requests.
- Replying to a request from inside Telegram or email (cabinet only).
- Routing to a single specific referrer or letting seekers pick a referrer.

## Decisions

### Company-pool model, not per-referrer

A `referral_requests` row references the company by `company_slug` (companies'
PK is the slug — the same key `jobs.company_slug` carries), not a specific offer
or referrer. Every approved referrer of that company sees the request in their
inbox; whichever one acts records `acted_by`. Rationale: the seeker asked "in
this company" (confirmed during brainstorming), never sees who the referrers
are, and anonymity falls out for free. Alternative — request targets one chosen
offer — was rejected: it leaks referrer count/identity and forces the seeker to
pick among people they can't see.

### Two tables mirroring existing patterns

- `referral_offers(id, user_id, company_slug, proof_object_key, status,
  decided_by, decided_at, created_at)` with `UNIQUE (user_id, company_slug)` and
  a `status` check (`pending|approved|rejected`). Moderation mirrors
  `link_contributions`.
- `referral_requests(id, seeker_user_id, company_slug, job_id NULL, cv_kind,
  cv_id NULL, contact_telegram NULL, contact_email NULL, note, status, acted_by
  NULL, acted_at NULL, created_at)` with `status` check
  (`sent|contacted|declined`), a `cv_kind` check (`original` ⟹ `cv_id` NULL;
  `built` ⟹ `cv_id` NOT NULL), and a contact check (Telegram or email present).
  A partial unique index enforces one active request per
  `(seeker_user_id, company_slug)` where `status = 'sent'`. `job_id` and `cv_id`
  are `ON DELETE SET NULL` seams so a request outlives the vacancy or CV it
  referenced (same pattern as `cvs.job_id`).

### CV attachment reuses both stores; access is cabinet-only

`cv_kind = original` points at `users.resume_object_key`; `cv_kind = built`
carries `cv_id` into `cvs` (rendered to PDF via the existing Typst path). The
referrer views the CV only through an authorized cabinet endpoint that checks
they are an approved referrer of the request's company; the notification links
into the cabinet rather than shipping a signed file URL. Rationale: prevents CV
leakage via forwarded links and keeps the seeker's anonymity boundary crisp.

### Referrer eligibility is proof-CV + manual moderation

Becoming a referrer requires uploading a CV as proof of employment (reusing the
résumé S3 upload path) and a moderator approving it. This is the chosen trust
model — no automated domain check in this change, but `referral_offers` is the
seam a later "verified" badge would hang off.

### Direct notifiers, not the subscription engine

Referrer pings dispatch directly through the `notify` notifiers (SES email
always, `telegramnotify` if the referrer linked Telegram) — not the
saved-search subscription/digest machinery, which is match-driven and
ledger-based. A new `internal/referral` package owns offer/request lifecycle,
status vocabularies, and the notify fan-out. Email is the guaranteed channel
(every user has one); Telegram is additive when linked; a referrer with no
reachable channel still sees the request in-cabinet.

### Referral-availability flag on read shapes

The company read shape and `jobview` gain a boolean derived from "≥1 approved
offer for this company" so the frontend can conditionally render the block. This
is a read-time projection (or a cheap cached count), not new denormalized state
to keep in sync.

## Risks / Trade-offs

- **Referrer overload / abuse** → one active request per `(seeker, company)` +
  soft per-day cap; free but bounded. Credits seam left if it escalates.
- **Stale availability flag** → derive from a live `EXISTS`/count query rather
  than a maintained counter; revisit only if it shows up in job-list latency.
- **Fan-out cost when a company has many referrers** → acceptable at expected
  scale (0–2 referrers per company); notification is per-request, not per-view.
- **Proof CV is weak evidence** → accepted for MVP; moderation is the human
  backstop and the domain-verification seam exists for later.
- **No seeker status pings** → seekers may not notice a decline promptly; the
  cabinet status is the source of truth, and a status-ping channel is an
  explicit later seam.

## Migration Plan

- Add tables via a new migration (Postgres initdb for fresh volumes; run
  manually on prod before deploying code that reads them, per the repo's
  migration convention).
- Regenerate sqlc for the new queries.
- Ship backend (package, handlers, routes) and frontend behind the availability
  flag — the block simply never renders until an offer is approved, so there is
  no user-visible half-state during rollout.
- Rollback: feature is additive; disabling the routes and hiding the block
  leaves the tables inert.

## Open Questions

- Soft per-day request cap value (start conservative, e.g. 10/day) — tune after
  observing real usage.
- Whether the referrer inbox should show the seeker's display identity at all or
  strictly the contact they provided (leaning: only the provided contact + note,
  to keep the exchange on the seeker's chosen terms).
