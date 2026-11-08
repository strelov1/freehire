# Base CV editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Base CVs are visible on `/my/cvs`, editable in their own editor, duplicable, and choosable when tailoring; a tailored CV remembers its base.

**Architecture:** Two new `cvs` columns (`base_cv_id`, `hand_edited`) plus a rewritten "default base" query give the whole backend its single rule. `internal/candidate/cv` (store) owns the rule, `internal/api/handler` adds a bases list and a duplicate route and threads an optional `base_cv_id` through tailoring. The web gets a base-cards block, a vacancy-less editor at `/my/cvs/[id]`, and a picker shown only for 2+ bases.

**Tech Stack:** Go + Fiber, sqlc, Postgres, SvelteKit (Svelte 5), vitest.

**Spec:** `docs/superpowers/specs/2026-11-03-base-cv-editor-design.md`

## Global Constraints

- English only: code, comments, commits, PR text.
- No new Go package (`blocks.go` untouched). SQL only in `internal/platform/db/queries/cvs.sql` (+ `make sqlc`).
- New migration is `0231_*` (after `0230`); must pass `pnpm check:sql` (squawk): FK added `NOT VALID` then `VALIDATE`, index non-blocking per squawk guidance.
- `GET /me/cvs` keeps its shape. `POST /me/cvs/tailor` works without `base_cv_id`.
- One base (or none) ⇒ no picker, no extra step.
- No AI/model call in the base editor.
- Every user-visible string goes through the neighbouring `messages.ts` with every locale it carries. `knip` gates unused exports.
- Before each Go commit: `gofmt -l .` empty, `go vet ./...`, `go test ./...`. Before push: `go vet -tags=integration ./...` + integration tests for `internal/api/handler` and `internal/candidate/cv`.

## Decisions (the builder's calls)

- Hand-edit marker: column `cvs.hand_edited boolean NOT NULL DEFAULT false`, set inside the `cvedit` commit transaction when `actor=candidate` and `origin IN ('editor','template')` and the document changed. A column, not a revision scan: the revision log is trimmed per CV, and a Duplicate of an edited base has no history yet must stay protected (copy the flag).
- Default base (one query, `GetBaseCVByUser`): the base referenced by the user's newest tailored CV with non-NULL `base_cv_id`; else the oldest-created base.
- Bases list: `GET /api/v1/me/cvs/bases` (key or cookie) → `[{id,title,template_id,created_at,updated_at,is_default,hand_edited}]`. Literal route registered before `:id`.
- Duplicate: `POST /api/v1/me/cvs/:id/duplicate` (cookie) → 201 full record; 409 for a tailored CV; 404 foreign/missing.
- Explicit refresh of the open base: `POST /me/cvs/:id/reseed` on a **base** id reseeds that base only (today it 409s for a base). On a tailored id it refreshes the copy only (no base write).
- Tailor base travels as `?base=<id>` on `/tailor/[slug]`.
- Editor autosaves (same 800 ms debounce through `PUT /me/cvs/:id` as the workspace).

## Review Focus

- Deleting a base that tailored copies came from: copies keep working, `base_cv_id` becomes NULL, ATS delta falls back to default. (Task 2, 4)
- Editing or duplicating a non-default base must not change the default. (Task 1, 2)
- `base_cv_id` pointing at a tailored or foreign CV → 409 / 404, never used. (Task 4)
- Duplicate of a hand-edited base stays hand-edited; stale-vs-upload refresh skips it. (Task 2, 4)
- A user with zero bases: bases list is `[]`, tailoring still seeds one, editor page shows empty state. (Task 4, 8)
- Existing tailored copy reopened with a different `?base=`: returns the existing copy unchanged (one copy per vacancy). (Task 4)

---

### Task 1: Migration and queries

**Files:**

- Create: `migrations/0231_cvs_base_cv_id_hand_edited.sql`
- Modify: `internal/platform/db/queries/cvs.sql`; regenerate `internal/platform/db/*` with `make sqlc`

**Interfaces:**

