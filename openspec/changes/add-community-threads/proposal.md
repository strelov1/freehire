## Why

Job seekers have no place inside freehire to ask the questions that actually
decide whether they apply — "is this vacancy real?", "does this company answer
applications?", "what's the stack really like?". That insider signal lives
nowhere on the platform today. We want a lightweight, anonymous discussion
primitive that lets any signed-in user start a topic attached to a company or a
vacancy, without exposing who they are. The CV-roast and company-transparency
products the community will eventually grow into are not designed yet, so this
change ships only the flexible thread primitive with the polymorphic seam needed
to attach those future surfaces without reshaping the core.

## What Changes

- Add a generic **anonymous discussion thread** primitive: a signed-in user
  creates a topic (thread) attached to a subject, and any signed-in user replies.
- Attach subjects are **companies** and **existing vacancies** at launch; the
  subject is polymorphic (`subject_type` + `subject_id`), so future subjects
  (CV roast, etc.) plug in without changing the thread model.
- Threads carry an optional, nullable `anchor_path` **seam** (unused at MVP) so a
  future subject can anchor a thread to a sub-part of itself (e.g., a CV bullet).
- Every author is shown under a **stable anonymous persona handle**; the real
  `user_id` is stored for moderation and rate-limiting but is never serialized to
  clients.
- Replies are **flat** (chronological). Nested replies and votes/karma are
  explicit seams, not built here.
- Web: a discussion section rendered on `/companies/[slug]` and `/jobs/[slug]`.
- Basic anti-abuse: per-user rate limits on thread and reply creation, keyed on
  the private `user_id`; `status` field lets a moderator close a thread.

## Capabilities

### New Capabilities
- `community-threads`: anonymous, polymorphic discussion threads attached to a
  company or vacancy, with stable pseudonymous personas, flat replies, per-user
  rate limiting, and moderator close.

### Modified Capabilities
<!-- none: this is a self-contained new subsystem -->

## Impact

- **New DB tables**: `threads`, `thread_replies`, `community_personas`
  (new migration; applied via Postgres initdb on fresh volumes, run manually on
  prod per the project's migration convention).
- **New sqlc queries** under `internal/db/queries/` + generated code.
- **New handler surface** under `internal/handler/` (thread create/list, reply
  create/list) wired into the existing route tree; behind `RequireAuth`.
- **New domain package** (e.g., `internal/community/`) for persona minting and
  subject resolution/validation (slug → subject_id for company/job).
- **Frontend** (`web/`): discussion component consumed by the company and job
  detail routes.
- Reuses existing primitives: `RequireAuth` middleware, company/job slug
  resolution, the `reports` surface as the future report seam. No changes to
  existing auth, search, or ingest paths.
