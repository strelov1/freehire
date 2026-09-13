## 1. Search document: compute the facet from `source`

- [x] 1.1 ~~Add a small, documented provider-set constant in
      `internal/search/search`~~ Superseded by 1.3 below: the constant
      (`AutoApplyProviders`) now lives in `internal/job/jobview`, not
      `internal/search/search` — see the note under 1.3/1.4.
- [x] 1.2 Add a unit test asserting the constant's exact expected 4-provider
      set (fails loudly if the list is ever edited to something unexpected).
      Now `TestAutoApplyProviders_ExactExpectedSet` in
      `internal/job/jobview/autoapply_test.go`, moved alongside the constant.
- [x] 1.3 ~~Add `AutoApplyAvailable bool` to `JobDocument`~~ **Corrected after
      GitHub PR review** (strelov1/freehire#2768): the spec's own "facet is
      served and filterable" requirement means the signal must reach the
      public wire shape, not just the search index document — but the
      original placement on `JobDocument` only ever flattened into the
      Meilisearch document; the actual served job object (`jobview.Job`,
      what `views[i] = hit.Job` extracts for every list/detail/search
      response) never carried it. Fixed by moving the field onto
      `jobview.Job` itself (`json:"auto_apply_available,omitempty"`,
      `internal/job/jobview/jobview.go`, mirroring `RequiresClearance`) and
      the provider-set constant into `internal/job/jobview/autoapply.go`
      (job, layer 5) — jobview can't import `internal/search`, so the
      constant had to move down to where it's needed, not stay up where it
      was merely convenient. `JobDocument.AutoApplyAvailable` is no longer a
      separate field: it is promoted from the embedded `jobview.Job`, the
      same way every other served facet already flattens into the document.
      Also added `auto_apply_available` to `web/static/openapi.yaml` (query
      param + `Job` schema property, mirroring `requires_clearance`) and
      regenerated `web/src/lib/generated/contracts.ts` (`go run
      ./cmd/gen-contracts`), which now picks up the field since it's a real
      `jobview.Job` member.
- [x] 1.4 Compute it inline in `jobview.FromDomain`
      (`internal/job/jobview/jobview.go`) as a lookup of the job's `Source`
      against `AutoApplyProviders` (1.3). `search.FromJob` no longer
      recomputes it — it reuses the value already on the `jobview.Job` it
      builds via `jobview.FromRow`, one fewer place for the two copies to
      drift apart than the original design had.
- [x] 1.5 Add test cases (in `document_test.go` or a new
      `auto_apply_available_test.go`, mirroring `ai_interview` coverage) for:
      each of the four eligible providers marks the document; `recruitee` and
      an arbitrary non-ATS source do not; the JSON omits the key when false.
      The provider-eligibility cases now live in
      `internal/job/jobview/autoapply_test.go`
      (`TestFromDomain_AutoApplyAvailableFacet`, mirroring
      `TestFromDomain_RequiresClearanceFacet`'s shape); the
      `internal/search/search/auto_apply_available_test.go` cases stay, now
      covering only that `FromJob` passes the already-computed value through
      to the document JSON unchanged.
      `internal/api/atsapply/auto_apply_facet_sync_test.go`'s cross-check now
      asserts against `jobview.AutoApplyProviders` (was
      `search.AutoApplyProviders`), same rationale, atsapply (layer 8) can
      import job (layer 5) same as it could import search (layer 6).

## 2. Meilisearch settings and query-filter plumbing

- [x] 2.1 Add `"auto_apply_available"` to `FilterableAttributes` in
      `facetSettings()` (`internal/search/search/client.go`), with a comment
      mirroring the `ai_interview` one about settings-before-binary ordering.
- [x] 2.2 Add `AutoApplyAvailableParam` exported constant
      (`internal/search/search/query_filter.go`), mirroring
      `AIInterviewParam`/`RequiresClearanceParam`.
- [x] 2.3 Add `"auto_apply_available": "auto_apply_available"` to
      `StringFacets`, and register it as a true-or-absent param (alongside
      `requires_clearance`/`ai_interview`).
- [x] 2.4 Add test cases (mirroring `clearance_filter_test.go` /
      `ai_interview_integration_test.go`) covering:
      `auto_apply_available=true` filters to marked postings only; omitting
      the param changes nothing; the param is included in the `/jobs/facets`
      distribution. (Distribution inclusion is structural — every
      `StringFacets` entry is enrolled automatically, the same way
      `requires_clearance`/`ai_interview` have no dedicated distribution
      test either.) Unit-level string-building coverage is in
      `auto_apply_available_filter_test.go`; the real end-to-end proof
      against a live Meilisearch (mirroring `TestSearchFiltersByAIInterviewFacet`,
      plus `TestAutoApplyAvailableParamIsKnown` mirroring
      `TestAIInterviewParamIsKnown`) is in the new
      `auto_apply_available_integration_test.go` (`-tags=integration`,
      needs Docker) — added after code review flagged the initial pass as
      unit-only. Both run green (`CGO_ENABLED=0 go test -tags=integration
      ./internal/search/search/...`, needed locally only because of an
      unrelated broken macOS SDK/linker issue on this host).
- [x] 2.5 Add the new attribute to the settings-drift expectations
      (`settings_test.go`/`settings_drift_test.go`) so
      `search-settings-drift` tracks it like every other filterable
      attribute. (`settings_drift_test.go`'s tests exercise the generic
      diff logic against fixture settings, not the real attribute list, so
      nothing there names individual attributes; added a
      `TestFacetSettings_AutoApplyAvailableIsFilterable` assertion in
      `auto_apply_available_filter_test.go`, mirroring the shape of
      `settings_test.go`'s existing per-attribute checks like
      `is_tech`/`ai_archetype`.)

## 3. Frontend: filter model

- [x] 3.1 Add `autoApplyAvailable: boolean` to the filter type in
      `web/src/lib/facetModel.ts` (near `visa`/`hideAIInterview`, ~line
      32/41), with a `false` default (~line 214/215).
- [x] 3.2 Add serialize logic in `filtersToParams`: when true, set
      `auto_apply_available=true`; when false, omit the param entirely
      (never serialize `=false`) — mirrors the true-or-absent contract from
      the spec, not the `hideAIInterview` negated-serialization shape.
- [x] 3.3 Add deserialize logic in `filtersFromParams`:
      `f.autoApplyAvailable = p.get('auto_apply_available') === 'true'`.
- [x] 3.4 Include it in `activeFilterCount` (~lines 185-186/338-339).
- [x] 3.5 Add dedicated serialize/deserialize/round-trip test cases in
      `facetModel.test.ts` for `autoApplyAvailable` (none exist yet for
      `visa`/`hideAIInterview` to mirror — follow the shape of the
      `experienceYearsMax`-style dedicated tests instead).

## 4. Frontend: checkbox UI

- [x] 4.0 (Discovered during implementation — `visa`/`hideAIInterview` wiring
      spans more than facetModel.ts.) Add `setAutoApplyAvailable(on: boolean)`
      to both `StagedFilters` (`web/src/lib/stagedFilters.svelte.ts`, mirrors
      `setVisa`/`setHideAIInterview`) and the live `FilterStore`
      (`web/src/lib/filters.ts`, same `setNow` discrete-input shape as its
      `setVisa`/`setHideAIInterview`).
- [x] 4.1 Add the "Auto-apply available" checkbox to
      `web/src/lib/components/filters/FilterModal.svelte`, alongside the
      existing "Offers visa sponsorship"/"Hide employers reported to
      interview with AI" checkboxes (~lines 482-501), wired to
      `staged.value.autoApplyAvailable` / `staged.setAutoApplyAvailable`. It
      lands under the same `relocation` rail entry as visa/AI-interview/
      clearance (the existing catch-all eligibility pane); its own badge
      count was added to that entry's `entryCount`.
- [x] 4.1b Add the sidebar chip in
      `web/src/lib/components/filters/FilterSummary.svelte`, mirroring the
      `visa` chip (`push('Visa', ...)` with a `remove` calling
      `store.setAutoApplyAvailable(false)`).
- [x] 4.2 Add the "Best-effort — successful submission isn't guaranteed"
      caption under the checkbox.
- [x] 4.3 Manually verify in the running app: checking the box stages the
      filter, "Show results" applies it, the sidebar chip reflects it, and
      removing the chip clears it — per `run` skill, drive the actual UI, not
      just the unit tests. Verified live via `make up` + `docker compose run
      --entrypoint /app/migrate|/app/reindex app` (empty catalogue, index
      created with 0 docs) + a throwaway Playwright script driving headless
      Chromium against http://localhost:8090/jobs: checkbox renders under
      the Relocation rail entry with its caption, checking it and clicking
      Show stages+applies `?auto_apply_available=true` in the URL, the
      sidebar shows an "AUTO-APPLY / Available ×" chip, and clicking its ×
      clears the param back to a bare `/jobs` with "No filters yet" — no
      console/page errors. Stack torn down (`make down`) afterward.

## 5. Rollout

- [ ] 5.1 Confirm deploy order in the release checklist/notes: Meilisearch
      settings patch reaches the live index before the binary carrying the
      new `StringFacets` entry (same rule `clearance-facet`/`ai_interview`
      already follow) — verify `/api/v1/jobs/facets` does not 500 during
      rollout.
- [ ] 5.2 After the binary is live, run one full `make reindex` so
      pre-existing open postings from eligible providers carry the facet.
- [ ] 5.3 Spot-check post-reindex: `GET /api/v1/jobs?auto_apply_available=true`
      returns only postings whose `source` is one of the four eligible
      providers, including postings that predate this change.