- Produces (sqlc): `GetBaseCVByUser` (new rule; rows now also return `hand_edited`), `ListBaseCVsByUser(userID) → rows {id,title,template_id,hand_edited,created_at,updated_at,is_default}`, `GetBaseCVByID(id,userID)` (non-tailored only, with data+hand_edited), `CreateTailoredCV` gains `base_cv_id` param (nullable uuid), `GetCVByID` returns `base_cv_id`, `hand_edited`; `DuplicateBaseCV(id,userID,title) → CreateCVRow-like` copying data/template/hand_edited; `MarkCVHandEdited(id,userID) :execrows`.
- [ ] **Step 1:** Write migration: `ADD COLUMN base_cv_id uuid`, `ADD COLUMN hand_edited boolean NOT NULL DEFAULT false`; FK `cvs_base_cv_id_fkey ... REFERENCES cvs(id) ON DELETE SET NULL NOT VALID` then `VALIDATE CONSTRAINT`; partial index on `(base_cv_id) WHERE base_cv_id IS NOT NULL` (needed by SET NULL and the default query); backfill `UPDATE cvs t SET base_cv_id = (newest-edited non-tailored CV of t.user_id) WHERE t.is_tailored AND base_cv_id IS NULL AND such a base exists`. Header comment states why the backfill matters (spec: "The default base").
- [ ] **Step 2:** Rewrite `GetBaseCVByUser` ordering and add the new queries in `cvs.sql` (default flag in the list is computed with the same expression as the single-row query; keep the expression in one comment-referenced shape).
- [ ] **Step 3:** Run `make sqlc`; fix compile breaks in `internal/candidate/cv/store.go` (repository adapters) minimally; `go build ./...`.
- [ ] **Step 4:** Run `pnpm check:sql`; expected: no findings on the new file.
- [ ] **Step 5:** Commit `Add cvs.base_cv_id and hand_edited, rewrite the default-base query`.

### Task 2: cv store (rule, list, duplicate, tailor-from-base)

**Files:**

- Modify: `internal/candidate/cv/store.go`, `internal/candidate/cv/AGENTS.md`
- Test: `internal/candidate/cv/store_test.go` / `store_tailor_test.go` (integration-tagged, testcontainers, same style as neighbours)

**Interfaces:**

- Produces: `Record` gains `BaseCVID uuid.UUID` (zero when NULL) and `HandEdited bool`; `BaseItem{Meta; IsDefault, HandEdited bool}`; `(*Store).Bases(ctx, userID) ([]BaseItem, error)`; `(*Store).BaseByID(ctx, id uuid.UUID, userID int64) (Record, error)` (ErrNotFound if missing/foreign, `ErrNotBase` if tailored); `(*Store).BaseFor(ctx, userID, tailored Record) (Record, bool, error)` (own base if set and live, else default); `(*Store).Duplicate(ctx, id uuid.UUID, userID int64) (Meta, error)` (title `"<title> (copy)"`, `ErrNotBase` for tailored); `Tailor(..., baseID uuid.UUID, ...)` — `uuid.Nil` ⇒ default; stores `base_cv_id`; `var ErrNotBase`.
- [ ] **Step 1:** Failing tests: (a) two bases, edit the older one → default unchanged (oldest-created); (b) tailor from base B → default becomes B; (c) delete B → tailored copy survives with `BaseCVID` zero and `BaseFor` falls back to default; (d) `Duplicate` copies document/template/`HandEdited`, title `"X (copy)"`, does not change default; (e) `Duplicate` of tailored → `ErrNotBase`; foreign → `ErrNotFound`; (f) `Tailor` with a tailored or foreign `baseID` → `ErrNotBase` / `ErrNotFound`; (g) `Bases` flags exactly one default, `[]` for none.
- [ ] **Step 2:** Run `go test -tags=integration ./internal/candidate/cv/ -run 'Base|Duplicate|Tailor'`; expected FAIL (undefined).
- [ ] **Step 3:** Implement the interfaces above on `Repository`/`Store`/`queriesRepository`.
- [ ] **Step 4:** Same command; expected PASS. `go test ./internal/candidate/cv/` green.
- [ ] **Step 5:** AGENTS.md: document the default-base rule, `hand_edited`, `base_cv_id`. Commit.

### Task 3: cvedit sets `hand_edited`

**Files:**

- Modify: `internal/candidate/cvedit/` (commit path + its repository tx), `internal/platform/db` use of `MarkCVHandEdited`
- Test: `internal/candidate/cvedit/*_test.go` (integration if that is where commit tests live)

**Interfaces:** Consumes `MarkCVHandEdited` (Task 1). Produces: a commit with `ActorCandidate` and origin `editor`/`template` that records a revision marks the CV; agent/system/`import` commits do not.

- [ ] **Step 1:** Failing tests: candidate+editor edit ⇒ `Get().HandEdited`; candidate+template ⇒ marked; candidate+import (header heal/reseed) ⇒ not; agent ⇒ not; no-op save (no change) ⇒ not.
- [ ] **Step 2:** Run package tests; expected FAIL.
- [ ] **Step 3:** Call `MarkCVHandEdited` inside the same transaction as the revision insert.
- [ ] **Step 4:** Tests PASS. Commit.

