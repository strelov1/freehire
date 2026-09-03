## 1. Fix the pagination cursor

- [x] 1.1 Expose the page size from `community.Service` (`PageSize() int32`) so
  a caller can tell a full page from a last page
- [x] 1.2 `ListThreads`: emit `next_cursor` only when the page returned is full
- [x] 1.3 `GetThread`: same rule for the reply cursor
- [x] 1.4 Integration tests: a partial page omits the cursor; a full page
  carries one; a thread's short reply list omits it

## 2. Database

- [x] 2.1 Migration `0126_threads_open_created_idx.sql`: partial index on
  `threads (created_at DESC, id DESC) WHERE status = 'open'`, with a comment
  saying why the existing subject-prefixed index cannot serve the feed
- [x] 2.2 `pnpm check:sql` passes on the new file
- [x] 2.3 Migration `0127_repair_threads_open_created_idx.sql`: repairs any
  database that recorded 0126 while the index it built was invalid. Needed
  because the host's autodeploy recorded 0126 between its failed release and
  the fix, so the edited 0126 can never run there

## 3. SQL queries (sqlc)

- [x] 3.1 `ListRecentOpenThreadsFirst`: open threads across all subjects,
  newest first, LEFT JOIN `community_personas` for the handle and LEFT JOIN
  `jobs`/`companies` for the subject's display name
- [x] 3.2 `ListRecentOpenThreadsAfter`: keyset continuation on
  `(created_at, id)`
- [x] 3.3 `make sqlc` and confirm the build

## 4. Domain — `internal/engage/community`

- [x] 4.1 `ThreadWithSubject` read model: a `Thread` plus `SubjectTitle` and
  `SubjectCompany`, with a comment on why the fields are not on `Thread`
- [x] 4.2 Repository port + implementation: `ListRecentOpenThreads(ctx, cur, limit)`
- [x] 4.3 `Service.ListRecentThreads(ctx, cur)` delegating with the configured
  page size
- [x] 4.4 Unit test over a fake repository: empty catalogue, mixed subject
  types, a row whose subject did not resolve

## 5. API — `internal/api/handler`

- [x] 5.1 `recentThreadResponse`: `threadResponse` plus `subject_title` and
  `subject_company`
- [x] 5.2 `ListRecentThreads` handler, standard envelope, corrected cursor rule
- [x] 5.3 Route `GET /api/v1/threads/recent`, registered before `/threads/:id`
- [x] 5.4 Integration tests: mixed subjects, an unresolvable subject returns an
  empty title rather than being dropped, closed threads excluded, no session
  required, paging across two pages

## 6. Web — `web/`

- [x] 6.1 `CommunityFeedThread` type and `api.listRecentThreads(cursor)`
- [x] 6.2 `DiscussionFeed.svelte`: rows naming the subject, falling back to the
  slug when the name is empty, linking to the thread's page under its subject
- [x] 6.3 `/discussions` route: `+page.server.ts` (SSR first page) and
  `+page.svelte`, with page title and meta description
- [x] 6.4 Footer link under RESOURCES
- [x] 6.5 `/discussions` added to `sitemap-pages.xml`
- [x] 6.6 Test for the subject-name fallback. Extracted to a pure
  `feedSubjectLine` (`web/src/lib/feedSubject.ts`) because this test environment
  has no Svelte runtime for runes — the same reason `paginated.svelte.test.ts`
  gives for testing its core rather than the component

## 7. Verification

- [x] 7.1 `gofmt -l .` clean, `go vet ./...`, `go test ./...`
- [x] 7.2 `go vet -tags=integration ./...`, then the tagged suite for the
  packages touched
- [x] 7.3 `pnpm -C web check` / lint, `pnpm check:links`
- [x] 7.4 Verify the feed and the fixed "Load more" in a browser against prod
  after deploy. Live 2026-09-03: /discussions serves its three threads with
  resolved subject names and logos, and the single-thread subject page no
  longer draws "Load more". Took three releases — see 0126/0127 for the
  CONCURRENTLY lock timeout and the invalid index it left behind.

## 8. Subject header (added after the feed shipped, on request)

- [x] 8.1 `discussionSubject.ts`: the pure mapping from a `Job`/`Company` to
  what the header prints — title, logo key, employer label, closed state
- [x] 8.2 `server/discussionSubject.ts`: fetch it, never throw, and tell a gone
  subject apart from an unreachable one; follow a merged company slug by hand
- [x] 8.3 `SubjectHeader.svelte`: one link when the subject resolved, a plain
  box when it did not
- [x] 8.4 Wire into all four subject-scoped discussion pages, replacing the
  breadcrumbs (the company ones printed a raw slug)
- [x] 8.5 `loadDiscussionIndex` shared by both index routes, as `loadThread`
  already was by both thread routes
- [x] 8.6 Unit tests for the mapping, including the employerless vacancy

## 9. Closing out

- [ ] 9.1 Archive the change (`opsx:archive`) and sync the delta into
  `openspec/specs/community-threads/spec.md`. Deliberately last: the header in
  section 8 arrived after the rest had shipped, so archiving earlier would have
  closed a change that was still growing
