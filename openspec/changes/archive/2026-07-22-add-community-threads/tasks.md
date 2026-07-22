## 1. Database schema

- [x] 1.1 Add migration creating `community_personas` (`user_id` PK, `handle`
  unique, `created_at`) with the handle-unique index
- [x] 1.2 Add `threads` table (`id`, `subject_type`, `subject_id`, nullable
  `anchor_path`, `title`, `author_user_id`, `reply_count`, `status`,
  `created_at`) with an index on `(subject_type, subject_id, created_at DESC)`
- [x] 1.3 Add `thread_replies` table (`id`, `thread_id` FK, nullable
  `author_user_id`, `is_ai`, `body`, `created_at`) with an index on
  `(thread_id, created_at)`

## 2. Domain package `internal/community`

- [x] 2.1 Handle generator: deterministic adjective-noun-number token, with a
  unit test asserting format and variability
- [x] 2.2 Persona minting: get-or-create a handle for a `user_id`, retrying on
  unique-constraint conflict (unit-tested via a fake store)
- [x] 2.3 Subject resolution/validation: map `(subject_type, slug)` →
  `subject_id` for `company` and `job`; reject unsupported types and unknown
  slugs (unit-tested)

## 3. SQL queries (sqlc)

- [x] 3.1 Persona queries: `GetPersonaByUser`, `InsertPersona`
- [x] 3.2 Thread queries: `InsertThread`, `GetThreadByID`, `ListThreadsBySubject`
  (excludes closed), `IncrementReplyCount`, `CloseThread`
- [x] 3.3 Reply queries: `InsertReply`, `ListRepliesByThread`
- [x] 3.4 Rate-limit count queries: threads/replies authored by a user since a
  timestamp
- [x] 3.5 Regenerate sqlc (`make sqlc`) and confirm build

## 4. HTTP handlers

- [x] 4.1 Wire feature (public JSON DTOs that expose the persona handle and omit
  `author_user_id`), plus route registration behind `RequireAuth`
- [x] 4.2 `POST /api/v1/threads` — resolve subject slug, enforce rate limit, mint
  persona, insert thread + opening reply; 400/401/404/429 paths
- [x] 4.3 `GET /api/v1/threads?subject_type=&subject_slug=` — list a subject's
  open threads newest first
- [x] 4.4 `GET /api/v1/threads/:id` — read a thread with its replies oldest first
- [x] 4.5 `POST /api/v1/threads/:id/replies` — enforce rate limit, reject if
  thread missing/closed, insert reply, increment count
- [x] 4.6 Moderator close endpoint following the existing `jobs_moderation`
  pattern

## 5. Handler integration tests (`//go:build integration`)

- [x] 5.1 Create-thread happy paths for `company` and `job`; assert no
  `author_user_id` in any response
- [x] 5.2 Rejections: unknown slug (404), bad subject_type (400), unauthenticated
  (401), over-limit (429)
- [x] 5.3 Reply flow: post reply increments count; reply to closed/missing thread
  rejected
- [x] 5.4 Persona stability: two posts by the same user share one handle; two
  users get different handles

## 6. Frontend (`web/`)

- [x] 6.1 API client functions for list/create thread and list/create reply
- [x] 6.2 Discussion component (thread list + create form + thread view with flat
  replies), rendering persona handles
- [x] 6.3 Mount the component on `/companies/[slug]` and `/jobs/[slug]` with SSR
  load of the subject's threads

## 7. Verification

- [x] 7.1 `go build ./... && go vet ./...` and `go test ./...` green
- [x] 7.2 Run the integration suite for the new handlers
- [x] 7.3 Drive the flow end-to-end in the running app (create a thread on a
  company and on a job, reply, confirm anonymity in the payload)
