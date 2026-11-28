## 1. Schema: context-gate flag

- [x] 1.1 Add `migrations/0077_users_experience_require_context.sql`: `ALTER TABLE public.users ADD COLUMN experience_require_context boolean NOT NULL DEFAULT false;` — header note that this is expansive (next binary reads it on `experience_add` / `get_profile` / `POST /me/experience/atoms`) and must run before that deploy
- [x] 1.2 Add sqlc queries `GetUserExperienceRequireContext` and `SetUserExperienceRequireContext` in `internal/db/queries/users.sql`; do not add the column to `GetUserByID` / `/auth/me`
- [x] 1.3 Run `make sqlc` and commit the generated `internal/db` output

## 2. Domain: similarity, richness, merge

- [x] 2.1 Add `SoftDuplicateClusters` (employment-scoped union-find, claim-only `meaningfulTokens`, Jaccard ≥ 0.40) and `Richness` (`needs_context` / `needs_metrics` per design) next to `meaningfulTokens` in `internal/experience`, with table-driven tests: the two faster-whisper plugin claims cluster; “20s to 1s” vs “30s to 1s” do not; stopword-only overlap does not; cross-employment same claim does not cluster; a “40%” claim is not `needs_metrics`
- [x] 2.2 Add merge sentinels (`ErrInvalidMerge` / cross-employment / same-atom) and `MergeAtoms` keep-selection + field-union helpers (score, longer context, metrics/skills union then Sanitize, publishable provenance rule, claim/`claim_key` unchanged) with unit tests covering the screenshot pair, inferred+publishable → publishable, inferred+inferred → inferred, cross-employment refuse, same id refuse
- [x] 2.3 Add sqlc `MergeExperienceAtoms`: one statement that `DELETE`s the loser `RETURNING` into a CTE and `UPDATE`s the keep only if the delete landed (owner-scoped on both ids); zero rows → not found
- [x] 2.4 Implement `Store.MergeAtoms` over Get×2 + validate + CTE write; fake-repo or store tests for not-found / not-yours; integration test that both atoms exist before and only the keep remains after, with unioned metrics/skills/context
- [x] 2.5 Update `internal/experience/AGENTS.md`: paraphrase-dup limitation now has an owner merge path; note dangling CV `evidence_id` after loser delete; drop the stale “No UI yet” line if it is still there

## 3. HTTP: list flags, merge route, create gate

- [x] 3.1 Project `GET /me/experience` atoms as views: atom fields plus `needs_context`, `needs_metrics`, optional `cluster_id` (first member id, response-local). Do not add those fields to `experience.Atom`
- [x] 3.2 Extend `experienceBankOwner` with `MergeAtoms`; add `POST /me/experience/atoms/merge` under `mw.cookie` (body `{"ids":[id1,id2]}`), map sentinels through `experienceError`, respond `200 {"data": <kept atom>}`
- [x] 3.3 Gate `AddAtom` (HTTP) on `experience_require_context`: if on and context empty → `ErrContextRequired` / 400, persist nothing; import and `UpdateAtom` stay ungated
- [x] 3.4 Tests in `internal/handler/me_experience_test.go`: merge happy path + reload list missing loser; cookie vs full-scope key (key refused); cross-employment / foreign id / malformed ids; list flags on the plugin pair; context-gate on/off for `POST /me/experience/atoms`

## 4. Assistant: merge tool, opt-in tool, prompts, summaries

- [x] 4.1 Add `experience_merge` (`ids`, every preset) calling `Store.MergeAtoms`; result names kept atom + deleted id; errors are tool results. Register beside the other bank tools
- [x] 4.2 Add `experience_set_require_context` (`enabled` bool, every preset) writing the user flag; `get_profile` / `experienceSummary` reports `require_context`, `soft_duplicate_clusters` (cap 8 clusters × 6 ids), `needs_context_count`, `needs_metrics_count` — still no atom bodies. Extend `TestProfileToolReportsShapeNotContents`
- [x] 4.3 Gate `experience_add` on the flag the same way as HTTP create; `experience_update` ungated
- [x] 4.4 Add `needs_context` / `needs_metrics` on `requirementEvidence` used by `interview_context` (and `cv_context` if it shares that struct)
- [x] 4.5 Update `profilePrompt` (and `TestPromptOnlyNamesToolsThePresetHas`): work list includes clusters + thin counts; ask before `experience_merge`; after merge optionally `experience_update` the claim; explain context-gate trade-off and call `experience_set_require_context` only after agreement

## 5. SPA: Experience tab + interviewer arrival

- [x] 5.1 Extend `ExperienceAtom` / bank types (or a list-view wrapper) and `api.ts` with `mergeExperienceAtoms(ids)` → `POST /api/v1/me/experience/atoms/merge`
- [x] 5.2 `ExperienceBankView.svelte`: multi-select + action bar (Merge when exactly two share a bucket; Tailor for 1+ → `/my/assistant?preset=profile&atoms=<ids>`); show cluster / thin hints; edit mode includes context + metrics; save still `PUT` (stamps `manual`)
- [x] 5.3 `entryFromQuery`: `atoms` comma-UUIDs append a kickoff line naming those ids; ignore malformed; existing `preset=profile` kickoff unchanged. Tests in `presets.test.ts`
- [x] 5.4 Offer a `/blog` changelog entry for the Experience tab merge + interviewer enrich (user-facing)

## 6. Verify

- [x] 6.1 `go build ./... && go vet ./... && go vet -tags=integration ./... && go test ./...`
- [x] 6.2 `openspec validate --all --strict` (or the repo’s equivalent) after artifacts land
