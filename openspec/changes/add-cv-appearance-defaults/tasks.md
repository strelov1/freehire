## 1. Storage

- [x] 1.1 Add migration `migrations/0128_cv_appearance_defaults.sql`: table `cv_appearance_defaults` (`user_id bigint primary key references users(id)`, `template_id text not null`, `style jsonb not null`, `margins jsonb not null`, `updated_at timestamptz not null default now()`). Run `pnpm check:sql` on it.
- [x] 1.2 Add sqlc queries in `internal/platform/db/queries/` for get-by-user and upsert-by-user on `cv_appearance_defaults`; run `make sqlc`.

## 2. Backend domain logic (`internal/candidate/cv`)

- [x] 2.1 Write a failing unit test for `Store.GetAppearanceDefaults`: returns the system defaults (`DefaultTemplateID`, `DefaultMargins()`, zero-value `Style`) with `ok=false` when nothing is saved, and the saved row with `ok=true` otherwise.
- [x] 2.2 Add `internal/candidate/cv/appearance_defaults.go`: `AppearanceDefaults{TemplateID string; Style Style; Margins Margins}` and `Store.GetAppearanceDefaults(ctx, userID) (AppearanceDefaults, bool, error)` to make 2.1 pass.
- [x] 2.3 Write a failing unit test for `Store.SetAppearanceDefaults`: rejects an unknown `template_id` (reusing `ResolveTemplate`/`TemplateIDs`), and clamps out-of-range `Style`/`Margins` values the same way `Sanitize` does on a CV document.
- [x] 2.4 Implement `Store.SetAppearanceDefaults(ctx, userID, AppearanceDefaults) error` to make 2.3 pass, reusing `Style.sanitized()`/`Margins.sanitized()` — no duplicated clamping logic.
- [x] 2.5 Write a failing unit test for a new helper (e.g. `Store.effectiveAppearance(ctx, userID) (templateID string, style Style, margins Margins)`) that resolves saved defaults or falls back to system defaults — this is the one place all three creation call sites will use, per design.md's "Risks" note on keeping them in sync.
- [x] 2.6 Implement the helper to make 2.5 pass. **Simplify pass (post-3.6):** a dedicated `effectiveAppearance` helper turned out reachable only from `Store.Tailor` — the other two creation call sites live in `internal/api/handler`, a different package, and always had to call the exported `GetAppearanceDefaults` directly. The helper's own comment claiming to be "the one place every call site reads this from" was therefore false. Removed it (and its two now-redundant tests, since they only re-tested `GetAppearanceDefaults` through a thin wrapper) and updated `Store.Tailor` to call `GetAppearanceDefaults` directly, matching the other two call sites — `GetAppearanceDefaults` is now genuinely the one shared place, and its doc comment says so.

## 3. Backend application at the three creation call sites

- [x] 3.1 Write a failing unit test asserting `Store.Tailor`'s résumé-seed branch (`store.go:314`) uses the caller's saved appearance defaults when present, and the system defaults otherwise.
- [x] 3.2 Update `Store.Tailor` to call the effective-appearance helper instead of hardcoding `DefaultTemplateID`/zero-value `Document` margins/style, to make 3.1 pass.
- [x] 3.3 Write a failing unit/integration test asserting `cv_reset.go`'s re-seed (`cv_reset.go:132`) uses the caller's saved appearance defaults when present, and the system defaults otherwise.
- [x] 3.4 Update `cv_reset.go` to make 3.3 pass.
- [x] 3.5 Write a failing integration test (per repo convention: `internal/api/handler` integration-tagged tests using unexported constructors) asserting `CreateCV` seeds typography/margins from saved defaults, defaults the template to the saved one when the request omits `template_id`, and still honors an explicit request `template_id` over the saved default.
- [x] 3.6 Update `CreateCV` (`internal/api/handler/cv.go`) to make 3.5 pass.

## 4. API endpoints

