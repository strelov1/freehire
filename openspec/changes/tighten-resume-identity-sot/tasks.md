## 1. Architecture note

- [x] 1.1 Add `internal/resume/AGENTS.md` with the three-layer identity table (owned contacts, current structure, provisional contacts) and which reader may use each layer for Profile, seed, heal, and reset
- [x] 1.2 Point `StructureForSeed`, `ProfileReadForUser`, `GetResume`, `bankedSeeder.Structured`, `reseedBaseIfStaleVsUpload`, and `healRecordHeader` at that note so a later edit has one place to update
- [x] 1.3 Document in `internal/experience/AGENTS.md` that project JSON uses `name` + `link` (storage field stays `Company`) and inbound still accepts legacy `company`

## 2. Align readers with the table

- [x] 2.1 Walk `internal/resume/resume.go`, `contacts.go`, `internal/handler/cv_seed.go`, `cv_seed_apply.go`, `cv_header_heal.go`, `cv_reset.go`, and `resume.go` against the table in design.md; change code only where a reader still contradicts it
- [x] 2.2 Keep the two header merges separate: seed-first `mergeSeedHeader` for reset/full reseed, keep-first `fillEmptyHeaderFields` for GET heal and pending stale-base refresh
- [x] 2.3 Confirm `GET /me/resume` omits superseded semantic sections while pending and prefers owned contacts as a block, matching `StructureForSeed`

## 3. Lock the table with tests

- [x] 3.1 Add a table-driven test over one fixture (current stamp, pending superseded blob, owned overlay, résumé deleted) that asserts `Structured`, `ProfileReadForUser`, `ProvisionalContacts`, `StructureForSeed`, and `bankedSeeder.Structured` agree with the identity table
- [x] 3.2 Keep or extend the existing stale-seed contacts-only case so a superseded summary cannot appear on reset
- [x] 3.3 Add or extend a test that owned contacts survive résumé delete and overlay both current and provisional extract contacts

## 4. GET heal justification and coverage

- [x] 4.1 Comment on `healRecordHeader` why owner GET may persist (blank-header repair, keep-first, body untouched, idempotent)
- [x] 4.2 Add a test that a second owner GET after a successful heal does not create another revision
- [x] 4.3 Add a test that `GET /me/cvs` (list) and PDF render do not persist a header heal
- [x] 4.4 Keep `TestGetCV_HealsEmptyTailoredHeaderFromProvisional` and `TestGetCV_PartialHeaderKeepsNameFillsEmail` green

## 5. Project wire contract

- [x] 5.1 Confirm `employment_json.go` emits `name` (not `company`) for `kind=project` and `company` (not `name`) for jobs
- [x] 5.2 Confirm unmarshal accepts project `name` and legacy project `company`; add a case if either is missing
- [x] 5.3 Check generated TS / web types do not document `company` as the project place label

## 6. Verify

- [x] 6.1 `go test ./internal/resume/ ./internal/experience/ ./internal/handler/ ./internal/cv/`
- [x] 6.2 `go vet -tags=integration ./internal/resume/ ./internal/experience/ ./internal/handler/ ./internal/cv/`
