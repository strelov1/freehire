## 1. The AI filter first, and alone

- [x] 1.1 Replace `searchintent`'s `role` enum with `category` + `seniority`, built from the same vocabularies the rest of the request uses
- [x] 1.2 Update the interpreter's tests, including the contradiction cases — a grade and a specialization are now two fields, so a proposal can disagree with itself in a way one slug could not
- [x] 1.3 Verify against real prompts before anything else lands: "senior backend in Berlin", "junior QA remote", "staff data engineer" must each still produce the search they name

## 2. Stop serving the facet

- [x] 2.1 Remove `role` from `search.StringFacets`, which takes it out of the filter grammar and the `/jobs/facets` distribution in one edit
- [x] 2.2 Confirm `role=` now lands in `meta.ignored_params` rather than being refused, and that `role_exclude` / `role_mode` go with it
- [x] 2.3 Remove the Role pane from the filter modal and the facet from `web/src/lib/facets.ts`
- [x] 2.4 Remove the facet, its parameter and its schema references from `web/static/openapi.yaml`; re-validate the document
- [x] 2.5 Drop the role branches from `seeAlsoMark.ts`, `familymarks.ts`, `saveSearchAlert.ts`, `cv.ts` and `facetValueLabel`

## 3. Stop building role suggestions

- [x] 3.1 Remove `KindRole` from the suggestion builder, with the one-row-per-base-role collapse it needed
- [x] 3.2 Remove the category-vs-role de-duplication — the collision it arbitrated is gone, and its absence is what lets specializations into the dictionary
- [x] 3.3 Drop `role` from the parse's kind precedence and from `singular`
- [x] 3.4 Rebuild the dictionary against production and confirm specializations now appear as suggestions, which they never have

## 4. Delete the dictionary

- [x] 4.1 Delete `internal/dict/roletag` and its entry in `internal/platform/arch/layering/blocks.go`
- [x] 4.2 Remove `roles` from `search.JobDocument` and from `document.go`'s derivation
- [x] 4.3 Remove `ROLE_LABELS` / `ROLE_ALIASES` from `cmd/gen-contracts`, regenerate the contracts, and delete `web/src/lib/roleRelated.ts` with `relatedOptions` if nothing else uses it
- [x] 4.4 Confirm `internal/dict/classify`, `internal/ai/aiarchetype` and `internal/dict/roletype` compile untouched — all three name roletag only in comments
- [x] 4.5 `pnpm check:dead` stays clean, and `go test ./...` plus `go vet -tags=integration ./...` pass

## 5. Ship

- [x] 5.1 Update `internal/search/AGENTS.md`, `openspec/specs` references and the root `CLAUDE.md` where they name the facet
- [x] 5.2 Deployed. The attribute is off the LIVE index's filterable list via a settings
  PUT (2.7 min) rather than via a rebuild: the host has 37G free against the rebuild's
  40G floor, and a swap-rebuild needs a second copy of a 33G index. The settings change
  does the load-bearing half — nothing can filter on it and the inverted index is
  dropped — and the field leaves the document JSON on the next successful full rebuild,
  which `facetSettings()` already declares without it. No drift: code and index agree.
- [x] 5.3 Verify on production: `role=backend` reports itself ignored, `category=backend&seniority=senior` returns what `role=senior_backend` used to, and the box offers specializations
