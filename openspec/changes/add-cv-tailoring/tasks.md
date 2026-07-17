## 1. Patch model (pure, unit-tested)

- [x] 1.1 Add `cv.Patch` type in a dependency-light file in `internal/cv/` (ops: set-summary, add/replace/remove/reorder bullet at `experience[i]`, set-skill-group, set-header-field), with a discriminated op field
- [x] 1.2 Implement pure `cv.Apply(doc Document, p Patch) (Document, error)` mirroring `cv.Seed` — no I/O, returns a client-style error for out-of-range/unknown addressing, never mutates the input
- [x] 1.3 Unit tests for `cv.Apply`: each op happy path, other sections byte-for-byte unchanged, reorder is a pure permutation (no add/drop), out-of-range index errors

## 2. SQL layer

- [x] 2.1 Add `cvs.sql` query to fetch a user's base CV (`job_id IS NULL`), owner-scoped
- [x] 2.2 Extend/add a `CreateCV` path that sets `job_id` for the tailored copy; regenerate `internal/db` via `make sqlc`
- [x] 2.3 Integration test (build-tag) for base-CV fetch and tailored-row creation with `job_id`

## 3. Store: tailoring bootstrap + patch

- [x] 3.1 `cv.Store` method to apply a patch: load → `cv.Apply` → `Sanitize` → update, owner-scoped; 422 on bad addressing
- [x] 3.2 `cv.Store`/service bootstrap: find base CV or seed one from `resume.Structured` (absent résumé → typed 409 error, no row created), then create the tailored copy from the base
- [x] 3.3 Unit/integration tests: bootstrap seeds base when absent, refuses without résumé, tailored copy equals base and base is untouched

## 4. Scoped credential

- [x] 4.1 Mint a short-lived API key for the user via the existing `api_keys` machinery (`mintTailoringKey`, 2h TTL; no per-endpoint scope column exists → owner-scoped only). Delivery to the CLI (`~/.freehire` config vs env) is a cross-repo companion concern
- [x] 4.2 Test: minted key authenticates the CV endpoints and is owner-scoped (cannot touch another user's CV) — unit test covers hash↔token match; endpoint owner-scoping exercised in `TestPatchCVViaKey`

## 5. HTTP surface + gating

- [x] 5.1 `PATCH /api/v1/me/cvs/:id` handler (`RequireAuthOrKey` + beta gate) → store patch; 422 on bad addressing, 404 on non-owner
- [x] 5.2 `POST /api/v1/me/cvs/tailor` handler (`RequireAuth` + beta gate): 409 when no cached `jobfit.Analysis`, 409 when no résumé; returns `{tailor_cv_id, base_cv_id, analysis, cli_token}`
- [x] 5.3 `GET /api/v1/me/cvs/:id/tailor-context` handler (`RequireAuthOrKey` + beta): verdict + recommendation + dimension comments + requirements split `missing-have`/`missing-gap`, from cached analysis, no LLM
- [x] 5.4 Wire routes in `internal/handler/handler.go` under the existing `cvGate`; handler integration tests for the three endpoints (preconditions, gating, owner-scoping)

## 6. Typed contracts

- [x] 6.1 Register `cv.Patch` in `cmd/gen-contracts` (added `patch.go` to the cv package); `PatchOp` + `Patch` land in `contracts.ts`. The tailor-context/response wrappers are handler projections composed on the web from the already-emitted `Analysis`/`Requirement`/`Dimension`, not emitted (unexported handler structs)

## 7. Web entry point

- [x] 7.1 On `/jobs/[slug]/fit`, show a "Tailor my CV" CTA only when a cached analysis exists (`data.fit?.analysis`), the caller has a CV, and is beta; `api.tailorCv(slug)` bootstraps and `goto`s the tailored CV editor. Added `api.tailorCv` + `TailorResult` type
- [~] 7.2 CROSS-REPO (deferred): a live split chat+preview where the agent edits on the fly requires `freehire-agent` to accept session context (`createSession` currently sends `{}`) + `freehire-cli` `cv` commands. In-repo, the tailored CV is viewable/renderable via the existing `CvEditor` + `cvPdfUrl` (`/my/cvs/[id]`), which the CTA opens
- [x] 7.3 Frontend verification: `svelte-check` clean (0 errors); CTA gated on analysis+CV+beta. Live visual verify of the seeded session belongs to the cross-repo agent surface (7.2)

## 8. Verify

- [x] 8.1 `go build ./...` (exit 0), `go vet ./...` (exit 0), `go test ./...` (0 fail), `svelte-check` (0 errors); `cv.Apply` + store + handler unit + integration green
- [x] 8.2 End-to-end via handler integration tests over real Postgres: bootstrap (409 no-analysis → 409 no-résumé → 201) → patch (apply / 422 / 404 owner) → tailor-context (missing_have/gap split / 409). The live UI walkthrough (fit → seeded agent session → live preview) is the cross-repo companion (see 7.2)
