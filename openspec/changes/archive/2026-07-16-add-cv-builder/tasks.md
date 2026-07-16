## 1. Schema & sanitization

- [x] 1.1 Define `cv.Document` wire shape in `internal/cv/cv.go` (header, summary, experience[], education[], skills[], languages[], projects[], certifications[]), extending the `resumeextract.Structured` fields
- [x] 1.2 Implement `cv.Sanitize(Document) Document` — bound every string, cap every array, drop out-of-range values; unit-test oversized/malicious input is bounded (persist + prompt-injection guard)
- [x] 1.3 Implement `cv.EmptyDocument()` skeleton and assert it passes `Sanitize` unchanged

## 2. Seeding from the stored résumé

- [x] 2.1 Implement `cv.Seed(resumeextract.Structured) Document` mapping contacts/summary/experience/education/languages/links into `Document`
- [x] 2.2 Unit-test seeding (populated structure → filled Document; empty structure → valid skeleton)

## 3. Persistence

- [x] 3.1 Write migration `0024_cvs.sql`: `cvs` table (`id bigint IDENTITY PK` — matches the codebase, no uuid; `user_id bigint FK ON DELETE CASCADE`, `title text`, `template_id text DEFAULT 'classic-ats'`, `data jsonb`, `job_id bigint NULL FK jobs ON DELETE SET NULL`, `created_at`, `updated_at`) + `cvs_user_id_updated_at_idx (user_id, updated_at DESC)`
- [x] 3.2 Add `internal/db/queries/cvs.sql` (create, list-by-user, get-by-id-and-user, update, delete-by-id-and-user) and regenerate sqlc
- [x] 3.3 Implement `internal/cv/store.go` repo over sqlc (CRUD, owner-scoped reads); integration build-tag test for round-trip + owner isolation

## 4. Renderer & template

- [x] 4.1 Define `Renderer` interface + `Template` type + template registry (`template.go`, `go:embed templates/*.typ`), default `classic-ats`, reject unknown ids
- [x] 4.2 Author `templates/classic-ats.typ` (single-column, standard headings, reads `json("data.json")`) using Typst's embedded Libertinus Serif — no bundled fonts
- [x] 4.3 Implement `TypstRenderer` (`exec.CommandContext`, temp `--root` dir, write `data.json`, `--ignore-system-fonts` for local==prod reproducibility, ctx timeout; never interpolate user data into argv)
- [x] 4.4 ATS-regression test (integration build-tag, requires typst): render a fixture Document → extract text via `ledongthuc/pdf` → assert name and skills present and selectable

## 5. Config & feature-gating

- [x] 5.1 Add optional `TYPST_BIN` to `internal/config`; construct the renderer nil-safe (absent binary → renderer disabled), with a unit test
- [x] 5.2 Wire the renderer + `cv.Store` into `cmd/server/main.go` following the blobstore/meili/llm nil-safe pattern

## 6. HTTP handlers

- [x] 6.1 `internal/handler/cv.go`: `GET/POST /me/cvs`, `GET/PUT/DELETE /me/cvs/:id` (`RequireAuth` cookie-only + `RequireModeratorOrBeta(a.queries, a.queries)` beta gate → 403 for non-beta), Sanitize on write, owner scoping → 404 on foreign id; validate `template_id` against the registry
- [x] 6.2 `POST /me/cvs` seeds from `resume_structured` when present (empty skeleton otherwise); handler test covers both
- [x] 6.3 `GET /me/cvs/:id/pdf` streams `application/pdf`; returns `501` when renderer disabled; handler tests (integration build-tag for DB) for CRUD, 404 isolation, and 501 gating

## 7. Contracts & frontend

- [x] 7.1 Emit `cv.Document` to `web/src/lib/generated/contracts.ts` via `cmd/gen-contracts`; regenerate and commit
- [x] 7.2 `web/src/routes/my/cvs/+page`: list CVs (title, template, updated) with create/delete/download; add API client calls; gate the nav entry + page on `user.beta_tester` (moderators too)
- [x] 7.3 `web/src/routes/my/cvs/[id]/+page`: section form editor bound to `Document`, Save (PUT) and Download PDF; vitest for pure form-serialization logic, svelte-check clean

## 8. Deploy & verification

- [x] 8.1 Update the Docker build: multi-stage fetch of the pinned Typst binary (no fonts — Libertinus Serif is embedded in the binary), `COPY` into the distroless image; document manual prod migration + `TYPST_BIN` in deploy notes
- [x] 8.2 End-to-end verification: create (seeded) CV → edit → download PDF → confirm selectable text via extraction; `go build ./... && go vet ./... && go test ./...` green
