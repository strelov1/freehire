## 1. Schema & sqlc

- [x] 1.1 Add migration: `user_jobs.vote smallint CHECK (vote IN (-1,1))` (nullable); `company_votes` table (PK `(user_id, company_slug)`, FKs, `vote` CHECK, timestamps); `upvote_count`/`downvote_count integer NOT NULL DEFAULT 0` on `jobs` and `companies`; partial index `user_jobs(job_id) WHERE vote IS NOT NULL` and index `company_votes(company_slug)`.
- [x] 1.2 Add sqlc queries: upsert/clear a job vote on `user_jobs` returning the caller's resulting vote; recompute a single job's `upvote_count`/`downvote_count` from `user_jobs`; read a caller's `vote` for a job. Regenerate `internal/db` (`make sqlc`).
- [x] 1.3 Add sqlc queries for company votes: upsert/clear in `company_votes`; recompute a single company's counters; read a caller's vote. Regenerate `internal/db`.

## 2. Domain — job votes (internal/userjob)

- [x] 2.1 Add a `Vote`/`ClearVote` path that runs the upsert (or clear) plus single-target counter recompute in one `pgx` transaction, returning `{upvote_count, downvote_count, my_vote}`. Enforce toggle (same direction clears) and flip (opposite replaces) semantics.
- [x] 2.2 Add a direction decoder that maps the request body (`"up"`/`"down"`) to `±1` and rejects anything else, before any DB touch.

## 3. Domain — company votes (internal/companyvote)

- [x] 3.1 Create `internal/companyvote` with `Vote`/`ClearVote` mirroring the job path (transactional upsert/clear + counter recompute), returning the same result shape.

## 4. HTTP endpoints

- [x] 4.1 Add handlers for `POST`/`DELETE /api/v1/jobs/:slug/vote` behind `keyAuth`; resolve slug → job id → 404 if unknown; 400 on invalid direction; return `{"data": {upvote_count, downvote_count, my_vote}}`.
- [x] 4.2 Add handlers for `POST`/`DELETE /api/v1/companies/:slug/vote` behind `keyAuth`, mirroring 4.1 against companies.
- [x] 4.3 Wire all four routes in `internal/handler/handler.go`.

## 5. Public wire shapes & counters on reads

- [x] 5.1 Add `UpvoteCount`/`DownvoteCount int32` (`upvote_count`/`downvote_count`) to `jobview.Job`, populated from the `jobs` row on list/detail/search (like `view_count`). Add the same two counters to the company response struct in `internal/handler/companies.go`.
- [x] 5.2 Add `MyVote int32` (`my_vote`) to `jobview.Job` and the company shape, documented as caller-scoped (only set on auth-aware detail reads, `0` otherwise).

## 6. OptionalAuth & my_vote on detail

- [x] 6.1 Add an `OptionalAuth(iss, keys)` middleware in `internal/auth` that attaches the caller id when a valid cookie/API key is present and passes through anonymously otherwise (never 401, tolerant of expired/invalid tokens on these reads).
- [x] 6.2 Apply `OptionalAuth` to `GET /jobs/:slug` and `GET /companies/:slug`; when a caller is present, populate `my_vote` for the job and company; leave it `0` for anonymous reads.

## 7. Frontend

- [x] 7.1 Build a shared `VoteControl` Svelte component (👍/👎 with counts, highlights `my_vote`, optimistic update from the endpoint response, sign-in prompt for anonymous visitors).
- [x] 7.1a Add a YouTube-style scale-bounce/pop animation on the tapped thumb as it becomes active; gate it behind `prefers-reduced-motion` (instant switch, no bounce, when reduced motion is requested); keep the animation purely presentational (never blocks the request or counter update).
- [x] 7.2 Add API client calls for the job and company vote/clear endpoints.
- [x] 7.3 Mount `VoteControl` on the job detail page and the company page, seeded from `upvote_count`/`downvote_count`/`my_vote`.

## 8. Verification

- [x] 8.1 `go build ./... && go vet ./... && go test ./...`; run the DB integration tests for the vote transaction paths (`-tags=integration ./internal/db/`).
- [x] 8.2 `web` build + lint; visually verify the vote control on a job page and a company page (signed-in highlight + toggle, anonymous sign-in prompt).
