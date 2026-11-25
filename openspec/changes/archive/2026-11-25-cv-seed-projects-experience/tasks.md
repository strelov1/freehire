## 1. Persist project link on employments

- [x] 1.1 Add migration `experience_employments.link` (text, default empty) and sqlc queries/models so Employment create/update/list round-trip `link`
- [x] 1.2 Extend `Employment` sanitize/validate wire shape and FillBlanks so empty bank links fill from import and non-empty links are preserved
- [x] 1.3 Map `project.Link` in `EntriesFromResume`; cover import + FillBlanks with unit tests

## 2. Bank projections for CV seed

- [x] 2.1 Add a seed-facing projection that returns job-kind work history and project-kind rows (name, link, publishable highlights) separately; keep `WorkHistory`/`Professional` flattening for fit
- [x] 2.2 Unit-test: projects carry link and publishable bullets; `agent_inferred` omitted; jobs do not appear in the projects list

## 3. Seed mapping and seeder composition

- [x] 3.1 Map `Structured.Certifications` → `Document.Certifications` in `cv.Seed`; add/extend seed unit tests
- [x] 3.2 Update `bankedSeeder` to use the split projection: bank jobs for experience; bank projects when any project-kind row exists else structure projects; empty-bank (zero employments) falls back to structure experience
- [x] 3.3 Extend `seedable` to treat non-empty projects/certifications as seedable content where needed
- [x] 3.4 Handler/unit tests: empty-bank experience fallback; populated bank ignores structure experience; projects and certs on create-CV and reset/tailor seed paths

## 4. Verification

- [x] 4.1 `go test` for affected packages; `go vet -tags=integration ./...`
- [x] 4.2 Run integration tests that cover tailor bootstrap / reset seed if signatures or seed fixtures changed
