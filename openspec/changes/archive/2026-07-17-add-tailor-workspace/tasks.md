## 1. Backend: persist the CV→session link

- [x] 1.1 Migration `migrations/0028_cvs_agent_session.sql` — `ALTER TABLE cvs ADD COLUMN agent_session_id text;`
- [x] 1.2 `cvs.sql`: return `agent_session_id` on `GetCVByID`; add `SetCVSession(id, user_id, session_id)`; extend `ListCVsByUser` to `job_id IS NOT NULL`, joining `jobs` for `public_slug`, returning `agent_session_id`. Regenerate `internal/db` (`make sqlc`)
- [x] 1.3 `internal/cv`: `Record.AgentSessionID`, `Meta`/list shape gains `JobSlug` + `AgentSessionID`; `Store.SetSession(ctx, id, userID, sessionID)`; unit tests (fake repo): set→get round-trip, owner-scoping

## 2. Backend: endpoints

- [x] 2.1 `PUT /me/cvs/:id/session {session_id}` handler (cookie or key + beta gate) → `Store.SetSession`; 404 on non-owner; wire route
- [x] 2.2 List/detail responses carry `agent_session_id` + `job_slug`; handler integration tests (set-session owner-scoping, list returns slug + session)

## 3. Backend: contracts + verify

- [x] 3.1 Update the CV wire types (list/detail) in `cmd/gen-contracts` if the TS shape changed; regenerate
- [x] 3.2 `go build ./... && go vet ./... && go test ./...`; integration tests green

## 4. Frontend: /tailor resume mode + editor tab

- [x] 4.1 `api`: `setCvSession(id, sessionId)`; extend CV list/detail types with `agent_session_id` + `job_slug`
- [x] 4.2 `/tailor/[slug]`: resume mode (`?cv=<id>` → GET CV + job + analysis, attach existing `agent_session_id`, no kickoff); bootstrap mode stores the session id via `setCvSession` after `createSession`
- [x] 4.3 `ArtifactPanel`: an **Edit** tab reusing `CvEditor` (load + save the tailored CV); plus an "Open PDF" action opening the CV in a new tab
- [x] 4.4 `<AssistantChat>` already opens a given `session` with no kickoff — verify resume attaches without re-sending

## 5. Frontend: /my/cvs rework

- [x] 5.1 `CvList`: drop the create button; list tailored CVs; each row → `/tailor/<job_slug>?cv=<id>`
- [x] 5.2 `/my/cvs/[id]` editor route → redirect into the workspace (`/tailor/<slug>?cv=<id>`)

## 6. Verify

- [x] 6.1 `svelte-check` (0 errors) + `vitest` (301 pass) + `npm run build` green
- [x] 6.2 Drive: match → tailor (fresh, stores session) → /my/cvs lists it → re-open resumes the SAME session (no kickoff) → Edit tab persists a field
