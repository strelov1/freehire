## 1. Data model & DB layer

- [x] 1.1 Add migration creating `referral_offers` (with `UNIQUE (user_id, company_slug)`, status check `pending|approved|rejected`, `proof_object_key`, `decided_by`, `decided_at`)
- [x] 1.2 Add `referral_requests` to the same migration (status check `sent|contacted|declined`, `job_id`/`cv_id` `ON DELETE SET NULL`, `cv_kind` check `original|built`, contact + note columns, `acted_by`/`acted_at`) plus a partial unique index on `(seeker_user_id, company_slug) WHERE status = 'sent'`
- [x] 1.3 Write sqlc queries: create/get/list-by-user offers, list-pending offers (moderator), decide offer; create request, list requests by seeker, list `sent` requests for a referrer's companies, mark request contacted/declined, count today's requests for a seeker
- [x] 1.4 Add a query (or `EXISTS`) exposing "company has ≥1 approved offer" for the availability flag
- [x] 1.5 Regenerate sqlc (`make sqlc`) and confirm `go build ./...`

## 2. Referral domain package

- [x] 2.1 Create `internal/referral` with offer/request status vocabularies + validation (mirror `userjob/stages.go`)
- [x] 2.2 Implement offer lifecycle: submit (dedup on `(user, company)`, requires proof CV), moderator decide (approve/reject, record decider)
- [x] 2.3 Implement request lifecycle: create (validate CV choice + at least one contact + eligibility + active-dup + daily cap), mark contacted/declined (authorize acting referrer, record `acted_by`/`acted_at`)
- [x] 2.4 Implement referrer notification fan-out: for each approved referrer of the company, ping via SES email always + `telegramnotify` if linked; anonymous, links to cabinet
- [x] 2.5 Implement authorized CV access: resolve `original` → `resume_object_key`, `built` → `cvs`/Typst render; gate on caller being an approved referrer of the request's company

## 3. HTTP handlers & routes

- [x] 3.1 Seeker: `POST` create referral request (behind `RequireAuth`); validation errors → 400
- [x] 3.2 Seeker: `GET` list own referral requests (company, CV, status)
- [x] 3.3 Referrer: `POST` submit offer (proof-CV upload via résumé storage path) + `GET` list own offers
- [x] 3.4 Referrer: `GET` incoming requests for my companies + `POST` mark contacted/declined + authorized CV view endpoint
- [x] 3.5 Moderator: `GET` pending offers queue + `POST` approve/reject (behind moderator auth)
- [x] 3.6 Add the `referral_available` flag to the company read shape and `jobview` projection
- [x] 3.7 Wire routes in the handler registry; return `{"data": ...}` shapes per convention

## 4. Frontend

- [x] 4.1 Referral block on `jobview` and company page (rendered only when `referral_available`), with "ask for a referral" action
- [x] 4.2 Request modal: choose CV (original vs a tailored CV), enter contact (Telegram and/or email) + note, submit
- [x] 4.3 Seeker cabinet section: "My referral requests" with status
- [x] 4.4 Referrer cabinet: "My offers" (moderation status) + "Incoming requests" (contact, CV view, note, source job, mark contacted/declined)
- [x] 4.5 Moderator queue UI: pending offers with proof-CV view + approve/reject

## 5. Verification

- [x] 5.1 Integration tests for offer + request lifecycle (dedup, active-dup, daily cap, eligibility, authorization)
- [x] 5.2 Test the availability flag on company/job reads (approved vs pending/rejected/none)
- [x] 5.3 Test authorized CV access (approved referrer allowed; others denied)
- [x] 5.4 `go build ./... && go vet ./... && go test ./...` green; manual end-to-end walk of both cabinets
