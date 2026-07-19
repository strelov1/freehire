## Why

Job seekers convert far better when an insider refers them, but freehire has no
way to connect a seeker with someone who works at the hiring company. Members
who could refer have no outlet, and seekers have no signal that a referral is
even possible. This adds a moderated, anonymous referral channel on top of the
existing job/company pages, CV storage, and notification infrastructure.

## What Changes

- Members can **offer to refer** into a company: pick a company, upload a CV as
  proof of employment, and submit for moderation (`pending → approved / rejected`).
- Moderators review pending offers (view the proof CV) and approve or reject —
  reusing the `link_contributions` moderation pattern.
- Every `jobview` and company page for a company with ≥1 approved referrer shows
  a **"ask for a referral"** block; the block is hidden otherwise.
- Seekers **request a referral** at the company level: choose which CV to attach
  (their stored original résumé or a tailored CV from the builder), provide a
  contact (Telegram and/or email) and a short note. The optional source `job_id`
  is recorded as context.
- A request pings **all approved referrers** of that company through their own
  channel (email always; Telegram if linked). The referrer stays anonymous.
- Referrers manage incoming requests in their **cabinet**: view the seeker's
  contact, CV, note, and source job; reach out directly; then mark
  `contacted` / `declined`. The CV is viewable only inside the authorized
  cabinet, never via a bare link.
- Seekers see their requests in their cabinet with status
  (`sent → contacted / declined`).
- Anti-spam: requests are free, but limited to one active request per
  `(seeker, company)` plus a soft per-day cap.

Out of scope (seams left, not built): email-domain "verified" badge, status
pings to the seeker, credit/paywall gating, replying from inside Telegram/email.

## Capabilities

### New Capabilities
- `employee-referrals`: the referral offer + request lifecycle — becoming a
  moderated referrer for a company, the seeker's request flow (CV choice,
  contact, note), referrer notification across channels, cabinet management on
  both sides, and the anti-spam limits.

### Modified Capabilities
<!-- No existing capability's requirements change; the job/company page block and
     cabinet surfaces are additive and owned by the new capability. -->

## Impact

- **New tables**: `referral_offers`, `referral_requests` (migration).
- **DB layer**: new sqlc queries under `internal/db/queries/`.
- **New domain package** (`internal/referral/`): offer/request lifecycle, status
  vocabularies, referrer notification (reuses `internal/notify` notifiers + SES /
  `telegramnotify`).
- **Handlers/routes**: seeker request endpoint, referrer offer + inbox endpoints,
  moderator approval queue; company/job read shape gains a "referral available"
  flag.
- **S3**: proof-CV upload reuses the existing résumé storage path.
- **Frontend**: referral block on `jobview` + company page, plus referrer and
  seeker cabinet sections and the moderator queue.
