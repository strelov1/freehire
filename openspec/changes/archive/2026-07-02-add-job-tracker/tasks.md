## 1. Schema + queries (saved_at, save/unsave, my-jobs listing)

- [x] 1.1 New migration: `user_jobs.saved_at timestamptz` (nullable)
- [x] 1.2 Queries: `SaveJob` (upsert sets `saved_at = now()`), `UnsaveJob`
      (clears `saved_at`, keeps the row), `ListUserJobs` (join `jobs`, filter
      param, `GREATEST(...)` ordering, limit/offset), `CountUserJobs`
      (per-filter counts via `COUNT(*) FILTER`)
- [x] 1.3 `make sqlc`; integration tests (tagged) for save/unsave idempotency,
      unsave preserves view/applied, listing filters + ordering + counts

## 2. API: save/unsave endpoints

- [x] 2.1 `POST /api/v1/jobs/:slug/save` and `DELETE /api/v1/jobs/:slug/save`
      behind `RequireAuth`, slug-resolved, returning `{"data": interaction}`;
      `saved_at` added to `interactionResponse`; handler tests (auth, unknown
      slug 404, idempotency, unsave-without-row)

## 3. API: my-jobs listing

- [x] 3.1 `GET /api/v1/me/jobs?filter=all|saved|applied` behind `RequireAuth`:
      jobview-shaped jobs + interaction fields, `meta` with
      `total/limit/offset` + `counts`; `400` on unknown filter; handler tests

## 4. Web: api client + types

- [x] 4.1 `saveJob`/`unsaveJob`/`listMyJobs` in `web/src/lib/api.ts`; `UserJob`
      type gains `saved_at`; `MyJob` item type

## 5. Web: My jobs page

- [x] 5.1 `/my/jobs` route + page with All / Saved / Applied tabs (counts as
      badges), reusing `JobRow`, signed-out state prompts sign-in; UserMenu
      link; `svelte-check` clean

## 6. Web: Save toggle on the job page

- [x] 6.1 Save/Unsave button on `JobView` driven by the interaction returned
      from the silent view recording; optimistic flip on the API response

## 7. Rollout

- [ ] 7.1 Recreate the dev volume (`docker compose down -v && make up`); note
      the manual `ALTER TABLE user_jobs ADD saved_at timestamptz` for prod