### Task 4: Handlers

**Files:**

- Modify: `internal/api/handler/cv.go` (routes, `ListBaseCVs`, `DuplicateCV`), `cv_tailor.go`, `cv_reseed.go`, `cv_ats_delta.go`, `auto_apply_tailor.go`/`auto_apply_enqueue.go` (callers of the changed helper)
- Test: new `internal/api/handler/cv_bases_integration_test.go`; update existing reseed tests for the changed semantics

**Interfaces:** Consumes Task 2. Produces routes: `GET /me/cvs/bases` (`mw.key`), `POST /me/cvs/:id/duplicate` (`mw.cookie`), `tailorCVRequest.BaseCVID string json:"base_cv_id,omitempty"`.

- [ ] **Step 1:** Failing integration tests (use `newCVHandlers`, mirror `cv_integration_test.go`): bases list shape + `is_default`; duplicate 201/409/404; tailor with valid `base_cv_id` stores it and response `base_cv_id` equals it; tailor with tailored id ⇒ 409, foreign ⇒ 404, malformed ⇒ 400; tailor without field ⇒ default; reopening existing vacancy with another base returns the existing copy; `GetCVATSDelta` and `StartTailorSession` report the copy's own base (create two bases, tailor from the non-default, assert `base_cv_id`); stale-vs-upload refresh skips a `HandEdited` base and still refreshes an unedited one; `ReseedCV` on a tailored CV leaves every base untouched; `ReseedCV` on a base id replaces that base only (even hand-edited) and `ReseedBaseCV` targets the default.
- [ ] **Step 2:** `go test -tags=integration ./internal/api/handler/ -run 'Base|Reseed|Tailor|ATSDelta'`; expected FAIL.
- [ ] **Step 3:** Implement. `reseedBaseIfStaleVsUpload` takes the base it is about to use (`uuid.Nil` ⇒ default) and returns early when `base.HandEdited`. `ReseedCV` split: tailored ⇒ copy only; base ⇒ that base only (`reseedBase(ctx,userID,record,seeded)` extracted from `reseedBaseFromSeed`). Route `/me/cvs/bases` registered before `/me/cvs/:id`.
- [ ] **Step 4:** Same command PASS; `go test ./...` and `go vet -tags=integration ./...` green.
- [ ] **Step 5:** Commit.

### Task 5: API docs

**Files:** `web/src/lib/docs/api-spec.ts` and the generated internal OpenAPI JSON (locate with `grep -rn "me/cvs/tailor" web/src`), contracts if generated types cover these routes.

- [ ] **Step 1:** Add `base_cv_id` request field, the bases list route and the duplicate route; regenerate whatever generator the repo uses. Verify the spec test (if any) passes: `pnpm vitest run` on the touched spec file.
- [ ] **Step 2:** Commit.

### Task 6: Web plumbing and picker

**Files:**

- Modify: `web/src/lib/api.ts`, `web/src/lib/cv.ts` (types `CvBaseItem`)
- Create: `web/src/lib/components/cv/BasePicker.svelte` (+ `.messages.ts`), `web/src/lib/tailor/tailorHref.ts`
- Test: `web/src/lib/tailor/tailorHref.test.ts`, `web/src/lib/components/cv/BasePicker.spec.ts`

**Interfaces:** Produces `api.listBaseCvs(): Promise<CvBaseItem[]>`, `api.duplicateCv(id): Promise<CvRecord>`, `api.tailorCv(jobSlug, baseCvId?)`, `tailorHref(slug: string, baseCvId?: string): string`, `<BasePicker bases value onChange>` rendering a radio list (renders nothing for fewer than 2 bases).

- [ ] **Step 1:** Failing tests: `tailorHref('a-b')` ⇒ `/tailor/a-b`; with base ⇒ `/tailor/a-b?base=<id>`; picker renders nothing for 0/1 bases, a radio per base for 2+, preselects `value`, marks the default.
- [ ] **Step 2:** `cd web && pnpm exec vitest run <files>` ⇒ FAIL. Implement. ⇒ PASS.
- [ ] **Step 3:** Commit.

### Task 7: Choose the base when tailoring

**Files:**

