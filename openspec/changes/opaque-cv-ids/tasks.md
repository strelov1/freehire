## 1. Schema

- [x] 1.1 Add migration `0045_cvs_uuid_id.sql`: generate a uuid for every CV, carry `referral_requests.cv_id` and `assistant_sessions.cv_id` across through the old key, swap the primary key, re-add both foreign keys with their original delete rules, and index the referencing columns. One transaction.
- [x] 1.2 Verify it against a real Postgres twice: once from scratch (initdb runs every migration in order) and once on a database seeded BEFORE the migration, asserting that each session still points at its own CV, that no row is orphaned, and that the cascade still fires. The second run is the one that matters — it is where a wrong statement order would silently break the links.

## 2. Query layer

- [x] 2.1 Change the CV queries in `internal/db/queries/cvs.sql` to take and return uuid ids; regenerate `internal/db` with `make sqlc` (add the `google/uuid` override for `cvs.id`, `referral_requests.cv_id` and `assistant_sessions.cv_id`).
- [x] 2.2 Update `internal/cv`: `Meta.ID`, `Record`, and every `Store` method that takes an id.
- [x] 2.3 Update the other packages that hold a CV id — `internal/referral` and the assistant's session binding.

## 3. HTTP surface

- [x] 3.1 Parse the `:id` route param as a uuid across the nine `/me/cvs/:id/*` endpoints; a malformed id is a 404, not a 400.
- [x] 3.2 Update the tailoring bootstrap's response ids and the assistant's CV tools (they close over the bound CV id).
- [x] 3.3 Update the handler tests and the CV integration tests for the new id type.

## 4. Web

- [x] 4.1 Type a CV id as `string` in `web/src/lib/cv.ts` and `api.ts`; update the `/my/cvs/[id]` route and the tailoring workspace's `?cv=` parameter.
- [x] 4.2 Run `svelte-check`, eslint, vitest and the production build.

## 5. Published clients (released after the backend)

- [ ] 5.1 `freehire-cli`: accept a string id in `cv get|edit|render|context`, update the request paths, and fix the skill's stale line about acting "via the session key" — the tailoring session is minted in the browser now, and the CLI acts with the user's own API key.
- [ ] 5.2 `freehire-mcp`: `cvId` becomes a string rather than `z.number().int()`, with the tool descriptions updated; bump the version and publish.
- [ ] 5.3 Update both READMEs where they show a numeric CV id.

## 6. Deploy

- [ ] 6.1 Apply the migration manually on production ahead of the API that reads it.
- [ ] 6.2 Release the backend + web, then the CLI, then the MCP publish. Between the first and the rest, `cv` commands on an un-updated client answer "not found" — expected, and the reason the order is fixed.
