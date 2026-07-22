## Context

freehire has no in-product discussion surface. The eventual community vision
(CV roast, company transparency, insider Q&A) is not designed yet — the
CV-tailoring product it would lean on is still immature. This change therefore
ships only the durable primitive underneath all of that: an anonymous discussion
thread that can attach to any subject. The project is MVP-stage, so the schema is
free to be reshaped later; the one thing we invest in now is a polymorphic seam
so future subjects plug in without a rewrite.

Reused primitives: `RequireAuth` cookie middleware; existing company/job slug
resolution; the sqlc + Postgres-initdb migration convention; the `{"data": ...,
"meta": {...}}` list / `{"data": ...}` single / `{"error": msg}` error response
shapes.

## Goals / Non-Goals

**Goals:**
- A generic thread primitive keyed on `(subject_type, subject_id)` that supports
  `company` and `job` subjects at launch.
- Anonymous authorship: a stable per-user persona handle is the only identity
  ever exposed; the real `user_id` stays server-side.
- Flat replies, per-user rate limiting, moderator close.
- A nullable `anchor_path` column reserved as a seam for future sub-part
  anchoring (e.g., a CV bullet) — present in schema, unused by code.
- A discussion section on the company and job detail pages.

**Non-Goals (explicit seams, not built):**
- CV-roast subjects, CV snapshots, AI seeding, market-coverage verdicts,
  revisions.
- Employer/experience verification and badges.
- Nested replies, votes/karma, reputation, credits-for-participation.
- Reporting UI (the existing `reports` surface is the future seam).

## Decisions

**D1 — Polymorphic subject via `(subject_type, subject_id)`, not per-subject
tables.** The thread stores a discriminator plus a bigint id. Adding a subject
later is a new enum value, zero thread-schema change. Alternative (a table per
subject, or a FK per subject) was rejected: it couples the primitive to today's
two subjects and multiplies migrations. The trade-off — no DB-level FK to the
subject — is handled by validating existence at write time (slug → id) and
tolerating that a hard-deleted subject can orphan threads (acceptable; subjects
here are companies/jobs which soft-close rather than delete).

**D2 — Anonymity by column discipline, mirroring the referral surface.** Every
row stores `author_user_id` (private); the wire DTOs simply never include it —
they carry only the persona handle. This is the same pattern the referral
handler uses to hide `user_id`/`proof_object_key`. The private id is what powers
rate limiting and moderation without a separate identity join.

**D3 — Persona minting.** A `community_personas(user_id → handle)` row is minted
lazily on a user's first authored content and reused forever, giving stable
pseudonymous continuity across a user's posts. Handle is a generated
adjective-noun-number token, unique-constrained. Alternative (throwaway handle
per post) was rejected: it kills continuity and makes moderation of a repeat
abuser invisible to the community while adding nothing for the MVP.

**D4 — Flat replies with seams.** `thread_replies` is chronological with a
denormalized `thread.reply_count`. `parent_reply_id` (nesting) and `vote_score`
are deliberately omitted now but are cheap to add later; the read API returns a
flat list, so adding nesting is additive.

**D5 — API by slug in, handle out.** Create/list operate on subject *slugs* (the
public identifier the frontend already has on `/companies/[slug]` and
`/jobs/[slug]`); the server resolves to ids. Responses use the standard envelope
and expose persona handles only.

**D6 — Domain package `internal/community/`.** Persona minting, handle
generation, and subject resolution/validation live in a small domain package so
the handler stays thin and the logic is unit-testable without a DB.

## Risks / Trade-offs

- [No FK to polymorphic subject → orphaned threads on subject delete] →
  Companies/jobs soft-close rather than hard-delete; acceptable for MVP, and a
  future cleanup can sweep orphans by `(subject_type, subject_id)`.
- [Anonymous abuse / spam] → Per-user rate limits keyed on the private id, plus
  moderator close. Deeper anti-abuse (shadow-ban, report queue) is a later seam.
- [De-anonymization is not a concern here] → subjects are companies/vacancies,
  not the user's own CV, so no PII is published; this risk lands only when a
  future CV subject is added, and is that change's problem.
- [Handle collisions] → unique constraint on `handle`; minting retries on
  conflict.

## Migration Plan

- One additive migration creating `threads`, `thread_replies`,
  `community_personas` with their indexes. Applied by Postgres initdb on fresh
  volumes; run manually on the prod volume BEFORE deploying code that reads the
  tables, per the project convention.
- No backfill, no changes to existing tables, no rollback data hazard (drop the
  three tables to revert).

## Resolved during implementation

- Rate-limit defaults: 10 threads / 24h and 30 replies / 1h per user (Config
  overridable; tune later).
- Moderator close is a dedicated endpoint `POST /threads/:id/close`, gated by the
  existing `auth.RequireRole(queries, "moderator")` middleware — the same
  `requireModerator` chain the submissions/reports routes use.
- A thread carries its own `body` column (the opening post); replies are responses
  only, so thread creation is a single insert (no transaction) and `reply_count`
  counts responses.
- `subject_ref` stores the subject's public slug, not a bigint id, because the two
  subjects have heterogeneous keys (companies' PK is its slug).