- Modify: `web/src/lib/confirmTailorDialog.svelte.ts`, `web/src/lib/components/ConfirmTailorDialog.svelte`, `JobView.svelte`, `JobDrawer.svelte` (call sites), `web/src/lib/cvIntakeDialog.svelte.ts` (`openCvIntake(baseCvId?)`, state `baseCvId`), `JdIntakeDialog.svelte` (picker when 2+ bases and none preselected; every `goto` uses `tailorHref`), `web/src/routes/my/cvs/+layout.svelte` (passes base), `web/src/routes/tailor/[slug]/+page.svelte` (reads `?base`, passes to both `api.tailorCv` calls)
- Test: extend/ add specs for dialog controller and intake

**Interfaces:** `askConfirmTailor(slug, label?): Promise<{ baseCvId?: string } | null>` (null = cancelled). Dialog loads `api.listBaseCvs()` on open; picker preselects `is_default`; with fewer than 2 bases the result has no `baseCvId` and no picker shows.

- [ ] **Step 1:** Failing tests: controller resolves `{baseCvId}` of the chosen base with 2 bases, `{}` with 1, `null` on cancel; intake shows picker only with 2+ bases and none preselected; intake with preselected base navigates to `?base=`.
- [ ] **Step 2:** Run vitest on them ⇒ FAIL; implement; ⇒ PASS. Note in the dialog: choosing a base does not change an already-tailored vacancy (one copy per vacancy).
- [ ] **Step 3:** `pnpm exec eslint` + `pnpm exec svelte-check` on touched files. Commit.

### Task 8: Base cards on `/my/cvs`

**Files:**

- Create: `web/src/lib/components/cv/BaseCvCards.svelte` (+ `BaseCvCards.messages.ts`)
- Modify: `web/src/lib/components/cv/CvList.svelte` (mount the block above the tailored list), `CvList.messages.ts` if shared strings
- Test: `BaseCvCards.spec.ts`

One card per base: title, template, last edited, "Default" badge, actions Edit (link `/my/cvs/<id>`), Tailor to a job (`openCvIntake(id)`), PDF (`api.cvPdfUrl`), Duplicate (`api.duplicateCv` then `goto` editor), Delete (existing `ConfirmDialog`; the description says tailored copies keep working). Empty state: button "Create from my CV" ⇒ `api.createCv({seed:true})` ⇒ editor. Preview thumbnail: skip (not cheap).

- [ ] **Step 1:** Failing spec: renders cards and all five actions; duplicate navigates; delete removes the card; empty state button present.
- [ ] **Step 2:** FAIL ⇒ implement ⇒ PASS. eslint/svelte-check. Commit.

### Task 9: Base editor page

**Files:**

- Modify: `web/src/routes/my/cvs/[id]/+page.svelte`, `messages.ts` (keeps the tailored redirect)
- Create: components only if the page exceeds ~250 lines
- Test: `web/src/routes/my/cvs/[id]/page.spec.ts` (or the repo's route-test convention)

Behaviour: `listBaseCvs()` contains the id ⇒ editor, else the existing redirect. Editor: title input, `CvSectionForm`, a Design tab with `TemplateGallery`, `StyleSettings`, `MarginSettings`, `CvHtmlPreview` live preview, PDF link, 800 ms autosave via `api.updateCv` (flush on destroy), **Update from my CV** (ConfirmDialog warning hand edits on this base are replaced ⇒ `api.reseedCv(id)` ⇒ re-hydrate). If the account shell constrains width, escape it with a layout reset (`+page@my.svelte`) and check visually.

- [ ] **Step 1:** Failing tests: base id renders editor and a title change triggers one debounced `updateCv`; tailored id redirects to `/tailor/<slug>?cv=<id>`; Update-from-CV requires confirm.
- [ ] **Step 2:** FAIL ⇒ implement ⇒ PASS. eslint, svelte-check, `pnpm check:dead` is CI-only: grep for newly unused exports by hand.
- [ ] **Step 3:** Browser check: run web against local Go server (see task brief), screenshot list + editor. Commit.

### Task 10: Ship

- [ ] **Step 1:** Full verification: `gofmt -l .`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./...`, integration tests for `internal/api/handler` and `internal/candidate/cv`, `pnpm check:sql`, `pnpm check:links` if AGENTS.md links changed, vitest/eslint/svelte-check on touched web files.
- [ ] **Step 2:** Run `/simplify`; commit its changes.
- [ ] **Step 3:** Push, open PR to `main`; read CodeRabbit (`gh api repos/<owner>/<repo>/pulls/<n>/comments`, `gh pr view <n> --json reviews`); fix valid findings, list skipped ones. Do not merge.