- [x] 4.1 Write a failing integration test for `GET /api/v1/me/cv-appearance-defaults`: returns system defaults for a user with none saved, and the saved values otherwise.
- [x] 4.2 Write a failing integration test for `PUT /api/v1/me/cv-appearance-defaults`: persists valid input, rejects an unknown `template_id` with a client error, clamps out-of-range typography/margins.
- [x] 4.3 Implement both routes (new file, e.g. `internal/api/handler/cv_appearance_defaults.go`), registered `RequireAuth` (cookie-only), to make 4.1 and 4.2 pass.
- [x] 4.4 Run `go vet -tags=integration ./...` and the full integration suite for the touched packages.

## 5. Frontend: reusable components

- [x] 5.1 Generalize `web/src/lib/tailor/TemplateGallery.svelte` to accept either the existing `cvId` prop (unchanged self-persisting behavior) or new `value`/`onChange` props (controlled, no API call). Verify the tailoring workspace (`web/src/routes/tailor/[slug]/+page.svelte`) still works unmodified in `cvId` mode.
- [x] 5.2 Add `$lib/api` client methods for `GET`/`PUT /me/cv-appearance-defaults` and the corresponding TypeScript types (check `web/src/lib/generated/contracts` for existing `Style`/`Margins` types to reuse).

## 6. Frontend: settings screen and entry point

- [x] 6.1 Add `web/src/routes/my/cvs/appearance/+page.svelte`: loads effective defaults via GET, binds `TemplateGallery` (controlled mode) + `StyleSettings` + `MarginSettings` to local state, explicit "Save" button calling PUT, saved confirmation. No autosave.
- [x] 6.2 Add an entry-point button/icon in `web/src/lib/components/cv/CvList.svelte` (next to "Tailor for a job") linking to `/my/cvs/appearance`. No changes to `accountNav.ts`/`accountNavIcons.ts`.

## 8. Code review fixes (2026-09-03, `/code-review`)

- [x] 8.1 `GetAppearanceDefaults` now self-heals a saved `template_id` that no longer resolves (the registry in `template.go` can shrink after a save) back to `DefaultTemplateID`, instead of handing an unresolvable id on to `Store.Create` — which never validates its `templateID` argument — where it would otherwise silently reach a new CV and only fail at render time. Covered by `TestStoreGetAppearanceDefaultsHealsAStaleTemplate` (written failing first).
- [x] 8.2 `Store.SetAppearanceDefaults` now returns the saved `AppearanceDefaults` instead of discarding it, so `SetCVAppearanceDefaults` no longer needs a second `GetAppearanceDefaults` round trip just to report back what it wrote. All call sites (tests, the handler) updated for the new `(AppearanceDefaults, error)` signature.
- [x] 8.3 Fixed the now-inaccurate doc comment on `createCVRequest.TemplateID` in `internal/api/handler/cv.go` (it still described "empty defaults to classic-ats", which stopped being true once an omitted template falls back to the saved appearance default first).

## 7. Manual verification

**Skipped by explicit user decision** (2026-09-03): a docker-compose stack was already running
on this machine's ports 5432/8080/8090, likely owned by a concurrent session on the same repo,
so a live browser run was not safe to set up without either disturbing it or spending
significant setup time on an isolated stack. The user chose to rely on the automated coverage
instead — every scenario below is already exercised by an integration test against a real
Postgres (via testcontainers) through the actual HTTP handlers (`cv_reset_integration_test.go`,
`cv_integration_test.go`, `cv_appearance_defaults_integration_test.go`), plus `svelte-check`
(0 errors) and `pnpm test` (1376 passed) on the frontend. What automated coverage does NOT
prove: that the new `/my/cvs/appearance` page and the `CvList.svelte` entry point actually
render and behave correctly in a live browser. Remains a gap until someone runs it manually.

- [ ] 7.1 Start the dev server; as a user with an existing base CV, open `/my/cvs`, reach the new appearance-defaults screen via the entry point, change template/typography/margins, save, and confirm the existing CV's own appearance is untouched.
- [ ] 7.2 As a user with saved appearance defaults and no CV yet, create a new CV (seeded and empty paths) and confirm it starts from the saved defaults.
- [ ] 7.3 As a user with no saved appearance defaults, create a new CV and confirm it still gets the system defaults (`classic-ats`, 0.5in margins, template-default typography) exactly as before this change.
- [ ] 7.4 Confirm an explicit `template_id` passed on CV creation still wins over a saved default.
