## 1. Candidate contacts storage and API

- [x] 1.1 Migration: persist candidate contacts (name, email, phone, location, links) per user; backfill from current/provisional structure where empty
- [x] 1.2 Store + sqlc: read/write contacts; résumé delete does not clear them
- [x] 1.3 HTTP: GET/PUT contacts (or fold into `/me/resume` read + dedicated PUT); validate/sanitize like CV header
- [x] 1.4 On successful `SetStructured`, fill-empty into candidate contacts; optional replace-from-CV endpoint or flag
- [x] 1.5 Unit/integration tests for fill-empty, no overwrite, delete résumé keeps contacts

## 2. Parse status and retry

- [x] 2.1 Persist last extract status for the current upload stamp (current/pending/failed + safe reason)
- [x] 2.2 Record failed/skipped outcomes from `extractStructuredResume` (PII/LLM unset, extract error)
- [x] 2.3 Expose status on `GET /me/resume`; add retry-parse endpoint that re-runs extract for stored CV bytes
- [x] 2.4 gen-contracts + handler tests

## 3. Seed / tailor composition

- [x] 3.1 `bankedSeeder` / seed composition: contacts from owned; body summary/skills/edu/langs/certs/projects from current structure or last blob when pending; bank experience/projects as today
- [x] 3.2 Stale-base refresh: keep header-only heal when pending; ensure full seed when current still works; never blank owned header via empty seed
- [x] 3.3 Tests: pending blob seeds summary/skills/projects onto new base; heal preserves summary; bank project still maps to `projects[]`

## 4. Web Profile and Experience

- [x] 4.1 Profile: editable contacts form; show parse status + retry; stop implying links are only from parse
- [x] 4.2 Experience: create/edit project employment (name, link, dates) via existing APIs
- [x] 4.3 Client API helpers + light UI tests if the suite already covers similar forms

## 5. Verification

- [x] 5.1 `go test` / `go vet -tags=integration ./...` for touched packages; manual Profile edit + tailor open with pending stamp
