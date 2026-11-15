## 1. Schema and queries

- [x] 1.1 Migration 0270: `direct_applications.stage` (CHECK, default `new`), `stage_changed_at`; table `direct_application_cvs` (squawk clean)
- [x] 1.2 `UpsertDirectApplication` writes the CV copy in the same statement; queries `ListEmployerApplications`, `CountEmployerApplicationsByStage`, `GetEmployerApplicationCV`, `SetDirectApplicationStage`; `make sqlc`
- [x] 1.3 `GetApplicationStageForMirror`, `MirrorEmployerStage` (compare-and-set + `stage_set` event)

## 2. Candidate tracking

- [x] 2.1 `appevent.SourceEmployer`, trusted for day math; regenerate the web vocabulary
- [x] 2.2 `userjob.EmployerMay` with unit tests
- [x] 2.3 `jobtracking.EmployerStages` with unit and integration tests

## 3. The inbox

- [x] 3.1 `directapply.Record.CVFile`, passed by `Apply`
- [x] 3.2 `directapply.Inbox` (List, CV, Move) behind repository, mirror and notifier ports; unit tests for every refusal, the no-op move, follow-up failures
- [x] 3.3 Repository integration tests: scoping, unsent hidden, the retried CV kept, counts, cascade on account deletion
- [ ] 3.4 `ApplicationMailer.SendStatusUpdate` with a test and a mail-preview sample

## 4. HTTP

- [ ] 4.1 `typespec/employer.tsp`: list, CV, move; `make api-contract`; `gen:api-docs`
- [ ] 4.2 Handlers and wiring (501 without mail); handler integration test

## 5. Web

- [ ] 5.1 API methods and wire types
- [ ] 5.2 `EmployerApplications.svelte` as a routed tab of the vacancies section, with a spec
- [ ] 5.3 The apply-mode hint names the inbox
